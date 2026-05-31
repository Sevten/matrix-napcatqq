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
