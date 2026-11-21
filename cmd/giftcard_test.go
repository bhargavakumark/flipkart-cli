package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bkancherla/flipkart-cli/pkg/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func setupGiftcardCommand(t *testing.T, transport http.RoundTripper) {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	oldTransport := http.DefaultTransport
	oldMobile := flagMobile
	oldSelectedMobile := config.SelectedMobile
	oldCardNumber := flagCardNumber
	oldCardPin := flagCardPin
	oldLogHTTP := flagLogHTTP
	oldFormat := flagFormat
	t.Cleanup(func() {
		http.DefaultTransport = oldTransport
		flagMobile = oldMobile
		config.SelectedMobile = oldSelectedMobile
		flagCardNumber = oldCardNumber
		flagCardPin = oldCardPin
		flagLogHTTP = oldLogHTTP
		flagFormat = oldFormat
	})

	http.DefaultTransport = transport
	flagMobile = "1234567890"
	config.SelectedMobile = flagMobile
	flagCardNumber = "6000123456789012"
	flagCardPin = "123456"
	flagLogHTTP = false
	flagFormat = "table"

	cfg := &config.Config{Mobile: flagMobile, CookieAT: "access-token", Site: "api.example.test"}
	if err := cfg.Save(); err != nil {
		t.Fatalf("save test config: %v", err)
	}
}

func responseFor(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func TestRunGiftcardAddHTTPBehavior(t *testing.T) {
	for _, tt := range []struct {
		name    string
		status  int
		body    string
		wantErr string
	}{
		{name: "success", status: http.StatusOK, body: `{"status":true}`},
		{name: "http error", status: http.StatusBadRequest, body: `bad request`, wantErr: "API returned HTTP 400"},
		{name: "API status error", status: http.StatusOK, body: `{"status":false,"message":"invalid card"}`, wantErr: "API returned status false: invalid card"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var addRequestSeen bool
			setupGiftcardCommand(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet {
					return responseFor(req, http.StatusOK, `{"data":[]}`), nil
				}
				addRequestSeen = true
				if req.Method != http.MethodPost || req.URL.Path != "/api/2/wallet/egv/link" {
					t.Errorf("add request = %s %s, want POST giftcard link endpoint", req.Method, req.URL)
				}
				if got := req.Header.Get("Cookie"); !strings.Contains(got, "at=access-token") {
					t.Errorf("Cookie header = %q, want access token cookie", got)
				}
				var payload map[string]string
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Errorf("decode request body: %v", err)
				} else if payload["cardNumber"] != flagCardNumber || payload["pin"] != flagCardPin {
					t.Errorf("request payload = %#v", payload)
				}
				return responseFor(req, tt.status, tt.body), nil
			}))

			err := runGiftcardAdd(nil, nil)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("runGiftcardAdd() error = %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("runGiftcardAdd() error = %v, want containing %q", err, tt.wantErr)
			}
			if !addRequestSeen {
				t.Error("giftcard add request was not made")
			}
		})
	}
}

func TestRunGiftcardListJSON(t *testing.T) {
	setupGiftcardCommand(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/api/2/wallet/egv/active" {
			t.Errorf("list request = %s %s, want GET active giftcards endpoint", req.Method, req.URL)
		}
		return responseFor(req, http.StatusOK, `{"RESPONSE":{"giftCardDetails":[{"giftCard":{"cardNumber":"12345678","balanceAmount":25,"originalAmount":50,"expiryDate":"2030-01-02"}}]}}`), nil
	}))
	flagFormat = "json"

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = oldStdout }()

	if err := runGiftcardList(nil, nil); err != nil {
		t.Fatalf("runGiftcardList() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	os.Stdout = oldStdout

	var output bytes.Buffer
	if _, err := io.Copy(&output, reader); err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	var got giftcardListJSON
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode printed JSON %q: %v", output.String(), err)
	}
	if len(got.Cards) != 1 || got.Cards[0].Balance != 25 || got.TotalBalance != 25 {
		t.Errorf("printed list = %#v", got)
	}
}

