package connector

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/sevten/matrix-napcatqq/pkg/onebot"
	"github.com/sevten/matrix-napcatqq/pkg/qqid"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
)

var _ bridgev2.RedactionHandlingNetworkAPI = (*QQClient)(nil)
var _ bridgev2.ReactionHandlingNetworkAPI = (*QQClient)(nil)
var _ bridgev2.ReadReceiptHandlingNetworkAPI = (*QQClient)(nil)
var _ bridgev2.TypingHandlingNetworkAPI = (*QQClient)(nil)
var _ bridgev2.RoomNameHandlingNetworkAPI = (*QQClient)(nil)
var _ bridgev2.RoomAvatarHandlingNetworkAPI = (*QQClient)(nil)
var _ bridgev2.RoomTopicHandlingNetworkAPI = (*QQClient)(nil)
var _ bridgev2.MembershipHandlingNetworkAPI = (*QQClient)(nil)
var _ bridgev2.MuteHandlingNetworkAPI = (*QQClient)(nil)
var _ bridgev2.PowerLevelHandlingNetworkAPI = (*QQClient)(nil)

func (qc *QQClient) HandleMatrixMessage(ctx context.Context, msg *bridgev2.MatrixMessage) (*bridgev2.MatrixMessageResponse, error) {
	sess := qc.session()
	if sess == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}

	elements, err := qc.Main.MsgConv.ToQQ(ctx, msg.Event, msg.Content, msg.Portal)
	if err != nil {
		return nil, fmt.Errorf("failed to convert message: %w", err)
	}

	if msg.ReplyTo != nil {
		if msgID, err := qqid.ParseMessageID(msg.ReplyTo.ID); err != nil {
			return nil, err
		} else {
			elements = append([]onebot.Segment{onebot.Reply(msgID.ID)}, elements...)
		}
	}

	target := string(msg.Portal.ID)
	meta := msg.Portal.Metadata.(*qqid.PortalMetadata)
	var resp *onebot.SendMessageResponse
	switch meta.ChatType {
	case qqid.ChatPrivate:
		resp, err = sess.SendPrivateMessage(ctx, target, elements)
	case qqid.ChatGroup:
		resp, err = sess.SendGroupMessage(ctx, target, elements)
	default:
		return nil, fmt.Errorf("unknown chat type")
	}
	if err != nil {
		return nil, bridgev2.WrapErrorInStatus(err).WithSendNotice(true)
	}
	if resp == nil || resp.MessageID == "" {
		return nil, bridgev2.WrapErrorInStatus(fmt.Errorf("sent message returned empty response")).WithSendNotice(true)
	}

	now := time.Now()
	return &bridgev2.MatrixMessageResponse{
		DB: &database.Message{
			ID:        qqid.MakeMessageID(string(msg.Portal.ID), resp.MessageID.String()),
			SenderID:  qqid.MakeUserID(string(qc.UserLogin.ID)),
			Timestamp: now,
		},
		StreamOrder: now.Unix(),
	}, nil
}

func (qc *QQClient) HandleMatrixMessageRemove(ctx context.Context, msg *bridgev2.MatrixMessageRemove) error {
	sess := qc.session()
	if sess == nil {
		return bridgev2.ErrNotLoggedIn
	}
	if msg.TargetMessage == nil {
		return nil
	}
	parsed, err := qqid.ParseMessageID(msg.TargetMessage.ID)
	if err != nil {
		return err
	}
	return sess.DeleteMessage(ctx, parsed.ID)
}

func (qc *QQClient) PreHandleMatrixReaction(ctx context.Context, msg *bridgev2.MatrixReaction) (bridgev2.MatrixReactionPreResponse, error) {
	emoji := msg.Content.GetRelatesTo().GetAnnotationKey()
	emojiID := emojiToQQEmojiID(emoji)
	if emojiID == "" {
		return bridgev2.MatrixReactionPreResponse{}, fmt.Errorf("%w %s", bridgev2.ErrUnsupportedMessageType, "reaction "+emoji)
	}
	return bridgev2.MatrixReactionPreResponse{
		SenderID:     qqid.MakeUserID(string(qc.UserLogin.ID)),
		EmojiID:      networkid.EmojiID(emojiID),
		Emoji:        emoji,
		MaxReactions: 1,
	}, nil
}

