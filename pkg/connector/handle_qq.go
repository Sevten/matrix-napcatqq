package connector

import (
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/sevten/matrix-napcatqq/pkg/onebot"
	"github.com/sevten/matrix-napcatqq/pkg/qqid"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/bridgev2/simplevent"
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
	case "request":
		client.handleOneBotRequest(evt)
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
	if evt.NoticeType == "group_msg_emoji_like" {
		qc.handleOneBotEmojiLike(evt)
		return
	}
	if evt.NoticeType == "notify" && evt.SubType == "input_status" {
		qc.handleOneBotInputStatus(evt)
		return
	}

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
		evt.NoticeType == "group_upload" || evt.NoticeType == "group_card" ||
		evt.NoticeType == "essence" || evt.NoticeType == "notify" {

		chatID, chatType := qc.chatFromEvent(evt)
		senderID := evt.UserID.String()
		if senderID == "" {
			senderID = string(qc.UserLogin.ID)
		}

		qc.Main.Bridge.QueueRemoteEvent(qc.UserLogin, &QQNoticeEvent{
			Message: &qqid.Message{
				ID:        noticeEventID(evt),
				Timestamp: eventTimestamp(evt),
				Type:      qqid.MsgNotice,
				ChatID:    chatID,
				ChatType:  chatType,
				SenderID:  senderID,
				Elements:  []onebot.Segment{onebot.Text(formatNoticeText(evt))},
			},
			NoticeType: evt.NoticeType,
			qc:         qc,
		})
	}
}

func (qc *QQClient) handleOneBotInputStatus(evt *onebot.Event) {
	chatID, chatType := qc.chatFromEvent(evt)
	if chatID == "" || chatType != qqid.ChatPrivate {
		return
	}
	senderID := evt.UserID.String()
	if senderID == "" {
		senderID = evt.TargetID.String()
	}
	if senderID == "" {
		return
	}
	qc.Main.Bridge.QueueRemoteEvent(qc.UserLogin, &simplevent.Typing{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventTyping,
			PortalKey: qc.makePortalKey(chatType, chatID),
			Sender:    qc.makeEventSender(senderID),
			Timestamp: time.UnixMilli(eventTimestamp(evt)),
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("notice_type", "input_status").Str("sender_id", senderID)
			},
		},
		Timeout: 5 * time.Second,
		Type:    bridgev2.TypingTypeText,
	})
}

func (qc *QQClient) handleOneBotEmojiLike(evt *onebot.Event) {
	if evt.MessageID == "" {
		return
	}
	chatID, chatType := qc.chatFromEvent(evt)
	emoji := evt.Emoji
	if emoji == "" {
		emoji = qqEmojiIDToEmoji(evt.EmojiID.String())
	}
	if emoji == "" {
		emoji = evt.EmojiName
	}
	if emoji == "" {
		emoji = fmt.Sprintf("[QQ emoji %s]", evt.EmojiID.String())
	}
	senderID := evt.UserID.String()
	if senderID == "" {
		senderID = evt.OperatorID.String()
	}
	if senderID == "" {
		senderID = string(qc.UserLogin.ID)
	}

	qc.Main.Bridge.QueueRemoteEvent(qc.UserLogin, &simplevent.Reaction{
		EventMeta: simplevent.EventMeta{
			Type:      bridgev2.RemoteEventReaction,
			PortalKey: qc.makePortalKey(chatType, chatID),
			Sender:    qc.makeEventSender(senderID),
			Timestamp: time.UnixMilli(eventTimestamp(evt)),
			LogContext: func(c zerolog.Context) zerolog.Context {
				return c.Str("message_id", evt.MessageID.String()).Str("emoji_id", evt.EmojiID.String())
			},
		},
		TargetMessage: qqid.MakeMessageID(chatID, evt.MessageID.String()),
		EmojiID:       networkid.EmojiID(evt.EmojiID.String()),
		Emoji:         emoji,
	})
}

