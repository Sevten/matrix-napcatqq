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
	NoticeType  string          `json:"notice_type"`
	MessageID   ID              `json:"message_id"`
	UserID      ID              `json:"user_id"`
	GroupID     ID              `json:"group_id"`
	OperatorID  ID              `json:"operator_id"`
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

type SendMessageResponse struct {
	MessageID ID `json:"message_id"`
}

type FileResponse struct {
	File string `json:"file"`
	URL  string `json:"url"`
}
