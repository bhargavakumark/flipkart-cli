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

Usage: echo "<headers>" | flipkart-cli auth setup --mobile <number>

After signing into Flipkart in your browser:
  1. Open Developer Tools → Network tab
  2. Find a request to 1.rome.api.flipkart.com (or another Rome API site)
  3. Right-click → Copy → Copy Request Headers
  4. Pipe them in: echo "<headers>" | flipkart-cli auth setup --mobile <number>

Or pass the Cookie header directly:
  flipkart-cli auth setup --mobile <number> --cookie-string "T=...; at=...; ..."

Examples:
  echo "Cookie: T=TI...; at=eyJ..." | flipkart-cli auth setup --mobile 9876543210
  flipkart-cli auth setup --mobile 9876543210 --cookie-string "T=TI...; at=eyJ..."
  flipkart-cli auth setup --mobile 9876543210 --site 2.rome.api.flipkart.com
`,
	RunE: runAuthSetup,
	Args: cobra.NoArgs,
}

var authShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current authentication configuration",
	RunE:  runAuthShow,
}

var authDefaultCmd = &cobra.Command{
	Use:   "default <mobile>",
	Short: "Set the default Flipkart account",
	Args:  cobra.ExactArgs(1),
	RunE:  runAuthDefault,
}

var (
	flagCookieString string
	flagUserAgent    string
	flagSite         string
	flagVerbose      bool
)

func init() {
	authCmd.AddCommand(authSetupCmd)
	authCmd.AddCommand(authShowCmd)
	authCmd.AddCommand(authDefaultCmd)

	authSetupCmd.Flags().StringVar(&flagCookieString, "cookie-string", "",
		"Cookie header value to parse directly (non-interactive)")
	authSetupCmd.Flags().StringVar(&flagUserAgent, "user-agent", "",
		"User-Agent header value (default: Mozilla/5.0 ... Firefox/152.0)")
	authSetupCmd.Flags().StringVar(&flagSite, "site", "",
		"API site host or URL (default: 1.rome.api.flipkart.com)")
	authShowCmd.Flags().BoolVarP(&flagVerbose, "verbose", "v", false,
		"Show full details including secondary cookies and headers")
}

func runAuthSetup(cmd *cobra.Command, args []string) error {
	if flagMobile == "" {
		return fmt.Errorf("--mobile <number> is required for auth setup")
	}

	// Try loading the existing config for this mobile if it exists
	var fkCfg *config.Config
	mc, err := config.LoadMultiConfig()
	if err == nil && mc.Accounts != nil {
		if existing, ok := mc.Accounts[flagMobile]; ok {
			fkCfg = existing
		}
	}
	if fkCfg == nil {
		fkCfg = &config.Config{}
	}
	fkCfg.Mobile = flagMobile

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
			return fmt.Errorf("auth setup reads headers from pipe — use:\n  echo \"Cookie: T=...; at=...\" | flipkart-cli auth setup --mobile %s", flagMobile)
		}

		// Read all piped input from stdin (reads until EOF — pipe closes)
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}

		text := strings.TrimSpace(string(data))
		if text == "" {
			return fmt.Errorf("no input received — pipe request headers in, e.g.:\n  echo \"Cookie: T=...\" | flipkart-cli auth setup --mobile %s", flagMobile)
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
			case "Accept-Language":
				fkCfg.AcceptLanguage = value
			case "sec-ch-ua":
				fkCfg.SecCHUA = value
			case "sec-ch-ua-mobile":
				fkCfg.SecCHUAMobile = value
			case "sec-ch-ua-platform":
				fkCfg.SecCHUAPlatform = value
			case "Referer":
				fkCfg.Referer = value
			case "Origin":
				fkCfg.Origin = value
			case "Host", "host":
				if fkCfg.Site == "" && flagSite == "" {
					fkCfg.Site = value
				}
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
	if flagSite != "" {
		fkCfg.Site = flagSite
	} else if fkCfg.Site == "" {
		fkCfg.Site = "1.rome.api.flipkart.com"
	}
	fkCfg.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)

	if err := fkCfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("  Config saved: %s\n", config.ConfigPath())
	fmt.Printf("  Site:         %s\n", fkCfg.GetSite())
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
	mc, err := config.LoadMultiConfig()
	if err != nil {
		fmt.Println("No configuration found.")
		fmt.Printf("Run 'flipkart-cli auth setup --mobile <number>' to configure credentials.\n")
		fmt.Println()
		fmt.Printf("Config location: %s\n", config.ConfigPath())
		return nil
	}

	mobile := flagMobile
	if mobile == "" {
		mobile = mc.DefaultAccount
	}

	var cfg *config.Config
	if mobile != "" && mc.Accounts != nil {
		cfg = mc.Accounts[mobile]
	}

	fmt.Println("Flipkart CLI Configuration")
	fmt.Println("──────────────────────────")
	fmt.Println()

	if cfg == nil {
		if flagMobile != "" {
			fmt.Printf("Account for mobile %q not found.\n", flagMobile)
		} else {
			fmt.Println("No active account setup. Use 'auth default <mobile>' or 'auth setup --mobile <number>'.")
		}
	} else {
		cookieHeader := cfg.BuildCookieHeader()

		fmt.Printf("Mobile:                 %s (Active)\n", mobile)
		if cfg.CookieAT != "" {
			fmt.Printf("at (Access Token):      %s\n", config.MaskString(cfg.CookieAT))
		}
		if cfg.CookieRT != "" {
			fmt.Printf("rt (Refresh Token):     %s\n", config.MaskString(cfg.CookieRT))
		}
		if cfg.CookieT != "" {
			fmt.Printf("T (Session):            %s\n", config.MaskString(cfg.CookieT))
		}
		if flagVerbose {
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
			fmt.Printf("Site:                   %s\n", cfg.GetSite())
			fmt.Println()
			fmt.Printf("Cookie header:          %d bytes\n", len(cookieHeader))
		} else {
			fmt.Printf("Site:                   %s\n", cfg.GetSite())
		}
		fmt.Printf("Last updated:           %s\n", cfg.UpdatedAt)
	}

	fmt.Println()
	fmt.Printf("Config file:            %s\n", config.ConfigPath())

	if mc.Accounts != nil && len(mc.Accounts) > 0 {
		fmt.Println()
		fmt.Println("Saved Accounts:")
		for mob := range mc.Accounts {
			isDefault := mob == mc.DefaultAccount
			isActive := mob == mobile
			status := ""
			if isActive && isDefault {
				status = " (active, default)"
			} else if isActive {
				status = " (active)"
			} else if isDefault {
				status = " (default)"
			}
			fmt.Printf("  - %s%s\n", mob, status)
		}
	}

	return nil
}

func runAuthDefault(cmd *cobra.Command, args []string) error {
	mobile := args[0]
	if err := config.SetDefaultAccount(mobile); err != nil {
		return err
	}
	fmt.Printf("Default account set to: %s\n", mobile)
	return nil
}
