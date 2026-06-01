package connector

import (
	"fmt"
	"time"

	"github.com/sevten/matrix-napcatqq/pkg/onebot"
	"github.com/sevten/matrix-napcatqq/pkg/qqid"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

func (qc *QQConnector) handleOneBotEvent(sess *onebot.Session, evt *onebot.Event) {
	if evt.PostType == "meta_event" {
		return
	}
	selfID := sess.SelfID()
	if selfID == "" {
		selfID = evt.SelfID.String()
	}
	login := qc.Bridge.GetCachedUserLoginByID(networkid.UserLoginID(selfID))
	if login == nil {
		qc.Bridge.Log.Trace().
			Str("self_id", selfID).
			Str("post_type", evt.PostType).
			Msg("Dropping OneBot event for unbound NapCatQQ account")
		return
	}
	client, ok := login.Client.(*QQClient)
	if !ok || client == nil {
		return
	}

	switch evt.PostType {
	case "message", "message_sent":
		client.handleOneBotMessage(evt)
	case "notice":
		client.handleOneBotNotice(evt)
	}
}

func (qc *QQClient) handleOneBotMessage(evt *onebot.Event) {
	qc.UserLogin.Log.Trace().Any("event", evt).Msg("Receive QQ message")
	if len(evt.Message) == 0 {
		qc.UserLogin.Log.Warn().Any("event", evt).Msg("Receive empty QQ message")
		return
	}

	chatID, chatType := qc.chatFromEvent(evt)
	senderID := evt.Sender.UserID.String()
	if senderID == "" {
		senderID = evt.UserID.String()
	}
	if evt.PostType == "message_sent" && senderID == "" {
		senderID = string(qc.UserLogin.ID)
	}

	qc.Main.Bridge.QueueRemoteEvent(qc.UserLogin, &QQMessageEvent{
		Message: &qqid.Message{
			ID:        evt.MessageID.String(),
			Timestamp: eventTimestamp(evt),
			Type:      qqid.ParseMessageType(evt.Message),
			ChatID:    chatID,
			ChatType:  chatType,
			SenderID:  senderID,
			Elements:  evt.Message,
		},
		qc: qc,
	})
}

func (qc *QQClient) handleOneBotNotice(evt *onebot.Event) {
	if evt.NoticeType == "friend_recall" || evt.NoticeType == "group_recall" {
		chatID, chatType := qc.chatFromEvent(evt)
		senderID := evt.OperatorID.String()
		if senderID == "" {
			senderID = evt.UserID.String()
		}
		if senderID == "" {
			senderID = string(qc.UserLogin.ID)
		}

		qc.Main.Bridge.QueueRemoteEvent(qc.UserLogin, &QQMessageEvent{
			Message: &qqid.Message{
				ID:        evt.MessageID.String(),
				Timestamp: eventTimestamp(evt),
				Type:      qqid.MsgRevoke,
				ChatID:    chatID,
				ChatType:  chatType,
				SenderID:  senderID,
				Elements:  nil,
			},
			qc: qc,
		})
		return
	}

	if evt.NoticeType == "group_increase" || evt.NoticeType == "group_decrease" ||
		evt.NoticeType == "group_admin" || evt.NoticeType == "group_ban" ||
		evt.NoticeType == "group_upload" {
		
		chatID, chatType := qc.chatFromEvent(evt)
		senderID := evt.UserID.String()
		if senderID == "" {
			senderID = string(qc.UserLogin.ID)
		}

		qc.Main.Bridge.QueueRemoteEvent(qc.UserLogin, &QQNoticeEvent{
			Message: &qqid.Message{
				ID:        fmt.Sprintf("%d", evt.Time),
				Timestamp: eventTimestamp(evt),
				Type:      qqid.MsgNotice,
				ChatID:    chatID,
				ChatType:  chatType,
				SenderID:  senderID,
				Elements:  nil,
			},
			NoticeType: evt.NoticeType,
			qc:         qc,
		})
	}
}

func (qc *QQClient) chatFromEvent(evt *onebot.Event) (string, qqid.ChatType) {
	if evt.GroupID != "" || evt.MessageType == "group" {
		return evt.GroupID.String(), qqid.ChatGroup
	}
	chatID := evt.UserID.String()
	if chatID == "" && evt.Sender.UserID != "" {
		chatID = evt.Sender.UserID.String()
	}
	if chatID == "" {
		chatID = fmt.Sprint(qc.UserLogin.ID)
	}
	return chatID, qqid.ChatPrivate
}

func eventTimestamp(evt *onebot.Event) int64 {
	if evt.Time == 0 {
		return time.Now().UnixMilli()
	}
	return evt.Time * 1000
}
