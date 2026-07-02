package onebot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

type Config struct {
	ListenAddress  string
	WebSocketPath  string
	AccessToken    string
	RequestTimeout time.Duration
}

type EventHandler func(*Session, *Event)

type Server struct {
	log      zerolog.Logger
	config   Config
	handler  EventHandler
	upgrader websocket.Upgrader

	httpServer *http.Server

	sessionsMu sync.RWMutex
	sessions   map[string]*Session
}

func NewServer(log zerolog.Logger, config Config, handler EventHandler) *Server {
	if config.ListenAddress == "" {
		config.ListenAddress = "0.0.0.0:8080"
	}
	if config.WebSocketPath == "" {
		config.WebSocketPath = "/onebot/v11/ws"
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second
	}
	return &Server{
		log:     log,
		config:  config,
		handler: handler,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
		sessions: make(map[string]*Session),
	}
}

func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc(s.config.WebSocketPath, s.handleWebSocket)

	ln, err := net.Listen("tcp", s.config.ListenAddress)
	if err != nil {
		return fmt.Errorf("failed to listen for NapCat reverse WebSocket: %w", err)
	}

	s.httpServer = &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		_ = s.httpServer.Shutdown(context.Background())
	}()
	go func() {
		err := s.httpServer.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Err(err).Msg("NapCat reverse WebSocket server stopped unexpectedly")
		}
	}()
	s.log.Info().
		Str("listen_address", s.config.ListenAddress).
		Str("path", s.config.WebSocketPath).
		Msg("NapCat reverse WebSocket server started")
	return nil
}

func (s *Server) Stop() {
	if s.httpServer != nil {
		_ = s.httpServer.Shutdown(context.Background())
	}
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	for _, sess := range s.sessions {
		sess.Close()
	}
	s.sessions = make(map[string]*Session)
}

func (s *Server) GetSession(selfID string) *Session {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	return s.sessions[selfID]
}

func (s *Server) ListSessions() []*Session {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	out := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	return out
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Err(err).Msg("Failed to upgrade NapCat WebSocket")
		return
	}

	sess := newSession(s, conn, r.RemoteAddr)
	go sess.readLoop()
	go s.identifySession(sess)
}

func (s *Server) authorize(r *http.Request) bool {
	if s.config.AccessToken == "" {
		return true
	}
	if r.URL.Query().Get("access_token") == s.config.AccessToken {
		return true
	}
	auth := r.Header.Get("Authorization")
	return auth == "Bearer "+s.config.AccessToken || auth == "Token "+s.config.AccessToken
}

func (s *Server) identifySession(sess *Session) {
	ctx, cancel := context.WithTimeout(context.Background(), s.config.RequestTimeout)
	defer cancel()

	info, err := sess.GetLoginInfo(ctx)
	if err != nil {
		s.log.Err(err).Str("remote_addr", sess.remoteAddr).Msg("Failed to identify NapCat session")
		sess.Close()
		return
	}
	if info.UserID == "" {
		s.log.Warn().Str("remote_addr", sess.remoteAddr).Msg("NapCat session returned empty user_id")
		sess.Close()
		return
	}

	selfID := info.UserID.String()
	sess.setSelf(*info)

	s.sessionsMu.Lock()
	if old := s.sessions[selfID]; old != nil && old != sess {
		old.Close()
	}
	s.sessions[selfID] = sess
	s.sessionsMu.Unlock()

	s.log.Info().Str("self_id", selfID).Str("nickname", info.Nickname).Msg("NapCat session connected")
}

func (s *Server) removeSession(sess *Session) {
	selfID := sess.SelfID()
	if selfID == "" {
		return
	}
	s.sessionsMu.Lock()
	if s.sessions[selfID] == sess {
		delete(s.sessions, selfID)
	}
	s.sessionsMu.Unlock()
	s.log.Info().Str("self_id", selfID).Msg("NapCat session disconnected")
}

type pendingCall struct {
	resp chan response
}

type Session struct {
	server     *Server
	conn       *websocket.Conn
	remoteAddr string

	writeMu sync.Mutex
	pending sync.Map
	closed  atomic.Bool
	seq     atomic.Uint64

	selfMu sync.RWMutex
	self   LoginInfo
}

