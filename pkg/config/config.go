package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GiftcardRecord stores a linked gift card for display/history.
type GiftcardRecord struct {
	CardNumber string `json:"card_number"`
	Pin        string `json:"pin"`
	LinkedAt   string `json:"linked_at"`
	Response   string `json:"response,omitempty"`
}

// Config represents ~/.config/flipkart-cli/config.json
type Config struct {
	// Individual cookies extracted from browser session
	CookieAT    string `json:"cookie_at"`    // access token JWT
	CookieRT    string `json:"cookie_rt"`    // refresh token JWT
	CookieT     string `json:"cookie_t"`     // T session cookie
	CookieSN    string `json:"cookie_sn"`    // SN token cookie
	CookieVD    string `json:"cookie_vd"`    // vd cookie
	CookieS     string `json:"cookie_s"`     // S cookie
	CookieULSN  string `json:"cookie_ulsn"`  // ULSN cookie
	CookieExtra string `json:"cookie_extra"` // any other cookies not captured individually

	// Browser fingerprint
	UserAgent       string `json:"user_agent"`
	AcceptLanguage  string `json:"accept_language,omitempty"`
	SecCHUA         string `json:"sec_ch_ua,omitempty"`
	SecCHUAMobile   string `json:"sec_ch_ua_mobile,omitempty"`
	SecCHUAPlatform string `json:"sec_ch_ua_platform,omitempty"`
	Referer         string `json:"referer"`
	Origin          string `json:"origin"`
	Site            string `json:"site,omitempty"`

	// Gift card history
	Giftcards []GiftcardRecord `json:"giftcards,omitempty"`

	UpdatedAt string `json:"updated_at,omitempty"`
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "flipkart-cli")
}

func configPath() string {
	return filepath.Join(configDir(), "config.json")
}

// ConfigPath returns the path for display purposes.
func ConfigPath() string { return configPath() }

// Load reads the config from disk.
func Load() (*Config, error) {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return nil, fmt.Errorf("config not found at %s\n\nRun: flipkart-cli auth setup\n\nOriginal error: %w",
			configPath(), err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", configPath(), err)
	}
	return &cfg, nil
}

// Save writes the config to disk.
func (c *Config) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.MkdirAll(configDir(), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := os.WriteFile(configPath(), data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}
	return nil
}

// BuildCookieHeader reconstructs the Cookie header from stored values.
func (c *Config) BuildCookieHeader() string {
	var cookies string

	// Always include the individually tracked cookies
	add := func(name, value string) {
		if value == "" {
			return
		}
		if cookies != "" {
			cookies += "; "
		}
		cookies += name + "=" + value
	}

	add("at", c.CookieAT)
	add("rt", c.CookieRT)
	add("T", c.CookieT)
	add("SN", c.CookieSN)
	add("vd", c.CookieVD)
	add("S", c.CookieS)
	add("ULSN", c.CookieULSN)
	if c.CookieExtra != "" {
		if cookies != "" {
			cookies += "; "
		}
		cookies += c.CookieExtra
	}

	return cookies
}

// BuildHeaders returns the map of HTTP headers needed for API calls.
func (c *Config) BuildHeaders() map[string]string {
	headers := map[string]string{
		"Content-Type":   "application/json",
		"Accept":         "*/*",
		"DNT":            "1",
		"Priority":       "u=0",
		"Sec-Fetch-Dest": "empty",
		"Sec-Fetch-Mode": "cors",
		"Sec-Fetch-Site": "same-site",
		"Connection":     "keep-alive",
	}

	if c.UserAgent != "" {
		headers["User-Agent"] = c.UserAgent
		headers["X-User-Agent"] = c.UserAgent + " FKUA/website/42/website/Desktop"
	}
	headers["Accept-Language"] = c.AcceptLanguage
	if headers["Accept-Language"] == "" {
		headers["Accept-Language"] = "en-US,en;q=0.9"
	}
	if c.SecCHUA != "" {
		headers["sec-ch-ua"] = c.SecCHUA
	}
	if c.SecCHUAMobile != "" {
		headers["sec-ch-ua-mobile"] = c.SecCHUAMobile
	}
	if c.SecCHUAPlatform != "" {
		headers["sec-ch-ua-platform"] = c.SecCHUAPlatform
	}
	if c.Referer != "" {
		headers["Referer"] = c.Referer
	} else {
		headers["Referer"] = "https://www.flipkart.com/"
	}
	if c.Origin != "" {
		headers["Origin"] = c.Origin
	} else {
		headers["Origin"] = "https://www.flipkart.com"
	}
	if cookie := c.BuildCookieHeader(); cookie != "" {
		headers["Cookie"] = cookie
	}

	return headers
}

// MaskString returns a masked version of a sensitive string.
func MaskString(s string) string {
	if s == "" {
		return "(not set)"
	}
	if len(s) <= 10 {
		return "***masked***"
	}
	return s[:6] + "..." + s[len(s)-4:]
}

// GetSite returns configured site or default 1.rome.api.flipkart.com.
func (c *Config) GetSite() string {
	site := c.Site
	if site == "" {
		return "1.rome.api.flipkart.com"
	}
	site = strings.TrimPrefix(site, "https://")
	site = strings.TrimPrefix(site, "http://")
	site = strings.TrimSuffix(site, "/")
	return site
}

// GiftcardAPI returns the giftcard link API URL for the configured site.
func (c *Config) GiftcardAPI() string {
	return fmt.Sprintf("https://%s/api/2/wallet/egv/link", c.GetSite())
}

// GiftcardListAPI returns the giftcard list API URL for the configured site.
func (c *Config) GiftcardListAPI() string {
	return fmt.Sprintf("https://%s/api/2/wallet/egv/active", c.GetSite())
}
