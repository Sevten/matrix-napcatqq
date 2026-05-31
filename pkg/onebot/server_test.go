package onebot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

func TestReverseWebSocketRegistersSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	srv := NewServer(zerolog.Nop(), Config{
		AccessToken:    "secret",
		RequestTimeout: time.Second,
	}, nil)
	httpSrv := httptest.NewServer(http.HandlerFunc(srv.handleWebSocket))
	defer httpSrv.Close()

	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "?access_token=secret"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		Action string `json:"action"`
		Echo   string `json:"echo"`
	}
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	if req.Action != "get_login_info" {
		t.Fatalf("unexpected action %q", req.Action)
	}
	if err := conn.WriteJSON(map[string]any{
		"status":  "ok",
		"retcode": 0,
		"echo":    req.Echo,
		"data": map[string]any{
			"user_id":  123456,
			"nickname": "tester",
		},
	}); err != nil {
		t.Fatal(err)
	}

	for {
		if sess := srv.GetSession("123456"); sess != nil {
			if sess.Nickname() != "tester" {
				t.Fatalf("unexpected nickname %q", sess.Nickname())
			}
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("session was not registered")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestReverseWebSocketRejectsBadToken(t *testing.T) {
	srv := NewServer(zerolog.Nop(), Config{AccessToken: "secret"}, nil)
	httpSrv := httptest.NewServer(http.HandlerFunc(srv.handleWebSocket))
	defer httpSrv.Close()

	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "?access_token=wrong"
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected dial to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
