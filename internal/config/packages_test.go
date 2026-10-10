package config

import "testing"

func TestPackageInstallEnvironment(t *testing.T) {
	for _, value := range []string{"", "true", "1", "false", "0", "invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("CPH_INSTALL_PACKAGES", value)
			cfg, err := Load()
			if value == "invalid" {
				if err == nil {
					t.Fatal("invalid installation policy accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DisablePackageAutoInstall != (value == "false" || value == "0") {
				t.Fatal("incorrect installation policy")
			}
		})
	}
}
