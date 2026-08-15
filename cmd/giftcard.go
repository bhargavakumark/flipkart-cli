package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bkancherla/flipkart-cli/pkg/config"
	"github.com/spf13/cobra"
)

var giftcardCmd = &cobra.Command{
	Use:   "giftcard",
	Short: "Manage Flipkart gift cards",
	Long:  "Commands for linking and managing Flipkart gift cards (eGift Vouchers)",
}

var giftcardListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active gift cards on your Flipkart account",
	Long: `List all active eGift Vouchers (gift cards) linked to your Flipkart account.

Shows card number (masked), balance, current date, and expiry date for each card.

Requires authentication (run 'flipkart-cli auth setup' first).

Examples:
  flipkart-cli giftcard list
  flipkart-cli giftcard list --log-http
`,
	Example: `  flipkart-cli giftcard list`,
	RunE:    runGiftcardList,
	Args:    cobra.NoArgs,
}

var giftcardAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Link a gift card to your Flipkart account",
	Long: `Link an eGift Voucher (EGV) gift card to your Flipkart account.

Requires authentication (run 'flipkart-cli auth setup' first).

Examples:
  flipkart-cli giftcard add --card-number 6000170910944181 --card-pin 149232
`,
	Example: `  flipkart-cli giftcard add --card-number 6000170910944181 --card-pin 149232`,
	RunE:    runGiftcardAdd,
}

var (
	flagCardNumber string
	flagCardPin    string
	flagLogHTTP    bool
)

func init() {
	giftcardCmd.AddCommand(giftcardAddCmd)
	giftcardCmd.AddCommand(giftcardListCmd)

	giftcardAddCmd.Flags().StringVar(&flagCardNumber, "card-number", "", "Gift card number (numeric)")
	giftcardAddCmd.Flags().StringVar(&flagCardPin, "card-pin", "", "Gift card PIN (numeric)")
	giftcardAddCmd.Flags().BoolVar(&flagLogHTTP, "log-http", false, "Log HTTP request and response to stderr")

	giftcardAddCmd.MarkFlagRequired("card-number")
	giftcardAddCmd.MarkFlagRequired("card-pin")

	giftcardListCmd.Flags().BoolVar(&flagLogHTTP, "log-http", false, "Log HTTP request and response to stderr")
}

