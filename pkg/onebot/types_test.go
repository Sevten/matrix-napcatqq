package onebot

import (
	"encoding/json"
	"testing"
)

func TestIDUnmarshalStringAndNumber(t *testing.T) {
	var got struct {
		A ID `json:"a"`
		B ID `json:"b"`
	}
	if err := json.Unmarshal([]byte(`{"a":12345,"b":"67890"}`), &got); err != nil {
		t.Fatal(err)
	}
	if got.A.String() != "12345" || got.B.String() != "67890" {
		t.Fatalf("unexpected IDs: %#v", got)
	}
}

func TestMessageUnmarshalString(t *testing.T) {
	var msg Message
	if err := json.Unmarshal([]byte(`"hello"`), &msg); err != nil {
		t.Fatal(err)
	}
	if len(msg) != 1 || msg[0].Type != "text" || msg[0].Data["text"] != "hello" {
		t.Fatalf("unexpected message: %#v", msg)
	}
}

func TestEventUnmarshalRequestAndEmojiLike(t *testing.T) {
	var evt Event
	if err := json.Unmarshal([]byte(`{
		"time": 1710000000,
		"self_id": 12345,
		"post_type": "request",
		"request_type": "group",
		"sub_type": "invite",
		"group_id": 67890,
		"user_id": "112233",
		"flag": "req-flag",
		"comment": "please"
	}`), &evt); err != nil {
		t.Fatal(err)
	}
	if evt.RequestType != "group" || evt.SubType != "invite" || evt.Flag.String() != "req-flag" {
		t.Fatalf("unexpected request event: %#v", evt)
	}

	if err := json.Unmarshal([]byte(`{
		"post_type": "notice",
		"notice_type": "group_msg_emoji_like",
		"group_id": 67890,
		"message_id": 445566,
		"user_id": 112233,
		"emoji_id": "76",
		"count": 1
	}`), &evt); err != nil {
		t.Fatal(err)
	}
	if evt.NoticeType != "group_msg_emoji_like" || evt.MessageID.String() != "445566" || evt.EmojiID.String() != "76" {
		t.Fatalf("unexpected emoji event: %#v", evt)
	}
}

func TestHistoryAndRecentContactUnmarshal(t *testing.T) {
	var history MessageHistoryResponse
	if err := json.Unmarshal([]byte(`{"messages":[{"message_id":1,"message_seq":"2","message":"hello"}]}`), &history); err != nil {
		t.Fatal(err)
	}
	if len(history.Messages) != 1 || history.Messages[0].BestMessageSeq().String() != "2" || len(history.Messages[0].Message) != 1 {
		t.Fatalf("unexpected history: %#v", history)
	}

	var contacts []RecentContact
	if err := json.Unmarshal([]byte(`[{"peerUin":"123","chatType":1,"lastestMsg":{"message_id":9,"message":"hi"}}]`), &contacts); err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 || contacts[0].PeerUin.String() != "123" || contacts[0].LatestMessage().BestMessageID().String() != "9" {
		t.Fatalf("unexpected contacts: %#v", contacts)
	}
}