func (qc *QQClient) HandleMatrixReaction(ctx context.Context, msg *bridgev2.MatrixReaction) (*database.Reaction, error) {
	sess := qc.session()
	if sess == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	if msg.TargetMessage == nil {
		return nil, nil
	}
	parsed, err := qqid.ParseMessageID(msg.TargetMessage.ID)
	if err != nil {
		return nil, err
	}
	var preResp bridgev2.MatrixReactionPreResponse
	if msg.PreHandleResp != nil {
		preResp = *msg.PreHandleResp
	}
	emojiID := string(preResp.EmojiID)
	if emojiID == "" {
		emojiID = emojiToQQEmojiID(msg.Content.GetRelatesTo().GetAnnotationKey())
	}
	if emojiID == "" {
		return nil, nil
	}
	if err = sess.SetMessageEmojiLike(ctx, parsed.ID, emojiID, true); err != nil {
		return nil, bridgev2.WrapErrorInStatus(err).WithSendNotice(false)
	}
	return &database.Reaction{EmojiID: networkid.EmojiID(emojiID), Emoji: preResp.Emoji}, nil
}

func (qc *QQClient) HandleMatrixReactionRemove(ctx context.Context, msg *bridgev2.MatrixReactionRemove) error {
	sess := qc.session()
	if sess == nil {
		return bridgev2.ErrNotLoggedIn
	}
	if msg.TargetReaction == nil {
		return nil
	}
	parsed, err := qqid.ParseMessageID(msg.TargetReaction.MessageID)
	if err != nil {
		return err
	}
	emojiID := string(msg.TargetReaction.EmojiID)
	if emojiID == "" {
		emojiID = emojiToQQEmojiID(msg.TargetReaction.Emoji)
	}
	if emojiID == "" {
		return nil
	}
	return sess.SetMessageEmojiLike(ctx, parsed.ID, emojiID, false)
}

func (qc *QQClient) HandleMatrixReadReceipt(ctx context.Context, msg *bridgev2.MatrixReadReceipt) error {
	sess := qc.session()
	if sess == nil {
		return bridgev2.ErrNotLoggedIn
	}
	if msg.ExactMessage != nil {
		if parsed, err := qqid.ParseMessageID(msg.ExactMessage.ID); err == nil {
			if err = sess.MarkMessageAsRead(ctx, parsed.ID); err == nil {
				return nil
			}
			qc.UserLogin.Log.Warn().Err(err).Str("message_id", parsed.ID).Msg("Failed to mark QQ message as read")
		}
	}
	if msg.Portal == nil {
		return nil
	}
	target := string(msg.Portal.ID)
	meta := msg.Portal.Metadata.(*qqid.PortalMetadata)
	var err error
	switch meta.ChatType {
	case qqid.ChatPrivate:
		err = sess.MarkPrivateMessageAsRead(ctx, target)
	case qqid.ChatGroup:
		err = sess.MarkGroupMessageAsRead(ctx, target)
	}
	if err != nil {
		qc.UserLogin.Log.Warn().Err(err).Str("portal_id", target).Msg("Failed to mark QQ chat as read")
	}
	return nil
}

func (qc *QQClient) HandleMatrixTyping(ctx context.Context, msg *bridgev2.MatrixTyping) error {
	sess := qc.session()
	if sess == nil {
		return bridgev2.ErrNotLoggedIn
	}
	if msg.Portal == nil {
		return nil
	}
	meta := msg.Portal.Metadata.(*qqid.PortalMetadata)
	if meta.ChatType != qqid.ChatPrivate {
		return nil
	}
	eventType := 0
	if msg.IsTyping {
		eventType = 1
	}
	if err := sess.SetInputStatus(ctx, string(msg.Portal.ID), eventType); err != nil {
		qc.UserLogin.Log.Warn().Err(err).Str("portal_id", string(msg.Portal.ID)).Msg("Failed to set QQ input status")
	}
	return nil
}

func (qc *QQClient) HandleMatrixRoomName(ctx context.Context, msg *bridgev2.MatrixRoomName) (bool, error) {
	sess := qc.session()
	if sess == nil {
		return false, bridgev2.ErrNotLoggedIn
	}
	target := string(msg.Portal.ID)
	meta := msg.Portal.Metadata.(*qqid.PortalMetadata)
	if meta.ChatType != qqid.ChatGroup {
		return false, nil
	}
	err := sess.SetGroupName(ctx, target, msg.Content.Name)
	if err != nil {
		return false, fmt.Errorf("failed to set group name: %w", err)
	}
	return true, nil
}

func (qc *QQClient) HandleMatrixRoomTopic(ctx context.Context, msg *bridgev2.MatrixRoomTopic) (bool, error) {
	sess := qc.session()
	if sess == nil {
		return false, bridgev2.ErrNotLoggedIn
	}
	target := string(msg.Portal.ID)
	meta := msg.Portal.Metadata.(*qqid.PortalMetadata)
	if meta.ChatType != qqid.ChatGroup {
		return false, nil
	}
	if msg.Content.Topic == "" {
		return false, nil
	}
	err := sess.SendGroupNotice(ctx, target, msg.Content.Topic)
	if err != nil {
		return false, fmt.Errorf("failed to send group notice: %w", err)
	}
	return true, nil
}

