package clientshare

import "testing"

func TestRuntimeEnvironment(t *testing.T) {
	for _, key := range []string{"PORT", "VITE_FRONTEND_PORT", "SERVER_BASE_URL", "DOCS_URL", "EMAIL_ENABLED"} {
		t.Setenv(key, "")
	}
	cfg := &Config{}
	cfg.Server.Port = 8080
	if err := cfg.ApplyRuntimeEnvironment(); err != nil || cfg.Server.Port != 8080 {
		t.Fatalf("defaults: %v", err)
	}
	t.Setenv("PORT", "32101")
	t.Setenv("VITE_FRONTEND_PORT", "32102")
	t.Setenv("SERVER_BASE_URL", "https://portal.example.test")
	t.Setenv("DOCS_URL", "https://guide.example.test")
	t.Setenv("EMAIL_ENABLED", "false")
	cfg.Email.Enabled = true
	if err := cfg.ApplyRuntimeEnvironment(); err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 32101 || cfg.Server.VitePort != 32102 || cfg.Server.BaseURL != "https://portal.example.test" || cfg.DocsURL() != "https://guide.example.test" || cfg.Email.Enabled {
		t.Fatal("runtime overrides not applied")
	}
	for _, value := range []string{"zero", "0", "65536", "-1"} {
		t.Setenv("PORT", value)
		if cfg.ApplyRuntimeEnvironment() == nil {
			t.Fatalf("accepted invalid port %s", value)
		}
	}
}