func (qc *QQClient) handleOneBotRequest(evt *onebot.Event) {
	chatID, chatType := qc.chatFromEvent(evt)
	if evt.RequestType == "group" && evt.GroupID != "" {
		chatID = evt.GroupID.String()
		chatType = qqid.ChatGroup
	}
	body := formatRequestNotice(evt)
	if body == "" {
		return
	}
	senderID := evt.UserID.String()
	if senderID == "" {
		senderID = string(qc.UserLogin.ID)
	}
	qc.Main.Bridge.QueueRemoteEvent(qc.UserLogin, &QQMessageEvent{
		Message: &qqid.Message{
			ID:        fmt.Sprintf("request-%s-%d", evt.Flag.String(), evt.Time),
			Timestamp: eventTimestamp(evt),
			Type:      qqid.MsgText,
			ChatID:    chatID,
			ChatType:  chatType,
			SenderID:  senderID,
			Elements:  []onebot.Segment{onebot.Text(body)},
		},
		qc: qc,
	})
}

func formatRequestNotice(evt *onebot.Event) string {
	switch evt.RequestType {
	case "friend":
		return fmt.Sprintf("QQ friend request from %s\nFlag: %s\nComment: %s", evt.UserID.String(), evt.Flag.String(), evt.Comment)
	case "group":
		action := "group request"
		if evt.SubType == "invite" {
			action = "group invitation"
		} else if evt.SubType == "add" {
			action = "group join request"
		}
		return fmt.Sprintf("QQ %s in group %s from %s\nFlag: %s\nComment: %s", action, evt.GroupID.String(), evt.UserID.String(), evt.Flag.String(), evt.Comment)
	default:
		return ""
	}
}

func formatNoticeText(evt *onebot.Event) string {
	switch evt.NoticeType {
	case "group_increase":
		return fmt.Sprintf("QQ group member joined: %s", evt.UserID.String())
	case "group_decrease":
		return fmt.Sprintf("QQ group member left: %s", evt.UserID.String())
	case "group_admin":
		if evt.SubType == "set" {
			return fmt.Sprintf("QQ group admin added: %s", evt.UserID.String())
		}
		return fmt.Sprintf("QQ group admin removed: %s", evt.UserID.String())
	case "group_ban":
		if evt.SubType == "lift_ban" || evt.Duration == 0 {
			return fmt.Sprintf("QQ group member unmuted: %s", evt.UserID.String())
		}
		return fmt.Sprintf("QQ group member muted: %s for %d seconds", evt.UserID.String(), evt.Duration)
	case "group_upload":
		if evt.File != nil && evt.File.Name != "" {
			return fmt.Sprintf("QQ group file uploaded: %s", evt.File.Name)
		}
		return "QQ group file uploaded"
	case "group_card":
		return fmt.Sprintf("QQ group card updated: %s", evt.UserID.String())
	case "essence":
		if evt.SubType == "delete" {
			return fmt.Sprintf("QQ essence message removed: %s", evt.MessageID.String())
		}
		return fmt.Sprintf("QQ essence message added: %s", evt.MessageID.String())
	case "notify":
		switch evt.SubType {
		case "poke":
			return fmt.Sprintf("QQ poke: %s -> %s", evt.UserID.String(), evt.TargetID.String())
		case "title":
			return fmt.Sprintf("QQ group title updated: %s", evt.UserID.String())
		case "profile_like":
			return fmt.Sprintf("QQ profile like: %s", evt.UserID.String())
		default:
			return fmt.Sprintf("QQ notify: %s", evt.SubType)
		}
	default:
		return fmt.Sprintf("QQ notice: %s", evt.NoticeType)
	}
}

func noticeEventID(evt *onebot.Event) string {
	parts := []string{
		evt.NoticeType,
		evt.SubType,
		evt.MessageID.String(),
		evt.UserID.String(),
		evt.OperatorID.String(),
		evt.TargetID.String(),
		fmt.Sprintf("%d", evt.Time),
	}
	return strings.Join(parts, "-")
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

func qqEmojiIDToEmoji(emojiID string) string {
	switch emojiID {
	case "76":
		return "\U0001f44d"
	case "77":
		return "\U0001f44e"
	case "66":
		return "\u2764\ufe0f"
	case "124":
		return "\U0001f602"
	case "9":
		return "\U0001f622"
	case "11":
		return "\U0001f621"
	default:
		return ""
	}
}
