package connector

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sevten/matrix-napcatqq/pkg/onebot"
	"github.com/sevten/matrix-napcatqq/pkg/qqid"
	"maunium.net/go/mautrix/bridgev2/status"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
)

type resyncQueueItem struct {
	portal *bridgev2.Portal
	ghost  *bridgev2.Ghost
}

type QQClient struct {
	Main      *QQConnector
	UserLogin *bridgev2.UserLogin

	stopLoops       atomic.Pointer[context.CancelFunc]
	resyncQueue     map[string]resyncQueueItem
	resyncQueueLock sync.Mutex
	nextResync      time.Time
}

var (
	_ bridgev2.NetworkAPI                    = (*QQClient)(nil)
	_ bridgev2.IdentifierResolvingNetworkAPI = (*QQClient)(nil)
)

func (qc *QQClient) Connect(ctx context.Context) {
	if !qc.IsLoggedIn() {
		state := status.BridgeState{
			StateEvent: status.StateBadCredentials,
			Message:    "NapCatQQ is not connected for this QQ account",
		}
		qc.UserLogin.BridgeState.Send(state)
		return
	}

	qc.startLoops()
}

func (qc *QQClient) Disconnect() {
	// Stop sync
	if stopSyncLoop := qc.stopLoops.Swap(nil); stopSyncLoop != nil {
		(*stopSyncLoop)()
	}

}

func (qc *QQClient) LogoutRemote(ctx context.Context) {
	qc.Disconnect()

	qc.UserLogin.Metadata = &qqid.UserLoginMetadata{}
	qc.UserLogin.Save(ctx)
}

func (qc *QQClient) IsLoggedIn() bool {
	return qc.session() != nil
}

func (qc *QQClient) IsThisUser(ctx context.Context, userID networkid.UserID) bool {
	return networkid.UserLoginID(userID) == qc.UserLogin.ID
}

func (qc *QQClient) startLoops() {
	ctx, cancel := context.WithCancel(context.Background())
	oldStop := qc.stopLoops.Swap(&cancel)
	if oldStop != nil {
		(*oldStop)()
	}

	go qc.ghostResyncLoop(ctx)
}

func (qc *QQClient) session() *onebot.Session {
	if qc.Main.OneBot == nil {
		return nil
	}
	sess := qc.Main.OneBot.GetSession(string(qc.UserLogin.ID))
	if sess == nil || !sess.IsConnected() {
		return nil
	}
	return sess
}
