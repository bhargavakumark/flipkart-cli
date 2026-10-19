# 1. Import browser session cookies for authentication

Date: 2026-10-19
Status: accepted

## Why
The wallet endpoints are undocumented browser APIs with no client registration 
or token endpoint, so the only credential available is a session cookie from a 
signed-in browser. Interactive header entry can block waiting for input, so 
`auth setup` requires headers piped on stdin.

## How
`cmd/auth.go` requires `--mobile` and takes the cookie from `--cookie-string` 
or stdin, refusing to run when `os.Stdin.Stat()` reports a character device. 
`parseCookies` splits it into `at`, `rt`, `T`, `SN`, `vd`, `S`, and `ULSN` plus 
`CookieExtra`, and `runAuthSetup` rejects input without a `Cookie` header or 
`at` before filling `User-Agent`, `Referer`, `Origin`, and `Site` defaults. 
`pkg/config/config.go` rebuilds requests in `BuildCookieHeader` and 
`BuildHeaders`.

## Consequences
No password is ever collected, but nothing refreshes `rt`, so an expired 
session breaks every command until the pipe step repeats. The config is a 
plaintext bearer credential protected only by `0600` file mode, and storing 
fingerprint headers assumes the server checks them.
