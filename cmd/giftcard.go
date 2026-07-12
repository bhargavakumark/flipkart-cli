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

const giftcardAPI = "https://1.rome.api.flipkart.com/api/2/wallet/egv/link"

var giftcardCmd = &cobra.Command{
	Use:   "giftcard",
	Short: "Manage Flipkart gift cards",
	Long:  "Commands for linking and managing Flipkart gift cards (eGift Vouchers)",
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
	RunE: runGiftcardAdd,
}

var (
	flagCardNumber string
	flagCardPin    string
	flagLogHTTP    bool
)

func init() {
	giftcardCmd.AddCommand(giftcardAddCmd)

	giftcardAddCmd.Flags().StringVar(&flagCardNumber, "card-number", "", "Gift card number (numeric)")
	giftcardAddCmd.Flags().StringVar(&flagCardPin, "card-pin", "", "Gift card PIN (numeric)")
	giftcardAddCmd.Flags().BoolVar(&flagLogHTTP, "log-http", false, "Log HTTP request and response to stderr")

	giftcardAddCmd.MarkFlagRequired("card-number")
	giftcardAddCmd.MarkFlagRequired("card-pin")
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
	req, err := http.NewRequest("POST", giftcardAPI, bytes.NewReader(bodyJSON))
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
		fmt.Fprintf(os.Stderr, "---[ HTTP Request ]---\n")
		fmt.Fprintf(os.Stderr, "POST %s\n", giftcardAPI)
		for k, v := range headers {
			fmt.Fprintf(os.Stderr, "%s: %s\n", k, v)
		}
		fmt.Fprintf(os.Stderr, "\n%s\n", string(bodyJSON))
		fmt.Fprintf(os.Stderr, "---[ END ]---\n")
	}

	// Execute
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Log HTTP response if requested
	if flagLogHTTP {
		fmt.Fprintf(os.Stderr, "---[ HTTP Response ]---\n")
		fmt.Fprintf(os.Stderr, "HTTP %d %s\n", resp.StatusCode, resp.Status)
		for k, vs := range resp.Header {
			for _, v := range vs {
				fmt.Fprintf(os.Stderr, "%s: %s\n", k, v)
			}
		}
		fmt.Fprintf(os.Stderr, "\n%s\n", string(respBody))
		fmt.Fprintf(os.Stderr, "---[ END ]---\n")
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
		return fmt.Errorf("API returned HTTP %d", resp.StatusCode)
	}
	return nil
}
