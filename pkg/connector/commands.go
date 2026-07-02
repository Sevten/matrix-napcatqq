package connector

import (
	"maunium.net/go/mautrix/bridgev2/commands"
)

var CommandAcceptFriend = &commands.FullHandler{
	Func: fnAcceptFriend,
	Name: "accept-friend",
	Help: commands.HelpMeta{
		Args:        "<flag>",
		Description: "Accept a QQ friend request.",
	},
	RequiresLogin: true,
}

var CommandAcceptGroup = &commands.FullHandler{
	Func: fnAcceptGroup,
	Name: "accept-group",
	Help: commands.HelpMeta{
		Args:        "<flag>",
		Description: "Accept a QQ group join/invite request.",
	},
	RequiresLogin: true,
}

func fnAcceptFriend(ce *commands.Event) {
	if len(ce.Args) == 0 {
		ce.Reply("Usage: accept-friend <flag>")
		return
	}
	flag := ce.Args[0]
	client := ce.User.GetDefaultLogin().Client.(*QQClient)
	sess := client.session()
	if sess == nil {
		ce.Reply("Not connected to NapCatQQ")
		return
	}
	err := sess.SetFriendAddRequest(ce.Ctx, flag, true, "")
	if err != nil {
		ce.Reply("Failed to accept friend request: %v", err)
	} else {
		ce.Reply("Friend request accepted.")
	}
}

func fnAcceptGroup(ce *commands.Event) {
	if len(ce.Args) == 0 {
		ce.Reply("Usage: accept-group <flag>")
		return
	}
	flag := ce.Args[0]
	client := ce.User.GetDefaultLogin().Client.(*QQClient)
	sess := client.session()
	if sess == nil {
		ce.Reply("Not connected to NapCatQQ")
		return
	}
	// Try "add" first, then "invite"
	err := sess.SetGroupAddRequest(ce.Ctx, flag, "add", true, "")
	if err != nil {
		err = sess.SetGroupAddRequest(ce.Ctx, flag, "invite", true, "")
	}
	if err != nil {
		ce.Reply("Failed to accept group request: %v", err)
	} else {
		ce.Reply("Group request accepted.")
	}
}
