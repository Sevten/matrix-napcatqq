package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/sevten/matrix-napcatqq/pkg/qqid"
	"go.mau.fi/util/jsontime"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
)

const (
	LoginStepNapCatQQ = "me.lxduo.qq.login.napcat"
	LoginStepComplete = "me.lxduo.qq.login.complete"
)

type NapCatLogin struct {
	User *bridgev2.User
	Main *QQConnector
}

var _ bridgev2.LoginProcessUserInput = (*NapCatLogin)(nil)

func (qc *QQConnector) GetLoginFlows() []bridgev2.LoginFlow {
	return []bridgev2.LoginFlow{{
		Name:        "NapCatQQ",
		Description: "Bind a QQ account that has connected to the bridge through NapCatQQ reverse WebSocket",
		ID:          "napcat",
	}}
}

func (qc *QQConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	if flowID != "napcat" {
		return nil, fmt.Errorf("invalid login flow ID")
	}
	return &NapCatLogin{User: user, Main: qc}, nil
}

func (nl *NapCatLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeUserInput,
		StepID:       LoginStepNapCatQQ,
		Instructions: "Configure NapCatQQ to connect to the bridge reverse WebSocket, then enter the QQ number to bind.",
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{{
				Type:        bridgev2.LoginInputFieldTypeUsername,
				ID:          "qq",
				Name:        "QQ number",
				Description: "The QQ number shown by the connected NapCatQQ instance",
				Pattern:     `^\d+$`,
			}},
		},
	}, nil
}

func (nl *NapCatLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	qq := strings.TrimSpace(input["qq"])
	if qq == "" {
		return nil, fmt.Errorf("QQ number is required")
	}
	if nl.Main.OneBot == nil {
		return nil, fmt.Errorf("NapCat reverse WebSocket server is not running")
	}
	sess := nl.Main.OneBot.GetSession(qq)
	if sess == nil {
		return nil, fmt.Errorf("NapCatQQ account %s is not connected", qq)
	}
	info, err := sess.GetLoginInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get NapCat login info: %w", err)
	}
	if info.UserID.String() != qq {
		return nil, fmt.Errorf("connected NapCatQQ account mismatch: expected %s, got %s", qq, info.UserID.String())
	}

	ul, err := nl.User.NewLogin(ctx, &database.UserLogin{
		ID:         qqid.MakeUserLoginID(qq),
		RemoteName: info.Nickname,
		Metadata: &qqid.UserLoginMetadata{
			SelfID:      qq,
			Nickname:    info.Nickname,
			ConnectedAt: jsontime.UnixNow(),
		},
	}, &bridgev2.NewLoginParams{
		DeleteOnConflict: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create user login: %w", err)
	}
	ul.Client.Connect(ul.Log.WithContext(context.Background()))

	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeComplete,
		StepID:       LoginStepComplete,
		Instructions: fmt.Sprintf("Successfully bound NapCatQQ account %s", info.Nickname),
		CompleteParams: &bridgev2.LoginCompleteParams{
			UserLoginID: ul.ID,
			UserLogin:   ul,
		},
	}, nil
}

func (nl *NapCatLogin) Cancel() {}
