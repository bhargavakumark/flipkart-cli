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
	oldVerbose := flagVerbose
	oldStdin := os.Stdin
	oldSelectedMobile := config.SelectedMobile
	t.Cleanup(func() {
		flagMobile = oldMobile
		flagCookieString = oldCookieString
		flagUserAgent = oldUserAgent
		flagSite = oldSite
		flagVerbose = oldVerbose
		os.Stdin = oldStdin
		config.SelectedMobile = oldSelectedMobile
	})

	flagMobile = "1234567890"
	flagCookieString = ""
	flagUserAgent = ""
	flagSite = ""
	flagVerbose = false
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

func TestRunAuthSetupPreservesExistingAccountSettings(t *testing.T) {
	isolateAuthSetup(t)
	other := &config.Config{Mobile: "0987654321", CookieAT: "other-token"}
	if err := other.Save(); err != nil {
		t.Fatalf("save other account: %v", err)
	}
	existing := &config.Config{
		Mobile: flagMobile, CookieAT: "old-token", UserAgent: "existing-agent",
		Site: "existing.example.test", AcceptLanguage: "en-CA",
	}
	if err := existing.Save(); err != nil {
		t.Fatalf("save existing account: %v", err)
	}

	pipeAuthInput(t, "Cookie: at=new-token\n")
	if err := runAuthSetup(nil, nil); err != nil {
		t.Fatalf("runAuthSetup() error = %v", err)
	}

	mc, err := config.LoadMultiConfig()
	if err != nil {
		t.Fatalf("LoadMultiConfig() error = %v", err)
	}
	updated := mc.Accounts[flagMobile]
	if updated.CookieAT != "new-token" || updated.UserAgent != "existing-agent" || updated.Site != "existing.example.test" || updated.AcceptLanguage != "en-CA" {
		t.Errorf("updated account did not preserve settings: %#v", updated)
	}
	if got := mc.Accounts[other.Mobile].CookieAT; got != "other-token" {
		t.Errorf("other account token = %q, want other-token", got)
	}
}

func TestRunAuthShowDisplaysSelectedAccountAndAccountList(t *testing.T) {
	isolateAuthSetup(t)
	first := &config.Config{Mobile: "1111111111", CookieAT: "first-secret-token", Site: "first.example.test"}
	second := &config.Config{Mobile: "2222222222", CookieAT: "second-secret-token", Site: "second.example.test"}
	if err := first.Save(); err != nil {
		t.Fatalf("save first account: %v", err)
	}
	if err := second.Save(); err != nil {
		t.Fatalf("save second account: %v", err)
	}
	if err := config.SetDefaultAccount(first.Mobile); err != nil {
		t.Fatalf("set default account: %v", err)
	}
	flagMobile = second.Mobile

	output := captureOutput(t, &os.Stdout, func() {
		if err := runAuthShow(nil, nil); err != nil {
			t.Errorf("runAuthShow() error = %v", err)
		}
	})
	for _, want := range []string{
		"Mobile:                 2222222222 (Active)",
		"Site:                   second.example.test",
		"1111111111 (default)",
		"2222222222 (active)",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("auth show output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "Gift cards linked:") {
		t.Errorf("auth show still displays removed gift-card statistics:\n%s", output)
	}
	if strings.Contains(output, "first-secret-token") || strings.Contains(output, "second-secret-token") {
		t.Errorf("auth show output exposed an access token:\n%s", output)
	}
}

func TestRunAuthDefaultChangesDefaultAccount(t *testing.T) {
	isolateAuthSetup(t)
	first := &config.Config{Mobile: "1111111111", CookieAT: "first"}
	second := &config.Config{Mobile: "2222222222", CookieAT: "second"}
	if err := first.Save(); err != nil {
		t.Fatalf("save first account: %v", err)
	}
	if err := second.Save(); err != nil {
		t.Fatalf("save second account: %v", err)
	}

	if err := runAuthDefault(nil, []string{"2222222222"}); err != nil {
		t.Fatalf("runAuthDefault() error = %v", err)
	}
	mc, err := config.LoadMultiConfig()
	if err != nil {
		t.Fatalf("LoadMultiConfig() error = %v", err)
	}
	if mc.DefaultAccount != "2222222222" {
		t.Errorf("default account = %q, want 2222222222", mc.DefaultAccount)
	}
}

func TestRunAuthSetupSiteOverrideAndFingerprintHeaders(t *testing.T) {
	isolateAuthSetup(t)
	flagSite = "https://explicit.example.test/"
	pipeAuthInput(t, "Cookie: at=access\nHost: copied.example.test\nsec-ch-ua: \"Browser\"\nsec-ch-ua-mobile: ?0\nsec-ch-ua-platform: \"macOS\"\n")

	if err := runAuthSetup(nil, nil); err != nil {
		t.Fatalf("runAuthSetup() error = %v", err)
	}
	mc, err := config.LoadMultiConfig()
	if err != nil {
		t.Fatalf("LoadMultiConfig() error = %v", err)
	}
	cfg := mc.Accounts[flagMobile]
	if cfg.Site != flagSite || cfg.GetSite() != "explicit.example.test" {
		t.Errorf("site = %q (normalised %q), want override %q", cfg.Site, cfg.GetSite(), flagSite)
	}
	if cfg.SecCHUA != `"Browser"` || cfg.SecCHUAMobile != "?0" || cfg.SecCHUAPlatform != `"macOS"` {
		t.Errorf("saved client hints = ua:%q mobile:%q platform:%q", cfg.SecCHUA, cfg.SecCHUAMobile, cfg.SecCHUAPlatform)
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
