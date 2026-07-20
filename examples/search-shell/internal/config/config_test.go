package config

import "testing"

func TestDefaultsUseSafeNetworkAndAuthenticationValues(t *testing.T) {
	cfg := Defaults()
	if cfg.Server.Address != "127.0.0.1:8484" {
		t.Fatalf("address = %q", cfg.Server.Address)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate defaults: %v", err)
	}
	if cfg.Auth.SigningKey == "search-shell-dev-signing-key" {
		t.Fatal("legacy signing key is still configured")
	}
	if cfg.Auth.DemoPassword == "admin.pwd" {
		t.Fatal("legacy demo password is still configured")
	}
}

func TestValidateRejectsLegacyAuthenticationValues(t *testing.T) {
	cfg := Defaults()
	cfg.Auth.SigningKey = "search-shell-dev-signing-key"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected legacy signing key to be rejected")
	}

	cfg = Defaults()
	cfg.Auth.DemoPassword = "admin.pwd"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected legacy password to be rejected")
	}
}
