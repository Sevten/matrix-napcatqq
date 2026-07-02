package connector

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/rs/zerolog"
	"github.com/sevten/matrix-napcatqq/pkg/msgconv"
	"github.com/sevten/matrix-napcatqq/pkg/onebot"
	"github.com/sevten/matrix-napcatqq/pkg/qqid"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
	"maunium.net/go/mautrix/event"
)

const DefaultHistoryBackfillLimit = 50

var _ bridgev2.BackfillingNetworkAPI = (*QQClient)(nil)

func (qc *QQClient) syncAllContacts(ctx context.Context) {
	sess := qc.session()
	if sess == nil {
		return
	}
	
	// Sync recent contacts (gets latest messages)
	contacts, err := sess.GetRecentContacts(ctx, DefaultHistoryBackfillLimit)
	if err != nil {
		qc.UserLogin.Log.Warn().Err(err).Msg("Failed to fetch recent QQ contacts")
	} else {
		for _, contact := range contacts {
			chatID, chatType := contactToChat(contact)
			if chatID == "" || chatType == qqid.ChatUnknown {
				continue
			}
			latestTS := contactLatestTime(contact)
			qc.Main.Bridge.QueueRemoteEvent(qc.UserLogin, &simplevent.ChatResync{
				EventMeta: simplevent.EventMeta{
					Type:         bridgev2.RemoteEventChatResync,
					PortalKey:    qc.makePortalKey(chatType, chatID),
					CreatePortal: true,
					LogContext: func(c zerolog.Context) zerolog.Context {
						return c.Str("sync_reason", "recent_contacts")
					},
				},
				GetChatInfoFunc: func(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
					return qc.GetChatInfo(ctx, portal)
				},
				LatestMessageTS: latestTS,
			})
		}
	}

	// Sync all friends
	friends, err := sess.GetFriendList(ctx)
	if err != nil {
		qc.UserLogin.Log.Warn().Err(err).Msg("Failed to fetch QQ friend list")
	} else {
		for _, f := range friends {
			qc.Main.Bridge.QueueRemoteEvent(qc.UserLogin, &simplevent.ChatResync{
				EventMeta: simplevent.EventMeta{
					Type:         bridgev2.RemoteEventChatResync,
					PortalKey:    qc.makePortalKey(qqid.ChatPrivate, f.UserID.String()),
					CreatePortal: true,
					LogContext: func(c zerolog.Context) zerolog.Context {
						return c.Str("sync_reason", "friend_list")
					},
				},
				GetChatInfoFunc: func(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
					return qc.GetChatInfo(ctx, portal)
				},
			})
		}
	}

	// Sync all groups
	groups, err := sess.GetGroupList(ctx)
	if err != nil {
		qc.UserLogin.Log.Warn().Err(err).Msg("Failed to fetch QQ group list")
	} else {
		for _, g := range groups {
			qc.Main.Bridge.QueueRemoteEvent(qc.UserLogin, &simplevent.ChatResync{
				EventMeta: simplevent.EventMeta{
					Type:         bridgev2.RemoteEventChatResync,
					PortalKey:    qc.makePortalKey(qqid.ChatGroup, g.GroupID.String()),
					CreatePortal: true,
					LogContext: func(c zerolog.Context) zerolog.Context {
						return c.Str("sync_reason", "group_list")
					},
				},
				GetChatInfoFunc: func(ctx context.Context, portal *bridgev2.Portal) (*bridgev2.ChatInfo, error) {
					return qc.GetChatInfo(ctx, portal)
				},
			})
		}
	}
}

