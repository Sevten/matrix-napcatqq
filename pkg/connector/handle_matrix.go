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
	"maunium.net/go/mautrix/event"
)

var _ bridgev2.RedactionHandlingNetworkAPI = (*QQClient)(nil)
var _ bridgev2.RoomNameHandlingNetworkAPI = (*QQClient)(nil)
var _ bridgev2.RoomAvatarHandlingNetworkAPI = (*QQClient)(nil)
var _ bridgev2.MembershipHandlingNetworkAPI = (*QQClient)(nil)

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

