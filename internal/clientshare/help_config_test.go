package clientshare

import "testing"

func TestHelpConfig(t *testing.T) {
	for _, tc := range []struct {
		name, provider, mode, url, label, booking string
		email                                     bool
		wantError                                 bool
	}{
		{name: "default"},
		{name: "operator", provider: "operator", url: "/contact", label: "Contact portal operator"},
		{name: "unsafe operator", provider: "operator", url: "javascript:alert(1)", label: "Contact", wantError: true},
		{name: "unlabelled operator", provider: "operator", url: "/contact", wantError: true},
		{name: "hosted", provider: "clientshare", mode: "hosted", email: true},
		{name: "single", provider: "clientshare", mode: "single", email: true, wantError: true},
		{name: "no email", provider: "clientshare", mode: "hosted", wantError: true},
		{name: "unsafe booking", booking: "//evil.invalid", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{}
			cfg.Help.Provider = tc.provider
			cfg.Tenancy.Mode = tc.mode
			cfg.Help.OperatorURL = tc.url
			cfg.Help.OperatorLabel = tc.label
			cfg.Help.BookingURL = tc.booking
			cfg.Email.Enabled = tc.email
			cfg.Email.SMTP.Host = "smtp.example.invalid"
			cfg.Help.Recipient = "support@example.invalid"
			if err := cfg.Validate(); (err != nil) != tc.wantError {
				t.Fatalf("error: %v", err)
			}
		})
	}
}