func (qc *QQClient) FetchMessages(ctx context.Context, params bridgev2.FetchMessagesParams) (*bridgev2.FetchMessagesResponse, error) {
	sess := qc.session()
	if sess == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	if params.Portal == nil {
		return &bridgev2.FetchMessagesResponse{HasMore: false}, nil
	}
	meta := params.Portal.Metadata.(*qqid.PortalMetadata)
	count := params.Count
	if count <= 0 || count > DefaultHistoryBackfillLimit {
		count = DefaultHistoryBackfillLimit
	}
	messageSeq := string(params.Cursor)
	if messageSeq == "" && params.AnchorMessage != nil {
		if parsed, err := qqid.ParseMessageID(params.AnchorMessage.ID); err == nil {
			messageSeq = parsed.ID
		}
	}

	var history *onebot.MessageHistoryResponse
	var err error
	switch meta.ChatType {
	case qqid.ChatPrivate:
		history, err = sess.GetFriendMessageHistory(ctx, string(params.Portal.ID), messageSeq, count, false)
	case qqid.ChatGroup:
		history, err = sess.GetGroupMessageHistory(ctx, string(params.Portal.ID), messageSeq, count, false)
	default:
		return &bridgev2.FetchMessagesResponse{HasMore: false}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch QQ message history: %w", err)
	}
	messages := historyToBackfill(qc, params.Portal, history.Messages)
	nextCursor := ""
	if len(history.Messages) > 0 {
		nextCursor = history.Messages[0].BestMessageSeq().String()
	}
	return &bridgev2.FetchMessagesResponse{
		Messages:                messages,
		Cursor:                  networkid.PaginationCursor(nextCursor),
		HasMore:                 len(messages) >= count,
		Forward:                 params.Forward,
		MarkRead:                params.Forward,
		AggressiveDeduplication: true,
	}, nil
}

func historyToBackfill(qc *QQClient, portal *bridgev2.Portal, details []onebot.MessageDetail) []*bridgev2.BackfillMessage {
	sort.SliceStable(details, func(i, j int) bool {
		return details[i].Time < details[j].Time
	})
	out := make([]*bridgev2.BackfillMessage, 0, len(details))
	for _, detail := range details {
		messageID := detail.BestMessageID().String()
		if messageID == "" {
			continue
		}
		chatID := string(portal.ID)
		senderID := detail.Sender.UserID.String()
		if senderID == "" {
			senderID = detail.UserID.String()
		}
		if senderID == "" {
			senderID = string(qc.UserLogin.ID)
		}
		timestamp := time.Unix(detail.Time, 0)
		if detail.Time == 0 {
			timestamp = time.Now()
		}
		body := msgconv.ToPlainText(detail.Message)
		if body == "" {
			body = detail.RawMessage
		}
		out = append(out, &bridgev2.BackfillMessage{
			ConvertedMessage: &bridgev2.ConvertedMessage{
				Parts: []*bridgev2.ConvertedMessagePart{{
					Type: event.EventMessage,
					Content: &event.MessageEventContent{
						MsgType: event.MsgText,
						Body:    body,
					},
				}},
			},
			Sender:      qc.makeEventSender(senderID),
			ID:          qqid.MakeMessageID(chatID, messageID),
			Timestamp:   timestamp,
			StreamOrder: timestamp.Unix(),
		})
	}
	return out
}

func contactToChat(contact onebot.RecentContact) (string, qqid.ChatType) {
	latest := contact.LatestMessage()
	if latest.GroupID != "" || latest.MessageType == "group" || contact.ChatType == 2 {
		if latest.GroupID != "" {
			return latest.GroupID.String(), qqid.ChatGroup
		}
		return contact.PeerUin.String(), qqid.ChatGroup
	}
	if latest.UserID != "" {
		return latest.UserID.String(), qqid.ChatPrivate
	}
	if contact.PeerUin != "" {
		return contact.PeerUin.String(), qqid.ChatPrivate
	}
	return "", qqid.ChatUnknown
}

func contactLatestTime(contact onebot.RecentContact) time.Time {
	latest := contact.LatestMessage()
	if latest.Time != 0 {
		return time.Unix(latest.Time, 0)
	}
	if contact.MsgTime != 0 {
		return time.Unix(contact.MsgTime, 0)
	}
	return time.Time{}
}
