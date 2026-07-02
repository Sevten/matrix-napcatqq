package onebot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

type ID string

func (id ID) String() string {
	return string(id)
}

func (id ID) Uint64() uint64 {
	v, _ := strconv.ParseUint(string(id), 10, 64)
	return v
}

func (id *ID) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		*id = ""
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*id = ID(s)
		return nil
	}
	var num json.Number
	if err := json.Unmarshal(data, &num); err != nil {
		return err
	}
	*id = ID(num.String())
	return nil
}

func AnyID(v any) ID {
	switch val := v.(type) {
	case string:
		return ID(val)
	case float64:
		return ID(strconv.FormatInt(int64(val), 10))
	case int64:
		return ID(strconv.FormatInt(val, 10))
	case uint64:
		return ID(strconv.FormatUint(val, 10))
	case int:
		return ID(strconv.Itoa(val))
	case json.Number:
		return ID(val.String())
	default:
		return ID(fmt.Sprint(val))
	}
}

type Segment struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data,omitempty"`
}

func NewSegment(typ string, data map[string]any) Segment {
	if data == nil {
		data = map[string]any{}
	}
	return Segment{Type: typ, Data: data}
}

func Text(text string) Segment {
	return NewSegment("text", map[string]any{"text": text})
}

func At(id string) Segment {
	return NewSegment("at", map[string]any{"qq": id})
}

func Reply(id string) Segment {
	return NewSegment("reply", map[string]any{"id": id})
}

type Message []Segment

func (m *Message) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		*m = Message{Text(text)}
		return nil
	}
	var segments []Segment
	if err := json.Unmarshal(data, &segments); err != nil {
		return err
	}
	*m = segments
	return nil
}

type Sender struct {
	UserID   ID     `json:"user_id"`
	Nickname string `json:"nickname"`
	Card     string `json:"card"`
	Role     string `json:"role"`
}

type Event struct {
	Time        int64           `json:"time"`
	SelfID      ID              `json:"self_id"`
	PostType    string          `json:"post_type"`
	MessageType string          `json:"message_type"`
	SubType     string          `json:"sub_type"`
	RequestType string          `json:"request_type"`
	NoticeType  string          `json:"notice_type"`
	MessageID   ID              `json:"message_id"`
	UserID      ID              `json:"user_id"`
	GroupID     ID              `json:"group_id"`
	OperatorID  ID              `json:"operator_id"`
	TargetID    ID              `json:"target_id"`
	Flag        ID              `json:"flag"`
	Comment     string          `json:"comment"`
	Duration    int             `json:"duration"`
	HonorType   string          `json:"honor_type"`
	EmojiID     ID              `json:"emoji_id"`
	EmojiName   string          `json:"emoji_name"`
	Emoji       string          `json:"emoji"`
	Count       int             `json:"count"`
	EventType   int             `json:"event_type"`
	File        *EventFile      `json:"file"`
	Message     Message         `json:"message"`
	RawMessage  string          `json:"raw_message"`
	Sender      Sender          `json:"sender"`
	Raw         json.RawMessage `json:"-"`
}

func (evt *Event) UnmarshalJSON(data []byte) error {
	type rawEvent Event
	var raw rawEvent
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*evt = Event(raw)
	evt.Raw = append(evt.Raw[:0], data...)
	return nil
}

type LoginInfo struct {
	UserID   ID     `json:"user_id"`
	Nickname string `json:"nickname"`
}

type GroupInfo struct {
	GroupID        ID     `json:"group_id"`
	GroupName      string `json:"group_name"`
	MemberCount    int    `json:"member_count"`
	MaxMemberCount int    `json:"max_member_count"`
}

type GroupMember struct {
	GroupID  ID     `json:"group_id"`
	UserID   ID     `json:"user_id"`
	Nickname string `json:"nickname"`
	Card     string `json:"card"`
	Role     string `json:"role"`
}

type StrangerInfo struct {
	UserID   ID     `json:"user_id"`
	Nickname string `json:"nickname"`
}

type FriendInfo struct {
	UserID   ID     `json:"user_id"`
	Nickname string `json:"nickname"`
	Remark   string `json:"remark"`
}


type EventFile struct {
	ID    ID     `json:"id"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	BusID ID     `json:"busid"`
	URL   string `json:"url"`
}

type SendMessageResponse struct {
	MessageID ID `json:"message_id"`
}

type FileResponse struct {
	File string `json:"file"`
	URL  string `json:"url"`
}

type MessageDetail struct {
	Time        int64   `json:"time"`
	SelfID      ID      `json:"self_id"`
	MessageType string  `json:"message_type"`
	SubType     string  `json:"sub_type"`
	MessageID   ID      `json:"message_id"`
	RealID      ID      `json:"real_id"`
	MessageSeq  ID      `json:"message_seq"`
	UserID      ID      `json:"user_id"`
	GroupID     ID      `json:"group_id"`
	Sender      Sender  `json:"sender"`
	Message     Message `json:"message"`
	RawMessage  string  `json:"raw_message"`
}

func (msg MessageDetail) BestMessageID() ID {
	if msg.MessageID != "" {
		return msg.MessageID
	}
	if msg.RealID != "" {
		return msg.RealID
	}
	return msg.MessageSeq
}

func (msg MessageDetail) BestMessageSeq() ID {
	if msg.MessageSeq != "" {
		return msg.MessageSeq
	}
	return msg.BestMessageID()
}

type MessageHistoryResponse struct {
	Messages []MessageDetail `json:"messages"`
}

type ForwardMessageResponse struct {
	Messages []ForwardMessageNode `json:"messages"`
}

type ForwardMessageNode struct {
	UserID   ID      `json:"user_id"`
	Nickname string  `json:"nickname"`
	Time     int64   `json:"time"`
	Content  Message `json:"content"`
	Message  Message `json:"message"`
	Sender   Sender  `json:"sender"`
}

func (node *ForwardMessageNode) Elements() Message {
	if len(node.Content) > 0 {
		return node.Content
	}
	return node.Message
}

type RecentContact struct {
	PeerUin   ID            `json:"peerUin"`
	ChatType  int           `json:"chatType"`
	MsgTime   int64         `json:"msgTime"`
	SendNick  string        `json:"sendNickName"`
	LastMsg   MessageDetail `json:"lastestMsg"`
	LastMsg2  MessageDetail `json:"latestMsg"`
	Remark    string        `json:"remark"`
	GroupName string        `json:"groupName"`
}

func (rc RecentContact) LatestMessage() MessageDetail {
	if rc.LastMsg.BestMessageID() != "" || len(rc.LastMsg.Message) > 0 {
		return rc.LastMsg
	}
	return rc.LastMsg2
}
