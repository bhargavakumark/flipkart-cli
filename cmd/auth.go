package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bkancherla/flipkart-cli/pkg/config"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage Flipkart authentication",
	Long:  "Commands for managing Flipkart session cookies and authentication configuration",
}

var authSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure Flipkart session cookies interactively",
	Long: `Configure Flipkart session cookies interactively or via flags.

Without flags: prompts for each cookie value.
With --cookie-string: parses a raw Cookie header to extract all values.

After signing into Flipkart in your browser:
1. Open Developer Tools → Network tab
2. Find any request to 1.rome.api.flipkart.com
3. Copy the Cookie header from the request headers
4. Run: flipkart-cli auth setup --cookie-string "T=...; at=...; ..."

Examples:
  flipkart-cli auth setup
  flipkart-cli auth setup --cookie-string "T=TI...; at=eyJ..."
`,
	RunE: runAuthSetup,
}

var authShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current authentication configuration",
	RunE:  runAuthShow,
}

var (
	flagCookieString string
	flagUserAgent    string
)

func init() {
	authCmd.AddCommand(authSetupCmd)
	authCmd.AddCommand(authShowCmd)

	authSetupCmd.Flags().StringVar(&flagCookieString, "cookie-string", "",
		"Raw Cookie header string to parse (non-interactive)")
	authSetupCmd.Flags().StringVar(&flagUserAgent, "user-agent", "",
		"User-Agent header value (default: Mozilla/5.0 ... Firefox/152.0)")
}

func runAuthSetup(cmd *cobra.Command, args []string) error {
	// Load existing config if it exists
	fkCfg, _ := config.Load()
	if fkCfg == nil {
		fkCfg = &config.Config{}
	}

	reader := bufio.NewReader(os.Stdin)

	if flagCookieString != "" {
		// Non-interactive: parse the cookie string
		parseCookies(flagCookieString, fkCfg)
	} else {
		// Interactive mode — paste cookie string
		fmt.Println("Flipkart Auth Setup")
		fmt.Println("───────────────────")
		fmt.Println()
		fmt.Println("After signing into Flipkart in your browser:")
		fmt.Println("  1. Open Developer Tools → Network tab")
		fmt.Println("  2. Find a request to 1.rome.api.flipkart.com")
		fmt.Println("  3. Right-click the request → Copy → Copy as cURL")
		fmt.Println("     OR copy the Cookie header value")
		fmt.Println()
		fmt.Print("Paste Cookie header (or full cURL command), then press Ctrl+D when done:\n\n")

		// Read multi-line paste — keep reading until EOF (Ctrl+D) or blank line
		var lines []string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				// EOF (Ctrl+D) — we're done
				break
			}
			line = strings.TrimSpace(line)
			if line == "" {
				// Blank line signals end of paste
				break
			}
			lines = append(lines, line)
		}
		input := strings.Join(lines, "\n")

		if strings.Contains(input, "-H 'Cookie:") || strings.Contains(input, `-H "Cookie:`) {
			// Looks like a cURL command — extract Cookie value
			cookieVal := extractCookieFromCurl(input)
			if cookieVal != "" {
				parseCookies(cookieVal, fkCfg)
			}
		} else {
			// Single cookie header line — just parse directly
			parseCookies(input, fkCfg)
		}
	}

	// User-Agent
	ua := flagUserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:152.0) Gecko/20100101 Firefox/152.0"
	}
	fkCfg.UserAgent = ua
	fkCfg.Referer = "https://www.flipkart.com/"
	fkCfg.Origin = "https://www.flipkart.com"
	fkCfg.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)

	if err := fkCfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	infofGreen("✓  Configuration saved!")
	fmt.Printf("  Config: %s\n", config.ConfigPath())
	fmt.Println()
	fmt.Println("Run 'flipkart-cli auth show' to verify.")
	fmt.Println("Run 'flipkart-cli giftcard add --card-number <num> --card-pin <pin>' to link a gift card.")

	return nil
}

