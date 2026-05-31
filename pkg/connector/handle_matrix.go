package connector

import (
	"context"
	"fmt"
	"time"

	"github.com/sevten/matrix-napcatqq/pkg/onebot"
	"github.com/sevten/matrix-napcatqq/pkg/qqid"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
)

var _ bridgev2.RedactionHandlingNetworkAPI = (*QQClient)(nil)

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
