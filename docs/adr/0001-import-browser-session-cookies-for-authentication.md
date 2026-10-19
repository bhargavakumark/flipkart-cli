# 1. Import browser session cookies for authentication

Date: 2026-10-19
Status: accepted

## Why

The wallet endpoints this tool calls are the ones the Flipkart website calls
from a signed-in browser. They are not a documented public API, and there is no
client registration, token endpoint, or API key anywhere in the repository
that could be used to obtain credentials on the tool's own behalf. The only
credential that exists is the session a browser already holds: the `at` access
token cookie and the handful of other cookies the site sets alongside it.

The tool therefore has to be taught a session that was created elsewhere. The
first version tried to accept a cURL command copied out of the browser and
extract the cookie from it, and then tried to prompt for header lines
interactively with `bufio.ReadString`. Both failed in use for reasons that are
visible in the history. cURL text carries line continuations and shell
quoting that have to be unpicked before anything can be parsed, and pasting it
into a `ReadString` loop hangs whenever the paste does not end with a newline.
An interactive header loop also hangs for a different reason: the reader blocks
waiting for input that a person has no clear signal to finish providing.

## How

`cmd/auth.go` implements `flipkart-cli auth setup`. It requires `--mobile`, and
then takes the credential from one of two places. With `--cookie-string` the
value is parsed directly. Otherwise the command inspects `os.Stdin.Stat()` and
refuses to run when stdin is a character device, which is what stops the
interactive hang rather than papering over it; the error message tells the
caller to pipe headers in instead. Piped input is read to EOF with
`io.ReadAll`, split on newlines, and each line is treated as a `Key: Value`
pair. `isRequestLine` drops the `GET /api/... HTTP/1.1` line that comes with a
pasted request, and `parseCookies` splits the `Cookie` value on semicolons into
the seven cookies the code names individually (`at`, `rt`, `T`, `SN`, `vd`,
`S`, `ULSN`), keeping everything else in a single `CookieExtra` field.

Not every header is kept, but the ones that look like a browser fingerprint
are: `User-Agent`, `Accept-Language`, the three `sec-ch-ua` headers, `Referer`,
`Origin`, and the `Host` of the request the headers were copied from, which
becomes the `Site` the API calls go to. When a value is missing the command
fills in a Firefox user agent, `https://www.flipkart.com/` as referer, and
`1.rome.api.flipkart.com` as site.

Validation is deliberately scoped to the input of the current run. Two local
flags, `hadCookieHeader` and `hadAT`, record whether this run's input contained
a `Cookie` header and whether that header contained `at`, and the command fails
if either is false, even when a previous config for the same mobile already
held good values. The resolved config is written by `Config.Save` in
`pkg/config/config.go`, and `auth show` prints it back through
`config.MaskString`, which reveals six leading and four trailing characters of
each cookie.

At request time the individual cookies are reassembled into one `Cookie`
header by `BuildCookieHeader`, and `BuildHeaders` adds the stored fingerprint
headers plus fixed values (`Content-Type`, `Accept`, `DNT`, `Sec-Fetch-*`,
`Connection`) and an `X-User-Agent` built by appending
`FKUA/website/42/website/Desktop` to the stored user agent.

## Consequences

No password is ever collected, typed, or stored, and the tool cannot outlive
the browser session it was given. That is also the main cost: when the session
cookies expire or the account signs out, every command fails with an API error
until someone repeats the copy-and-pipe step. Nothing in the code refreshes the
`rt` refresh token, even though it is captured and replayed, so refresh is a
manual operation.

Storing headers as well as cookies assumes the server checks them. If it does
not, they are harmless but they also do not need to be there; if it does, they
are the reason requests are accepted at all, and a stale `User-Agent` from an
old browser version becomes a possible cause of rejection.

The config file is a bearer credential in plaintext JSON. `SaveMultiConfig`
creates the directory with mode `0700` and the file with mode `0600`, and that
file mode is the entire protection: any process running as the same user can
read the access token. Masking in `auth show` keeps the value off the terminal
but not off the disk.

Requiring `--mobile` on every setup is a small friction that the config does
not remove, because the account key is not known until the credential exists.
The piped-stdin requirement is likewise a permanent constraint on how this
command can be called; making it interactive again would reintroduce the hang
that `os.Stdin.Stat` now prevents.
