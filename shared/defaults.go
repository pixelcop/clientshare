// Package shared provides build-time defaults shared with the frontend.
package shared

import (
	_ "embed"
	"strings"
)

//go:embed invite-welcome.txt
var inviteWelcomeText string

// DefaultInviteWelcomeText is used when no custom invite welcome text is set.
var DefaultInviteWelcomeText = strings.TrimSpace(inviteWelcomeText)