func runGiftcardAdd(cmd *cobra.Command, args []string) error {
	// Validate numeric flags
	cardNumber := strings.TrimSpace(flagCardNumber)
	cardPin := strings.TrimSpace(flagCardPin)

	if _, err := strconv.ParseInt(cardNumber, 10, 64); err != nil {
		return fmt.Errorf("--card-number must be numeric, got %q", flagCardNumber)
	}
	if _, err := strconv.ParseInt(cardPin, 10, 64); err != nil {
		return fmt.Errorf("--card-pin must be numeric, got %q", flagCardPin)
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("authentication required — run 'flipkart-cli auth setup' first: %w", err)
	}

	// Validate we have cookies
	if cfg.CookieAT == "" {
		return fmt.Errorf("access token (at cookie) not set — run 'flipkart-cli auth setup'")
	}

	// Build request body
	body := map[string]string{
		"cardNumber": cardNumber,
		"pin":        cardPin,
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Build request
	apiURL := cfg.GiftcardAPI()
	req, err := http.NewRequest("POST", apiURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	headers := cfg.BuildHeaders()
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Log HTTP request if requested
	if flagLogHTTP {
		fmt.Fprint(os.Stderr, grey)
		fmt.Fprintf(os.Stderr, "---[ HTTP Request ]---\n")
		fmt.Fprintf(os.Stderr, "POST %s\n", apiURL)
		for k, v := range headers {
			fmt.Fprintf(os.Stderr, "%s: %s\n", k, v)
		}
		fmt.Fprintf(os.Stderr, "\n%s\n", string(bodyJSON))
		fmt.Fprintf(os.Stderr, "---[ END ]---\n")
		fmt.Fprint(os.Stderr, reset)
	}

	// Execute
	startedAt := time.Now()
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		addErr := fmt.Errorf("API request failed: %w", err)
		if flagLogHTTP {
			logHTTPTransportError(startedAt, err)
		}
		return verifyGiftcardAdded(cfg, cardNumber, addErr)
	}
	defer resp.Body.Close()
	if flagLogHTTP {
		logHTTPResponseReceived(resp, startedAt)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		readErr := fmt.Errorf("failed to read response: %w", err)
		if flagLogHTTP {
			logHTTPResponseReadError(startedAt, respBody, err)
		}
		return verifyGiftcardAdded(cfg, cardNumber, readErr)
	}

	// Log HTTP response if requested
	if flagLogHTTP {
		fmt.Fprint(os.Stderr, grey)
		fmt.Fprintf(os.Stderr, "---[ HTTP Response ]---\n")
		fmt.Fprintf(os.Stderr, "HTTP %d %s\n", resp.StatusCode, resp.Status)
		for k, vs := range resp.Header {
			for _, v := range vs {
				fmt.Fprintf(os.Stderr, "%s: %s\n", k, v)
			}
		}
		fmt.Fprintf(os.Stderr, "\n%s\n", string(respBody))
		fmt.Fprintf(os.Stderr, "---[ END ]---\n")
		fmt.Fprint(os.Stderr, reset)
		logHTTPResponseComplete(startedAt, len(respBody))
	}

	// Record to config
	record := config.GiftcardRecord{
		CardNumber: cardNumber,
		Pin:        cardPin,
		LinkedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Response:   string(respBody),
	}
	cfg.Giftcards = append(cfg.Giftcards, record)
	if saveErr := cfg.Save(); saveErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save gift card history: %v\n", saveErr)
	}

	// Non-2xx -> error exit code so scripts can detect failure
	if resp.StatusCode >= 400 {
		addErr := fmt.Errorf("API returned HTTP %d: %s", resp.StatusCode, string(respBody))
		return verifyGiftcardAdded(cfg, cardNumber, addErr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		addErr := fmt.Errorf("failed to parse API response: %w\nBody: %s", err, string(respBody))
		return verifyGiftcardAdded(cfg, cardNumber, addErr)
	}

	// Check for explicit success/status field
	if status, ok := result["status"]; ok {
		if s, ok := status.(float64); ok && s >= 200 && s < 300 {
			// success
		} else if s, ok := status.(bool); ok && s {
			// success
		} else {
			// Non-success status — verify the mutation before returning an error.
			var addErr error
			if msg, hasErr := result["error"]; hasErr {
				addErr = fmt.Errorf("API returned status %v: %v", status, msg)
			} else if msg, hasErr := result["message"]; hasErr {
				addErr = fmt.Errorf("API returned status %v: %v", status, msg)
			} else {
				addErr = fmt.Errorf("API returned non-success status: %v", status)
			}
			return verifyGiftcardAdded(cfg, cardNumber, addErr)
		}
	}

	infofGreen("Gift card %s linked successfully", cardNumber)
	return nil
}

func logHTTPTransportError(startedAt time.Time, err error) {
	infofGrey("---[ HTTP Transport Error ]---")
	infofGrey("error: %v", err)
	infofGrey("elapsed: %s", time.Since(startedAt))
	infofGrey("---[ END ]---")
}

func logHTTPResponseReceived(resp *http.Response, startedAt time.Time) {
	infofGrey("---[ HTTP Response Received ]---")
	infofGrey("HTTP %d %s", resp.StatusCode, resp.Status)
	infofGrey("Content-Length: %d", resp.ContentLength)
	infofGrey("Transfer-Encoding: %s", strings.Join(resp.TransferEncoding, ", "))
	infofGrey("Content-Encoding: %s", resp.Header.Get("Content-Encoding"))
	infofGrey("Connection: %s", resp.Header.Get("Connection"))
	infofGrey("elapsed: %s", time.Since(startedAt))
	infofGrey("---[ END ]---")
}

func logHTTPResponseReadError(startedAt time.Time, respBody []byte, err error) {
	infofGrey("---[ HTTP Response Read Error ]---")
	infofGrey("response body bytes read: %d", len(respBody))
	infofGrey("response read error: %v", err)
	infofGrey("elapsed: %s", time.Since(startedAt))
	infofGrey("partial response body: %s", string(respBody))
	infofGrey("---[ END ]---")
}

func logHTTPResponseComplete(startedAt time.Time, bodySize int) {
	infofGrey("response body bytes read: %d", bodySize)
	infofGrey("elapsed: %s", time.Since(startedAt))
}

// verifyGiftcardAdded checks the active-card list after a failed link request.
// The link mutation may succeed even when its response is incomplete or a gateway
// reports an error, so the original error is suppressed only after verification.
func verifyGiftcardAdded(cfg *config.Config, cardNumber string, addErr error) error {
	const maxAttempts = 3
	const retryDelay = 5 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if flagLogHTTP {
			infofGrey("gift card verification attempt %d/%d", attempt, maxAttempts)
		}
		cards, err := fetchActiveGiftcards(cfg)
		if err == nil {
			for _, card := range cards {
				if strings.TrimSpace(card.CardNumber) == cardNumber {
					infofGreen("Gift card %s linked successfully (verified)", cardNumber)
					return nil
				}
			}
			return addErr
		}

		if flagLogHTTP {
			infofGrey("gift card verification failed: %v", err)
		}
		if attempt < maxAttempts {
			time.Sleep(retryDelay)
		}
	}
	return addErr
}