func newSession(server *Server, conn *websocket.Conn, remoteAddr string) *Session {
	return &Session{
		server:     server,
		conn:       conn,
		remoteAddr: remoteAddr,
	}
}

func (s *Session) SelfID() string {
	s.selfMu.RLock()
	defer s.selfMu.RUnlock()
	return s.self.UserID.String()
}

func (s *Session) Nickname() string {
	s.selfMu.RLock()
	defer s.selfMu.RUnlock()
	return s.self.Nickname
}

func (s *Session) IsConnected() bool {
	return !s.closed.Load()
}

func (s *Session) setSelf(info LoginInfo) {
	s.selfMu.Lock()
	s.self = info
	s.selfMu.Unlock()
}

func (s *Session) Close() {
	if s.closed.Swap(true) {
		return
	}
	_ = s.conn.Close()
	s.server.removeSession(s)
}

func (s *Session) readLoop() {
	defer s.Close()
	for {
		_, data, err := s.conn.ReadMessage()
		if err != nil {
			return
		}
		var probe struct {
			Echo json.RawMessage `json:"echo"`
		}
		if err := json.Unmarshal(data, &probe); err == nil && len(probe.Echo) > 0 && string(probe.Echo) != "null" {
			var resp response
			if err := json.Unmarshal(data, &resp); err != nil {
				s.server.log.Err(err).RawJSON("packet", data).Msg("Failed to parse OneBot response")
				continue
			}
			echo := strings.Trim(string(probe.Echo), `"`)
			if pending, ok := s.pending.LoadAndDelete(echo); ok {
				pending.(pendingCall).resp <- resp
			}
			continue
		}

		var evt Event
		if err := json.Unmarshal(data, &evt); err != nil {
			s.server.log.Err(err).RawJSON("packet", data).Msg("Failed to parse OneBot event")
			continue
		}
		if evt.SelfID != "" && s.SelfID() != "" && evt.SelfID.String() != s.SelfID() {
			s.server.log.Warn().
				Str("session_self_id", s.SelfID()).
				Str("event_self_id", evt.SelfID.String()).
				Msg("Dropping event for different self_id")
			continue
		}
		if s.server.handler != nil {
			s.server.handler(s, &evt)
		}
	}
}

type request struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
	Echo   string         `json:"echo"`
}

