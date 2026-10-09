package config

import "testing"

func TestAdminOrigins(t *testing.T) {
	t.Setenv("CPH_ADMIN_ORIGINS", " https://client.example, http://localhost:5173 ,http://[::1]:8080")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AdminOrigins) != 3 {
		t.Fatal(cfg.AdminOrigins)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"*", "null", "https://client.example/", "https://client.example#", "https://user:pass@client.example", "http://client.example", "https://client.example?x=1"} {
		cfg.AdminOrigins = []string{origin}
		if cfg.Validate() == nil {
			t.Fatalf("accepted %q", origin)
		}
	}
}
