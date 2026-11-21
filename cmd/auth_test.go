package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bkancherla/flipkart-cli/pkg/config"
)

func isolateAuthSetup(t *testing.T) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	oldMobile := flagMobile
	oldCookieString := flagCookieString
	oldUserAgent := flagUserAgent
	oldSite := flagSite
	oldStdin := os.Stdin
	oldSelectedMobile := config.SelectedMobile
	t.Cleanup(func() {
		flagMobile = oldMobile
		flagCookieString = oldCookieString
		flagUserAgent = oldUserAgent
		flagSite = oldSite
		os.Stdin = oldStdin
		config.SelectedMobile = oldSelectedMobile
	})

	flagMobile = "1234567890"
	flagCookieString = ""
	flagUserAgent = ""
	flagSite = ""
}

func pipeAuthInput(t *testing.T, input string) {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	if _, err := io.WriteString(writer, input); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		t.Fatalf("write stdin pipe: %v", err)
	}
	if err := writer.Close(); err != nil {
		_ = reader.Close()
		t.Fatalf("close stdin pipe writer: %v", err)
	}
	os.Stdin = reader
	t.Cleanup(func() { _ = reader.Close() })
}

func TestHasCookieMatchesExactName(t *testing.T) {
	for _, tt := range []struct {
		cookieString string
		name         string
		want         bool
	}{
		{cookieString: "cat=not-an-access-token", name: "at", want: false},
		{cookieString: "T=session; at=token", name: "at", want: true},
		{cookieString: " at = token ", name: "at", want: true},
		{cookieString: "at-token=value", name: "at", want: false},
	} {
		if got := hasCookie(tt.cookieString, tt.name); got != tt.want {
			t.Errorf("hasCookie(%q, %q) = %t, want %t", tt.cookieString, tt.name, got, tt.want)
		}
	}
}

func TestParseCookies(t *testing.T) {
	cfg := &config.Config{}
	parseCookies("at=token; rt=refresh; T=session; custom=value=with=equals", cfg)

	if cfg.CookieAT != "token" || cfg.CookieRT != "refresh" || cfg.CookieT != "session" {
		t.Fatalf("parsed cookies = at:%q rt:%q T:%q", cfg.CookieAT, cfg.CookieRT, cfg.CookieT)
	}
	if cfg.CookieExtra != "custom=value=with=equals" {
		t.Fatalf("extra cookies = %q, want preserved value", cfg.CookieExtra)
	}
}

func TestIsRequestLine(t *testing.T) {
	for _, line := range []string{"GET /api HTTP/1.1", "POST /api HTTP/1.1", "HEAD / HTTP/1.1"} {
		if !isRequestLine(line) {
			t.Errorf("isRequestLine(%q) = false, want true", line)
		}
	}
	for _, line := range []string{"Cookie: at=token", "get /api HTTP/1.1", "PATCHED /api"} {
		if isRequestLine(line) {
			t.Errorf("isRequestLine(%q) = true, want false", line)
		}
	}
}

func TestRunAuthSetupParsesPipedRequestHeaders(t *testing.T) {
	isolateAuthSetup(t)
	pipeAuthInput(t, "GET /api/example HTTP/1.1\r\nCookie: at=access; rt=refresh; custom=value\r\nUser-Agent: test-agent\r\nAccept-Language: en-GB\r\nHost: api.example.test\r\n\r\n")

	if err := runAuthSetup(nil, nil); err != nil {
		t.Fatalf("runAuthSetup() error = %v", err)
	}

	mc, err := config.LoadMultiConfig()
	if err != nil {
		t.Fatalf("LoadMultiConfig() error = %v", err)
	}
	cfg := mc.Accounts[flagMobile]
	if cfg == nil {
		t.Fatalf("account %q was not saved", flagMobile)
	}
	if cfg.CookieAT != "access" || cfg.CookieRT != "refresh" || cfg.CookieExtra != "custom=value" {
		t.Errorf("saved cookies = at:%q rt:%q extra:%q", cfg.CookieAT, cfg.CookieRT, cfg.CookieExtra)
	}
	if cfg.UserAgent != "test-agent" || cfg.AcceptLanguage != "en-GB" {
		t.Errorf("saved browser headers = user-agent:%q accept-language:%q", cfg.UserAgent, cfg.AcceptLanguage)
	}
	if cfg.Site != "api.example.test" {
		t.Errorf("saved site = %q, want api.example.test", cfg.Site)
	}
	if mc.DefaultAccount != flagMobile {
		t.Errorf("default account = %q, want %q", mc.DefaultAccount, flagMobile)
	}
	if cfg.UpdatedAt == "" {
		t.Error("UpdatedAt was not set")
	}
}

func TestRunAuthSetupRejectsMissingCookieAndToken(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "no cookie header", input: "User-Agent: test\n", want: "no Cookie: header"},
		{name: "cookie without access token", input: "Cookie: rt=refresh; T=session\n", want: "'at' cookie not found"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			isolateAuthSetup(t)
			pipeAuthInput(t, tt.input)
			err := runAuthSetup(nil, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("runAuthSetup() error = %v, want containing %q", err, tt.want)
			}
			if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".config", "flipkart-cli", "config.json")); !os.IsNotExist(err) {
				t.Errorf("config file exists after invalid input (stat error: %v)", err)
			}
		})
	}
}

func TestRunAuthSetupCookieStringRequiresAccessTokenCookie(t *testing.T) {
	isolateAuthSetup(t)
	flagCookieString = "cat=not-an-access-token"

	err := runAuthSetup(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "'at' cookie not found") {
		t.Fatalf("runAuthSetup() error = %v, want missing access-token-cookie error", err)
	}
}
