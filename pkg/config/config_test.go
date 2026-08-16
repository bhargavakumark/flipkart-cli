package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "flipkart-cli-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	customConfigDir = dir
	t.Cleanup(func() {
		os.RemoveAll(dir)
		customConfigDir = ""
		SelectedMobile = ""
	})
	return dir
}

func TestLoadNonExistent(t *testing.T) {
	setupTestDir(t)
	_, err := Load()
	if err == nil {
		t.Fatal("expected error loading non-existent config, got nil")
	}
}

func TestSaveAndLoadSingle(t *testing.T) {
	setupTestDir(t)

	cfg := &Config{
		CookieAT: "test-at",
		CookieRT: "test-rt",
	}
	cfg.Mobile = "1234567890"

	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Loading should succeed now since it sets the default account
	loaded, err := Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if loaded.Mobile != "1234567890" {
		t.Errorf("expected mobile 1234567890, got %q", loaded.Mobile)
	}
	if loaded.CookieAT != "test-at" {
		t.Errorf("expected CookieAT 'test-at', got %q", loaded.CookieAT)
	}
}

func TestLoadMultipleAccounts(t *testing.T) {
	dir := setupTestDir(t)

	// Manually construct a multi-config
	mc := &MultiConfig{
		DefaultAccount: "1111111111",
		Accounts: map[string]*Config{
			"1111111111": {
				CookieAT: "at-1",
			},
			"2222222222": {
				CookieAT: "at-2",
			},
		},
	}

	data, err := json.Marshal(mc)
	if err != nil {
		t.Fatalf("failed to marshal multi-config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0600); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	// 1. Loading with no SelectedMobile should return DefaultAccount
	cfg, err := Load()
	if err != nil {
		t.Fatalf("failed to load default config: %v", err)
	}
	if cfg.Mobile != "1111111111" {
		t.Errorf("expected default mobile 1111111111, got %q", cfg.Mobile)
	}
	if cfg.CookieAT != "at-1" {
		t.Errorf("expected at-1, got %q", cfg.CookieAT)
	}

	// 2. Loading with SelectedMobile set should return that account
	SelectedMobile = "2222222222"
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("failed to load specific config: %v", err)
	}
	if cfg2.Mobile != "2222222222" {
		t.Errorf("expected mobile 2222222222, got %q", cfg2.Mobile)
	}
	if cfg2.CookieAT != "at-2" {
		t.Errorf("expected at-2, got %q", cfg2.CookieAT)
	}

	// 3. Loading with non-existent SelectedMobile should error
	SelectedMobile = "3333333333"
	_, err = Load()
	if err == nil {
		t.Fatal("expected error loading non-existent account, got nil")
	}
}

func TestSaveWithEmptyMobile(t *testing.T) {
	setupTestDir(t)
	cfg := &Config{
		CookieAT: "test-at",
	}
	// Neither cfg.Mobile nor SelectedMobile is set
	err := cfg.Save()
	if err == nil {
		t.Fatal("expected error saving config with empty mobile, got nil")
	}
}

func TestSetDefaultAccount(t *testing.T) {
	setupTestDir(t)

	// Try setting on non-existent config file
	err := SetDefaultAccount("1234567890")
	if err == nil {
		t.Fatal("expected error setting default on non-existent config, got nil")
	}

	// Save one account
	cfg := &Config{CookieAT: "at-1"}
	cfg.Mobile = "1111111111"
	if err := cfg.Save(); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Save second account
	cfg2 := &Config{CookieAT: "at-2"}
	cfg2.Mobile = "2222222222"
	if err := cfg2.Save(); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Default should be "1111111111" automatically because it was the first
	mc, err := LoadMultiConfig()
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if mc.DefaultAccount != "1111111111" {
		t.Errorf("expected default 1111111111, got %q", mc.DefaultAccount)
	}

	// Set default to "2222222222"
	if err := SetDefaultAccount("2222222222"); err != nil {
		t.Fatalf("failed to set default: %v", err)
	}

	mc, err = LoadMultiConfig()
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if mc.DefaultAccount != "2222222222" {
		t.Errorf("expected default 2222222222, got %q", mc.DefaultAccount)
	}

	// Try setting default to a non-existent account
	err = SetDefaultAccount("3333333333")
	if err == nil {
		t.Fatal("expected error setting default to non-existent account, got nil")
	}
}