func TestParseGiftcards(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    []giftcardInfo
		wantErr string
	}{
		{
			name: "array in data wrapper",
			body: `{"data":[{"cardNumber":"12345678","balance":25.5,"currency":"INR","expiry":"2030-01-01"}]}`,
			want: []giftcardInfo{{CardNumber: "12345678", Balance: 25.5, Currency: "INR", Expiry: "2030-01-01"}},
		},
		{
			name: "Flipkart nested gift card details",
			body: `{"RESPONSE":{"giftCardDetails":[{"id":"id-1","giftCard":{"cardNumber":"987654321","balanceAmount":42,"originalAmount":100,"expiryDate":"2031-02-03"}}]}}`,
			want: []giftcardInfo{{CardNumber: "987654321", BalanceAmount: 42, OriginalAmount: 100, ExpiryDate: "2031-02-03", Name: "987654321"}},
		},
		{
			name:    "API error string",
			body:    `{"error":"not authorized"}`,
			wantErr: "API error: not authorized",
		},
		{
			name:    "malformed JSON",
			body:    `{not json`,
			wantErr: "invalid character",
		},
		{
			name: "empty list",
			body: `{"data":[]}`,
			want: []giftcardInfo{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards, err := parseGiftcards([]byte(tt.body))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseGiftcards() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGiftcards() error = %v", err)
			}
			if len(cards) != len(tt.want) {
				t.Fatalf("parseGiftcards() returned %d cards, want %d: %#v", len(cards), len(tt.want), cards)
			}
			for i := range tt.want {
				if cards[i] != tt.want[i] {
					t.Errorf("card[%d] = %#v, want %#v", i, cards[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseListFormat(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
		bad   bool
	}{
		{input: " table ", want: "table"},
		{input: "JSON", want: "json"},
		{input: "csv", bad: true},
	} {
		got, err := parseListFormat(tt.input)
		if tt.bad {
			if err == nil {
				t.Errorf("parseListFormat(%q) error = nil, want error", tt.input)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("parseListFormat(%q) = %q, %v; want %q, nil", tt.input, got, err, tt.want)
		}
	}
}

func TestEffectiveGiftcardFields(t *testing.T) {
	card := giftcardInfo{Balance: 12, BalanceAmount: 0, Expiry: "legacy-date", ExpiryDate: "preferred-date"}
	if got := card.effectiveBalance(); got != 12 {
		t.Errorf("effectiveBalance() = %v, want 12", got)
	}
	if got := card.effectiveExpiry(); got != "preferred-date" {
		t.Errorf("effectiveExpiry() = %q, want preferred-date", got)
	}
	card.BalanceAmount = 20
	if got := card.effectiveBalance(); got != 20 {
		t.Errorf("effectiveBalance() = %v, want 20", got)
	}
}

func TestGiftcardDisplayHelpers(t *testing.T) {
	if got := maskCardDisplay(" 123456789 "); got != "123456789" {
		t.Errorf("maskCardDisplay() = %q, want trimmed card number", got)
	}
	if got := shortDate("2030-01-02T03:04:05Z"); got != "2030-01-02" {
		t.Errorf("shortDate() = %q, want 2030-01-02", got)
	}
	if got := shortDate(""); got != "-" {
		t.Errorf("shortDate(empty) = %q, want -", got)
	}
	if got := cardStatus(giftcardInfo{IsExpired: true}); got != "expired" {
		t.Errorf("cardStatus(expired) = %q, want expired", got)
	}
	if got := cardStatus(giftcardInfo{Status: "ACTIVE"}); got != "active" {
		t.Errorf("cardStatus(status) = %q, want active", got)
	}
	if got := cardStatus(giftcardInfo{ExpiryDate: time.Now().AddDate(-1, 0, 0).Format("2006-01-02")}); got != "expired" {
		t.Errorf("cardStatus(past expiry) = %q, want expired", got)
	}
}

func TestTotalBalance(t *testing.T) {
	cards := []giftcardInfo{
		{Balance: 12},
		{Balance: 99, BalanceAmount: 8},
	}
	if got := totalBalance(cards); got != 20 {
		t.Errorf("totalBalance() = %v, want 20", got)
	}
}

func TestPrintGiftcardJSON(t *testing.T) {
	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = oldStdout })

	cards := []giftcardInfo{{
		CardNumber: "12345678", Balance: 10, OriginalAmount: 25,
		ExpiryDate: "2030-01-02", IsExpired: true,
	}}
	if err := printGiftcardJSON(cards); err != nil {
		t.Fatalf("printGiftcardJSON() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	os.Stdout = oldStdout

	var output bytes.Buffer
	if _, err := io.Copy(&output, reader); err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}

	var got giftcardListJSON
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode printed JSON %q: %v", output.String(), err)
	}
	if len(got.Cards) != 1 || got.Cards[0].CardNumber != "12345678" || got.Cards[0].Balance != 10 || got.Cards[0].OriginalAmount != 25 || got.Cards[0].Expires != "2030-01-02" || got.Cards[0].Status != "expired" {
		t.Errorf("printed cards = %#v", got.Cards)
	}
	if got.TotalBalance != 10 {
		t.Errorf("printed total balance = %v, want 10", got.TotalBalance)
	}
}