// parseCookies parses a semicolon-separated cookie string into config fields.
func parseCookies(cookieStr string, cfg *config.Config) {
	pairs := strings.Split(cookieStr, ";")
	var extra []string

	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		eq := strings.IndexByte(pair, '=')
		if eq < 0 {
			continue
		}
		name := strings.TrimSpace(pair[:eq])
		value := strings.TrimSpace(pair[eq+1:])

		switch name {
		case "at":
			cfg.CookieAT = value
		case "rt":
			cfg.CookieRT = value
		case "T":
			cfg.CookieT = value
		case "SN":
			cfg.CookieSN = value
		case "vd":
			cfg.CookieVD = value
		case "S":
			cfg.CookieS = value
		case "ULSN":
			cfg.CookieULSN = value
		default:
			extra = append(extra, pair)
		}
	}

	if len(extra) > 0 {
		cfg.CookieExtra = strings.Join(extra, "; ")
	}
}

// extractCookieFromCurl attempts to extract the Cookie header value from a cURL command.
func extractCookieFromCurl(curl string) string {
	// Look for -H 'Cookie: ...' or -H "Cookie: ..."
	markers := []string{"-H 'Cookie: ", "-H \"Cookie: "}
	for _, marker := range markers {
		start := strings.Index(curl, marker)
		if start < 0 {
			continue
		}
		start += len(marker)

		// Find the closing quote
		endQuote := "'"
		if marker[4] == '"' {
			endQuote = "\""
		}
		end := strings.Index(curl[start:], endQuote)
		if end < 0 {
			continue
		}
		return curl[start : start+end]
	}
	return ""
}

func runAuthShow(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		fmt.Println("No configuration found.")
		fmt.Printf("Run 'flipkart-cli auth setup' to configure credentials.\n")
		fmt.Println()
		fmt.Printf("Config location: %s\n", config.ConfigPath())
		return nil
	}

	cookieHeader := cfg.BuildCookieHeader()

	fmt.Println("Flipkart CLI Configuration")
	fmt.Println("──────────────────────────")
	fmt.Println()
	if cfg.CookieAT != "" {
		fmt.Printf("at (Access Token):      %s\n", config.MaskString(cfg.CookieAT))
	}
	if cfg.CookieRT != "" {
		fmt.Printf("rt (Refresh Token):     %s\n", config.MaskString(cfg.CookieRT))
	}
	if cfg.CookieT != "" {
		fmt.Printf("T (Session):            %s\n", config.MaskString(cfg.CookieT))
	}
	if cfg.CookieSN != "" {
		fmt.Printf("SN (Token):             %s\n", config.MaskString(cfg.CookieSN))
	}
	if cfg.CookieVD != "" {
		fmt.Printf("vd:                     %s\n", config.MaskString(cfg.CookieVD))
	}
	if cfg.CookieS != "" {
		fmt.Printf("S:                      %s\n", config.MaskString(cfg.CookieS))
	}
	if cfg.CookieULSN != "" {
		fmt.Printf("ULSN:                   %s\n", config.MaskString(cfg.CookieULSN))
	}
	if cfg.CookieExtra != "" {
		fmt.Printf("Extra cookies:          %s\n", config.MaskString(cfg.CookieExtra))
	}

	fmt.Println()
	fmt.Printf("User-Agent:             %s\n", cfg.UserAgent)
	fmt.Printf("Referer:                %s\n", cfg.Referer)
	fmt.Printf("Origin:                 %s\n", cfg.Origin)
	fmt.Println()
	fmt.Printf("Cookie header:          %d bytes\n", len(cookieHeader))
	fmt.Println()
	fmt.Printf("Gift cards linked:      %d\n", len(cfg.Giftcards))
	if len(cfg.Giftcards) > 0 {
		last := cfg.Giftcards[len(cfg.Giftcards)-1]
		fmt.Printf("Last linked:            %s... (%s)\n",
			maskCard(last.CardNumber), last.LinkedAt)
	}
	fmt.Println()
	fmt.Printf("Config file:            %s\n", config.ConfigPath())
	fmt.Printf("Last updated:           %s\n", cfg.UpdatedAt)

	return nil
}

func maskCard(num string) string {
	if len(num) <= 4 {
		return "****"
	}
	return "****" + num[len(num)-4:]
}