func (qc *QQClient) HandleMatrixRoomAvatar(ctx context.Context, msg *bridgev2.MatrixRoomAvatar) (bool, error) {
	sess := qc.session()
	if sess == nil {
		return false, bridgev2.ErrNotLoggedIn
	}
	target := string(msg.Portal.ID)
	meta := msg.Portal.Metadata.(*qqid.PortalMetadata)
	if meta.ChatType != qqid.ChatGroup {
		return false, nil
	}
	data, err := qc.Main.Bridge.Bot.DownloadMedia(ctx, msg.Content.URL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to download avatar: %w", err)
	}
	file := "base64://" + base64.StdEncoding.EncodeToString(data)
	err = sess.SetGroupPortrait(ctx, target, file)
	if err != nil {
		return false, fmt.Errorf("failed to set group avatar: %w", err)
	}
	return true, nil
}

func (qc *QQClient) HandleMatrixMembership(ctx context.Context, msg *bridgev2.MatrixMembershipChange) (*bridgev2.MatrixMembershipResult, error) {
	sess := qc.session()
	if sess == nil {
		return nil, bridgev2.ErrNotLoggedIn
	}
	target := string(msg.Portal.ID)
	meta := msg.Portal.Metadata.(*qqid.PortalMetadata)
	if meta.ChatType != qqid.ChatGroup {
		return nil, nil
	}

	var targetUserID string
	if msg.Target != nil {
		if ghost, ok := msg.Target.(*bridgev2.Ghost); ok {
			targetUserID = string(ghost.ID)
		} else if user, ok := msg.Target.(*bridgev2.UserLogin); ok {
			targetUserID = string(user.ID)
		}
	}

	if msg.Type.To == event.MembershipLeave {
		if msg.Type.IsSelf {
			err := sess.SetGroupLeave(ctx, target, false)
			if err != nil {
				return nil, fmt.Errorf("failed to leave group: %w", err)
			}
		} else if targetUserID != "" {
			err := sess.SetGroupKick(ctx, target, targetUserID, false)
			if err != nil {
				return nil, fmt.Errorf("failed to kick user: %w", err)
			}
		}
	} else if msg.Type.To == event.MembershipBan && targetUserID != "" {
		err := sess.SetGroupKick(ctx, target, targetUserID, true)
		if err != nil {
			return nil, fmt.Errorf("failed to ban user: %w", err)
		}
	}
	return nil, nil
}

func (qc *QQClient) HandleMatrixPowerLevels(ctx context.Context, msg *bridgev2.MatrixPowerLevelChange) (bool, error) {
	sess := qc.session()
	if sess == nil {
		return false, bridgev2.ErrNotLoggedIn
	}
	target := string(msg.Portal.ID)
	meta := msg.Portal.Metadata.(*qqid.PortalMetadata)
	if meta.ChatType != qqid.ChatGroup {
		return false, nil
	}

	handled := false
	for _, change := range msg.Users {
		targetUserID := ghostOrLoginID(change.Target)
		if targetUserID == "" || change.OrigLevel == change.NewLevel {
			continue
		}
		enable := change.NewLevel >= powerAdmin
		if (change.OrigLevel >= powerAdmin) == enable {
			continue
		}
		if err := sess.SetGroupAdmin(ctx, target, targetUserID, enable); err != nil {
			return handled, fmt.Errorf("failed to set group admin: %w", err)
		}
		handled = true
	}
	return handled, nil
}

func (qc *QQClient) HandleMute(ctx context.Context, msg *bridgev2.MatrixMute) error {
	sess := qc.session()
	if sess == nil {
		return bridgev2.ErrNotLoggedIn
	}
	target := string(msg.Portal.ID)
	meta := msg.Portal.Metadata.(*qqid.PortalMetadata)
	if meta.ChatType != qqid.ChatGroup {
		return nil
	}
	return sess.SetGroupWholeBan(ctx, target, msg.Content.IsMuted())
}

func ghostOrLoginID(target bridgev2.GhostOrUserLogin) string {
	switch val := target.(type) {
	case *bridgev2.Ghost:
		return string(val.ID)
	case *bridgev2.UserLogin:
		return string(val.ID)
	default:
		return ""
	}
}

func emojiToQQEmojiID(emoji string) string {
	switch emoji {
	case "\U0001f44d", "\U0001f44d\ufe0f", "+1":
		return "76"
	case "\U0001f44e", "\U0001f44e\ufe0f", "-1":
		return "77"
	case "\u2764\ufe0f", "\u2764", "\u2665\ufe0f", "\u2665":
		return "66"
	case "\U0001f602":
		return "124"
	case "\U0001f62e":
		return "0"
	case "\U0001f622":
		return "9"
	case "\U0001f621":
		return "11"
	default:
		return ""
	}
}