func fetchActiveGiftcards(cfg *config.Config) ([]giftcardInfo, error) {
	apiURL := cfg.GiftcardListAPI()
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create verification request: %w", err)
	}
	for k, v := range cfg.BuildHeaders() {
		req.Header.Set(k, v)
	}

	startedAt := time.Now()
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if flagLogHTTP {
			logHTTPTransportError(startedAt, err)
		}
		return nil, fmt.Errorf("verification request failed: %w", err)
	}
	defer resp.Body.Close()
	if flagLogHTTP {
		logHTTPResponseReceived(resp, startedAt)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if flagLogHTTP {
			logHTTPResponseReadError(startedAt, body, err)
		}
		return nil, fmt.Errorf("failed to read verification response: %w", err)
	}
	if flagLogHTTP {
		infofGrey("verification response body bytes read: %d", len(body))
		infofGrey("verification elapsed: %s", time.Since(startedAt))
		infofGrey("verification response: %s", string(body))
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("verification API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	cards, err := parseGiftcards(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse verification response: %w", err)
	}
	return cards, nil
}

// --- list ---

// giftcardResponse is a flexible response from the giftcard list API.
type giftcardResponse struct {
	Status  interface{}     `json:"status"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
	Error   interface{}     `json:"error"`
}

func runGiftcardList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("authentication required — run 'flipkart-cli auth setup' first: %w", err)
	}
	if cfg.CookieAT == "" {
		return fmt.Errorf("access token (at cookie) not set — run 'flipkart-cli auth setup'")
	}

	cards, err := fetchActiveGiftcards(cfg)
	if err != nil {
		return err
	}
	if len(cards) == 0 {
		fmt.Println("No active gift cards found.")
		return nil
	}
	printGiftcardTable(cards)
	return nil
}

// giftcardInfo represents a single gift card from the API.
type giftcardInfo struct {
	CardNumber     string  `json:"cardNumber"`
	Balance        float64 `json:"balance"`
	BalanceAmount  float64 `json:"balanceAmount"`
	OriginalAmount float64 `json:"originalAmount"`
	Currency       string  `json:"currency"`
	ExpiryDate     string  `json:"expiryDate"`
	Expiry         string  `json:"expiry"`
	IsExpired      bool    `json:"isExpired"`
	Name           string  `json:"name"`
	Status         string  `json:"status"`
}

// effectiveBalance returns whichever balance field is populated.
func (g *giftcardInfo) effectiveBalance() float64 {
	if g.BalanceAmount != 0 {
		return g.BalanceAmount
	}
	return g.Balance
}

// effectiveExpiry returns whichever expiry field is populated.
func (g *giftcardInfo) effectiveExpiry() string {
	if g.ExpiryDate != "" {
		return g.ExpiryDate
	}
	return g.Expiry
}

// giftCardDetail is the wrapper struct for the API response (RESPONSE.giftCardDetails[].giftCard).
type giftCardDetail struct {
	GiftCard struct {
		CardNumber      string  `json:"cardNumber"`
		BalanceAmount   float64 `json:"balanceAmount"`
		ExpiryDate      string  `json:"expiryDate"`
		OriginalAmount  float64 `json:"originalAmount"`
		TransferAllowed bool    `json:"transferAllowed"`
	} `json:"giftCard"`
	ID        string `json:"id"`
	EmailID   string `json:"emailId"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
}

