package connector

import (
	"context"
	"time"

	"github.com/duo/matrix-qq/pkg/msgconv"
	"github.com/duo/matrix-qq/pkg/onebot"

	"maunium.net/go/mautrix/bridgev2"
)

var (
	_ bridgev2.NetworkConnector      = (*QQConnector)(nil)
	_ bridgev2.MaxFileSizeingNetwork = (*QQConnector)(nil)
	_ bridgev2.StoppableNetwork      = (*QQConnector)(nil)
)

type QQConnector struct {
	Bridge  *bridgev2.Bridge
	Config  Config
	MsgConv *msgconv.MessageConverter
	OneBot  *onebot.Server
}

func (qc *QQConnector) Init(bridge *bridgev2.Bridge) {
	qc.Bridge = bridge
	qc.MsgConv = msgconv.NewMessageConverter(bridge)
}

func (qc *QQConnector) Start(ctx context.Context) error {
	qc.OneBot = onebot.NewServer(
		qc.Bridge.Log.With().Str("component", "onebot").Logger(),
		onebot.Config{
			ListenAddress:  qc.Config.NapCat.ListenAddress,
			WebSocketPath:  qc.Config.NapCat.WebSocketPath,
			AccessToken:    qc.Config.NapCat.AccessToken,
			RequestTimeout: time.Duration(qc.Config.NapCat.RequestTimeout) * time.Second,
		},
		qc.handleOneBotEvent,
	)
	return qc.OneBot.Start(ctx)
}

func (qc *QQConnector) Stop() {
	if qc.OneBot != nil {
		qc.OneBot.Stop()
	}
}

func (qc *QQConnector) SetMaxFileSize(maxSize int64) {
	qc.MsgConv.MaxFileSize = maxSize
}

func (qc *QQConnector) GetName() bridgev2.BridgeName {
	return bridgev2.BridgeName{
		DisplayName:      "Matrix QQ",
		NetworkURL:       "https://github.com/duo/matrix-qq",
		NetworkIcon:      "mxc://matrix.org/nKrjlWVnjIGQRJicsBqDFLnc",
		NetworkID:        "qq",
		BeeperBridgeType: "github.com/duo/matrix-qq",
		DefaultPort:      17777,
	}
}

func (qc *QQConnector) LoadUserLogin(ctx context.Context, login *bridgev2.UserLogin) error {
	_ = ctx
	q := &QQClient{
		Main:        qc,
		UserLogin:   login,
		resyncQueue: make(map[string]resyncQueueItem),
	}
	login.Client = q
	return nil
}
