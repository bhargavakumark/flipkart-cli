package cmd

import (
	"fmt"
	"io"
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
	Short: "Configure Flipkart session cookies",
	Long: `Configure Flipkart session cookies by piping in request headers.

Usage: echo "<headers>" | flipkart-cli auth setup

After signing into Flipkart in your browser:
  1. Open Developer Tools → Network tab
  2. Find a request to 1.rome.api.flipkart.com
  3. Right-click → Copy → Copy Request Headers
  4. Pipe them in: echo "<headers>" | flipkart-cli auth setup

Or pass the Cookie header directly:
  flipkart-cli auth setup --cookie-string "T=...; at=...; ..."

Examples:
  echo "Cookie: T=TI...; at=eyJ..." | flipkart-cli auth setup
  flipkart-cli auth setup --cookie-string "T=TI...; at=eyJ..."
`,
	RunE: runAuthSetup,
	Args: cobra.NoArgs,
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
		"Cookie header value to parse directly (non-interactive)")
	authSetupCmd.Flags().StringVar(&flagUserAgent, "user-agent", "",
		"User-Agent header value (default: Mozilla/5.0 ... Firefox/152.0)")
}

func runAuthSetup(cmd *cobra.Command, args []string) error {
	fkCfg, _ := config.Load()
	if fkCfg == nil {
		fkCfg = &config.Config{}
	}

	// Track whether this run actually parsed key values (vs loading from old config)
	hadCookieHeader := false
	hadAT := false

	if flagCookieString != "" {
		hadCookieHeader = true
		if strings.Contains(flagCookieString, "at=") {
			hadAT = true
		}
		parseCookies(flagCookieString, fkCfg)
	} else {
		// Must be piped — refuse to run interactively
		fi, _ := os.Stdin.Stat()
		if (fi.Mode() & os.ModeCharDevice) != 0 {
			return fmt.Errorf("auth setup reads headers from pipe — use:\n  echo \"Cookie: T=...; at=...\" | flipkart-cli auth setup")
		}

		// Read all piped input from stdin (reads until EOF — pipe closes)
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}

		text := strings.TrimSpace(string(data))
		if text == "" {
			return fmt.Errorf("no input received — pipe request headers in, e.g.:\n  echo \"Cookie: T=...\" | flipkart-cli auth setup")
		}

		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Skip request line (GET /api/... HTTP/1.1)
			if isRequestLine(line) {
				continue
			}
			idx := strings.Index(line, ":")
			if idx < 0 {
				continue
			}
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])

			switch key {
			case "Cookie", "cookie":
				hadCookieHeader = true
				if strings.Contains(value, "at=") {
					hadAT = true
				}
				parseCookies(value, fkCfg)
			case "User-Agent":
				fkCfg.UserAgent = value
			case "Referer":
				fkCfg.Referer = value
			case "Origin":
				fkCfg.Origin = value
			}
		}
	}

	// Validate based on what this run actually parsed (not old config values)
	if !hadCookieHeader {
		return fmt.Errorf("no Cookie: header found in input — paste the full request headers from DevTools (Copy → Copy Request Headers)")
	}
	if !hadAT {
		return fmt.Errorf("'at' cookie not found in the Cookie header — expected a cookie named 'at' with the access token JWT")
	}

	// Fill in defaults for anything not provided
	if fkCfg.UserAgent == "" {
		if flagUserAgent != "" {
			fkCfg.UserAgent = flagUserAgent
		} else {
			fkCfg.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:152.0) Gecko/20100101 Firefox/152.0"
		}
	}
	if fkCfg.Referer == "" {
		fkCfg.Referer = "https://www.flipkart.com/"
	}
	if fkCfg.Origin == "" {
		fkCfg.Origin = "https://www.flipkart.com"
	}
	fkCfg.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)

	if err := fkCfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("  Config saved: %s\n", config.ConfigPath())
	fmt.Printf("  at:           %s\n", config.MaskString(fkCfg.CookieAT))
	fmt.Printf("  User-Agent:   %s\n", fkCfg.UserAgent)

	return nil
}

// isRequestLine returns true if line looks like an HTTP request line.
func isRequestLine(line string) bool {
	for _, method := range []string{"GET ", "POST ", "PUT ", "DELETE ", "PATCH ", "OPTIONS ", "HEAD "} {
		if strings.HasPrefix(line, method) {
			return true
		}
	}
	return false
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