// parseGiftcards extracts gift cards from various response shapes.
func parseGiftcards(body []byte) ([]giftcardInfo, error) {
	// Try parsing as a wrapper response first
	var wrapper giftcardResponse
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, err
	}

	// Check error field
	if wrapper.Error != nil {
		switch e := wrapper.Error.(type) {
		case string:
			if e != "" {
				return nil, fmt.Errorf("API error: %s", e)
			}
		case map[string]interface{}:
			if msg, ok := e["message"]; ok {
				return nil, fmt.Errorf("API error: %v", msg)
			}
		}
	}

	// Try extracting from .data
	if wrapper.Data != nil {
		cards, err := extractCards(wrapper.Data)
		if err == nil && len(cards) > 0 {
			return cards, nil
		}
		// data might be a wrapper itself — try unwrapping one more level
		var inner struct {
			Cards json.RawMessage `json:"giftCards"`
			List  json.RawMessage `json:"list"`
			Items json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(wrapper.Data, &inner); err == nil {
			for _, raw := range []json.RawMessage{inner.Cards, inner.List, inner.Items} {
				if raw != nil {
					cards, err := extractCards(raw)
					if err == nil && len(cards) > 0 {
						return cards, nil
					}
				}
			}
		}
	}

	// Try the full body directly
	cards, err := extractCards(body)
	if err == nil && len(cards) > 0 {
		return cards, nil
	}

	// Try looking for known fields in the top-level response
	var rawMap map[string]interface{}
	if err := json.Unmarshal(body, &rawMap); err == nil {
		// First try keys that may hold wrapped giftCardDetails
		for _, key := range []string{"giftCardDetails", "giftCards", "cards", "list", "items", "vouchers", "egv"} {
			if val, ok := rawMap[key]; ok {
				cards := unwrapGiftCardDetails(val)
				if len(cards) > 0 {
					return cards, nil
				}
				data, err := json.Marshal(val)
				if err == nil {
					cards, err := extractCards(data)
					if err == nil && len(cards) > 0 {
						return cards, nil
					}
				}
			}
		}
		// Try navigating through wrapper keys like RESPONSE
		for _, key := range []string{"RESPONSE", "data", "result"} {
			if val, ok := rawMap[key]; ok {
				data, err := json.Marshal(val)
				if err == nil {
					cards, err := parseGiftcards(data)
					if err == nil && len(cards) > 0 {
						return cards, nil
					}
				}
			}
		}
	}

	// If we got here with data but no cards, return empty
	return []giftcardInfo{}, nil
}

// unwrapGiftCardDetails tries to unmarshal a value as an array of giftCardDetail
// and extract the inner giftCard fields into giftcardInfo.
func unwrapGiftCardDetails(val interface{}) []giftcardInfo {
	data, err := json.Marshal(val)
	if err != nil {
		return nil
	}
	var details []giftCardDetail
	if err := json.Unmarshal(data, &details); err != nil || len(details) == 0 {
		return nil
	}
	cards := make([]giftcardInfo, 0, len(details))
	for _, d := range details {
		cards = append(cards, giftcardInfo{
			CardNumber:     d.GiftCard.CardNumber,
			BalanceAmount:  d.GiftCard.BalanceAmount,
			OriginalAmount: d.GiftCard.OriginalAmount,
			ExpiryDate:     d.GiftCard.ExpiryDate,
			Name:           d.GiftCard.CardNumber,
		})
	}
	return cards
}

