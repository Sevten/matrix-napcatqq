package qqid

import (
	"go.mau.fi/util/jsontime"
)

type UserLoginMetadata struct {
	SelfID        string        `json:"self_id,omitempty"`
	Nickname      string        `json:"nickname,omitempty"`
	ConnectedAt   jsontime.Unix `json:"connected_at,omitempty"`
	ConnectionTag string        `json:"connection_tag,omitempty"`
}

type GhostMetadata struct {
	LastSync jsontime.Unix `json:"last_sync,omitempty"`
}

type PortalMetadata struct {
	ChatType ChatType      `json:"chat_type"`
	LastSync jsontime.Unix `json:"last_sync,omitempty"`
}