type response struct {
	Status  string          `json:"status"`
	RetCode int             `json:"retcode"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Wording string          `json:"wording"`
	Echo    string          `json:"echo"`
}

func (s *Session) Call(ctx context.Context, action string, params map[string]any, out any) error {
	if s.closed.Load() {
		return fmt.Errorf("NapCat session is disconnected")
	}
	echo := fmt.Sprintf("%s-%d", action, s.seq.Add(1))
	pending := pendingCall{resp: make(chan response, 1)}
	s.pending.Store(echo, pending)
	defer s.pending.Delete(echo)

	s.writeMu.Lock()
	err := s.conn.WriteJSON(request{Action: action, Params: params, Echo: echo})
	s.writeMu.Unlock()
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp := <-pending.resp:
		if (resp.Status != "" && resp.Status != "ok") || resp.RetCode != 0 {
			if resp.Message != "" {
				return fmt.Errorf("%s failed: %s", action, resp.Message)
			}
			if resp.Wording != "" {
				return fmt.Errorf("%s failed: %s", action, resp.Wording)
			}
			return fmt.Errorf("%s failed with retcode %d", action, resp.RetCode)
		}
		if out == nil || len(resp.Data) == 0 || string(resp.Data) == "null" {
			return nil
		}
		dec := json.NewDecoder(strings.NewReader(string(resp.Data)))
		dec.UseNumber()
		return dec.Decode(out)
	}
}

func (s *Session) GetLoginInfo(ctx context.Context) (*LoginInfo, error) {
	var info LoginInfo
	err := s.Call(ctx, "get_login_info", nil, &info)
	return &info, err
}

func (s *Session) SendPrivateMessage(ctx context.Context, userID string, msg []Segment) (*SendMessageResponse, error) {
	var resp SendMessageResponse
	err := s.Call(ctx, "send_private_msg", map[string]any{
		"user_id": userID,
		"message": msg,
	}, &resp)
	return &resp, err
}

func (s *Session) SendGroupMessage(ctx context.Context, groupID string, msg []Segment) (*SendMessageResponse, error) {
	var resp SendMessageResponse
	err := s.Call(ctx, "send_group_msg", map[string]any{
		"group_id": groupID,
		"message":  msg,
	}, &resp)
	return &resp, err
}

func (s *Session) DeleteMessage(ctx context.Context, messageID string) error {
	return s.Call(ctx, "delete_msg", map[string]any{"message_id": messageID}, nil)
}

func (s *Session) GetGroupInfo(ctx context.Context, groupID string) (*GroupInfo, error) {
	var info GroupInfo
	err := s.Call(ctx, "get_group_info", map[string]any{"group_id": groupID}, &info)
	return &info, err
}

func (s *Session) GetGroupMemberList(ctx context.Context, groupID string) ([]GroupMember, error) {
	var members []GroupMember
	err := s.Call(ctx, "get_group_member_list", map[string]any{"group_id": groupID}, &members)
	return members, err
}

func (s *Session) GetStrangerInfo(ctx context.Context, userID string) (*StrangerInfo, error) {
	var info StrangerInfo
	err := s.Call(ctx, "get_stranger_info", map[string]any{"user_id": userID}, &info)
	return &info, err
}

func (s *Session) GetFile(ctx context.Context, fileID string) (*FileResponse, error) {
	var resp FileResponse
	err := s.Call(ctx, "get_file", map[string]any{"file_id": fileID}, &resp)
	return &resp, err
}

func (s *Session) GetMessage(ctx context.Context, messageID string) (*MessageDetail, error) {
	var resp MessageDetail
	err := s.Call(ctx, "get_msg", map[string]any{"message_id": messageID}, &resp)
	return &resp, err
}

func (s *Session) GetForwardMessage(ctx context.Context, messageID string) (*ForwardMessageResponse, error) {
	var resp ForwardMessageResponse
	err := s.Call(ctx, "get_forward_msg", map[string]any{"message_id": messageID}, &resp)
	return &resp, err
}

func (s *Session) GetRecentContacts(ctx context.Context, count int) ([]RecentContact, error) {
	var resp []RecentContact
	err := s.Call(ctx, "get_recent_contact", map[string]any{"count": count}, &resp)
	return resp, err
}

func (s *Session) GetFriendMessageHistory(ctx context.Context, userID string, messageSeq string, count int, reverseOrder bool) (*MessageHistoryResponse, error) {
	params := map[string]any{
		"user_id":       userID,
		"count":         count,
		"reverseOrder":  reverseOrder,
		"reverse_order": reverseOrder,
	}
	if messageSeq != "" {
		params["message_seq"] = messageSeq
	}
	var resp MessageHistoryResponse
	err := s.Call(ctx, "get_friend_msg_history", params, &resp)
	return &resp, err
}

func (s *Session) GetGroupMessageHistory(ctx context.Context, groupID string, messageSeq string, count int, reverseOrder bool) (*MessageHistoryResponse, error) {
	params := map[string]any{
		"group_id":      groupID,
		"count":         count,
		"reverseOrder":  reverseOrder,
		"reverse_order": reverseOrder,
	}
	if messageSeq != "" {
		params["message_seq"] = messageSeq
	}
	var resp MessageHistoryResponse
	err := s.Call(ctx, "get_group_msg_history", params, &resp)
	return &resp, err
}

func (s *Session) MarkMessageAsRead(ctx context.Context, messageID string) error {
	return s.Call(ctx, "mark_msg_as_read", map[string]any{"message_id": messageID}, nil)
}

func (s *Session) MarkPrivateMessageAsRead(ctx context.Context, userID string) error {
	return s.Call(ctx, "mark_private_msg_as_read", map[string]any{"user_id": userID}, nil)
}

func (s *Session) MarkGroupMessageAsRead(ctx context.Context, groupID string) error {
	return s.Call(ctx, "mark_group_msg_as_read", map[string]any{"group_id": groupID}, nil)
}

func (s *Session) SetInputStatus(ctx context.Context, userID string, eventType int) error {
	return s.Call(ctx, "set_input_status", map[string]any{
		"user_id":    userID,
		"event_type": eventType,
	}, nil)
}

func (s *Session) SetGroupName(ctx context.Context, groupID string, groupName string) error {
	return s.Call(ctx, "set_group_name", map[string]any{
		"group_id":   groupID,
		"group_name": groupName,
	}, nil)
}

func (s *Session) SetGroupPortrait(ctx context.Context, groupID string, file string) error {
	return s.Call(ctx, "set_group_portrait", map[string]any{
		"group_id": groupID,
		"file":     file,
	}, nil)
}

func (s *Session) SetGroupKick(ctx context.Context, groupID string, userID string, rejectAddRequest bool) error {
	return s.Call(ctx, "set_group_kick", map[string]any{
		"group_id":           groupID,
		"user_id":            userID,
		"reject_add_request": rejectAddRequest,
	}, nil)
}

func (s *Session) SetGroupLeave(ctx context.Context, groupID string, isDismiss bool) error {
	return s.Call(ctx, "set_group_leave", map[string]any{
		"group_id":   groupID,
		"is_dismiss": isDismiss,
	}, nil)
}

func (s *Session) SetGroupBan(ctx context.Context, groupID string, userID string, duration int) error {
	return s.Call(ctx, "set_group_ban", map[string]any{
		"group_id": groupID,
		"user_id":  userID,
		"duration": duration, // in seconds, 0 to unban
	}, nil)
}

func (s *Session) SetGroupWholeBan(ctx context.Context, groupID string, enable bool) error {
	return s.Call(ctx, "set_group_whole_ban", map[string]any{
		"group_id": groupID,
		"enable":   enable,
	}, nil)
}

func (s *Session) SetGroupAdmin(ctx context.Context, groupID string, userID string, enable bool) error {
	return s.Call(ctx, "set_group_admin", map[string]any{
		"group_id": groupID,
		"user_id":  userID,
		"enable":   enable,
	}, nil)
}

func (s *Session) SendGroupNotice(ctx context.Context, groupID string, content string) error {
	return s.Call(ctx, "_send_group_notice", map[string]any{
		"group_id": groupID,
		"content":  content,
	}, nil)
}

func (s *Session) SetMessageEmojiLike(ctx context.Context, messageID string, emojiID string, set bool) error {
	return s.Call(ctx, "set_msg_emoji_like", map[string]any{
		"message_id": messageID,
		"emoji_id":   emojiID,
		"set":        set,
	}, nil)
}

func (s *Session) SetFriendAddRequest(ctx context.Context, flag string, approve bool, remark string) error {
	return s.Call(ctx, "set_friend_add_request", map[string]any{
		"flag":    flag,
		"approve": approve,
		"remark":  remark,
	}, nil)
}

func (s *Session) SetGroupAddRequest(ctx context.Context, flag string, subType string, approve bool, reason string) error {
	return s.Call(ctx, "set_group_add_request", map[string]any{
		"flag":     flag,
		"sub_type": subType,
		"approve":  approve,
		"reason":   reason,
	}, nil)
}

func (s *Session) SetQQProfile(ctx context.Context, nickname string, personalNote string, sex string) error {
	return s.Call(ctx, "set_qq_profile", map[string]any{
		"nickname":      nickname,
		"personal_note": personalNote,
		"sex":           sex,
	}, nil)
}

func (s *Session) SetQQAvatar(ctx context.Context, file string) error {
	return s.Call(ctx, "set_qq_avatar", map[string]any{
		"file": file,
	}, nil)
}

func (s *Session) GetFriendList(ctx context.Context) ([]FriendInfo, error) {
	var list []FriendInfo
	err := s.Call(ctx, "get_friend_list", nil, &list)
	return list, err
}

func (s *Session) GetGroupList(ctx context.Context) ([]GroupInfo, error) {
	var list []GroupInfo
	err := s.Call(ctx, "get_group_list", nil, &list)
	return list, err
}