// extractCards tries to unmarshal raw JSON as an array of giftcardInfo.
func extractCards(raw json.RawMessage) ([]giftcardInfo, error) {
	// Try array first
	var cards []giftcardInfo
	if err := json.Unmarshal(raw, &cards); err == nil {
		return cards, nil
	}

	// If the array is wrapped in an object with a single "amount" or value field,
	// try extracting it as a single card
	var single map[string]interface{}
	if err := json.Unmarshal(raw, &single); err == nil {
		// Check if this looks like a single card object
		if _, hasNum := single["cardNumber"]; hasNum {
			data, _ := json.Marshal(raw)
			var card giftcardInfo
			if err := json.Unmarshal(data, &card); err == nil {
				return []giftcardInfo{card}, nil
			}
		}
	}

	return nil, fmt.Errorf("not a gift card array or object")
}

// printGiftcardTable formats and prints gift cards in a readable table.
func printGiftcardTable(cards []giftcardInfo) {
	// Balance and Original columns fixed at 7 chars (fits upto 9,999,999)
	const balWidth = 7
	const origWidth = 8
	maxCard := len("Card Number")
	maxExp := len("Expires")
	for _, c := range cards {
		n := len(maskCardDisplay(c.CardNumber))
		if n > maxCard {
			maxCard = n
		}
		e := len(shortDate(c.effectiveExpiry()))
		if e > maxExp {
			maxExp = e
		}
	}

	// Header
	fmt.Printf("  %-*s  %*s  %*s  %-*s  %s\n", maxCard, "Card Number", balWidth, "Balance", origWidth, "Original", maxExp, "Expires", "Status")
	fmt.Printf("  %s  %s  %s  %s  %s\n",
		strings.Repeat("-", maxCard),
		strings.Repeat("-", balWidth),
		strings.Repeat("-", origWidth),
		strings.Repeat("-", maxExp),
		strings.Repeat("-", 10))

	for _, c := range cards {
		cardStr := maskCardDisplay(c.CardNumber)
		balanceStr := fmt.Sprintf("%.0f", c.effectiveBalance())
		origStr := fmt.Sprintf("%.0f", c.OriginalAmount)
		expiryStr := shortDate(c.effectiveExpiry())
		statusStr := cardStatus(c)
		fmt.Printf("  %-*s  %*s  %*s  %-*s  %s\n", maxCard, cardStr, balWidth, balanceStr, origWidth, origStr, maxExp, expiryStr, statusStr)
	}
}

// maskCardDisplay formats a card number for display.
func maskCardDisplay(num string) string {
	return strings.TrimSpace(num)
}

// shortDate normalises various date formats to YYYY-MM-DD.
func shortDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	// Try common layouts
	layouts := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"02-01-2006",
		"01/02/2006",
		"02/01/2006",
		"Jan 02, 2006",
		"January 02, 2006",
		"2006/01/02",
		"20060102",
	}
	for _, layout := range layouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t.Format("2006-01-02")
		}
	}
	// If it's just digits (epoch ms), try that
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		t := time.UnixMilli(n)
		if t.Year() > 2000 && t.Year() < 2100 {
			return t.Format("2006-01-02")
		}
		t = time.Unix(n, 0)
		if t.Year() > 2000 && t.Year() < 2100 {
			return t.Format("2006-01-02")
		}
	}
	// Return as-is if we can't parse
	return s
}

// cardStatus returns a human-readable status string.
func cardStatus(c giftcardInfo) string {
	if c.IsExpired {
		return "expired"
	}
	if c.Status != "" {
		return strings.ToLower(c.Status)
	}
	// Check expiry date
	expiry := strings.TrimSpace(c.Expiry)
	if expiry == "" {
		expiry = c.ExpiryDate
	}
	if expiry != "" {
		if t, err := time.Parse("2006-01-02", shortDate(expiry)); err == nil {
			if t.Before(time.Now()) {
				return "expired"
			}
		}
	}
	return "active"
}
