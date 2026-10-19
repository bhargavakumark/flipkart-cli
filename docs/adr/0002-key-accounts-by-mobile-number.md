# 2. Key accounts by mobile number in a multi-account config

Date: 2026-10-19
Status: accepted

## Why

Gift cards are attached to a Flipkart account, not to a person, so a session
cookie is only meaningful together with the account it belongs to. The first
version of the config was a single flat structure with one set of cookies, one
user agent, and no notion of identity. Using a second account meant running
`auth setup` again and destroying the first account's credentials, and there
was no way to say which account a command should act on.

Two shapes were available. The obvious one was one config file per account,
keyed by file name. That would have kept each account independent, but it moves
the notion of a default account into directory conventions or a separate
pointer file, and it makes `auth show` and any future account-scoped command
read the whole directory. The shape that was taken keeps every account in one
file and gives the file an explicit default, which is a single read for every
command and leaves the default under the tool's own control.

## How

`pkg/config/config.go` defines `MultiConfig`, which holds an `Accounts` map
from mobile number to `*Config` and a `DefaultAccount` string. `Config` keeps
only per-account data: the mobile number, the seven named cookies plus extras,
and the fingerprint headers.

Resolution happens in `Load`. It prefers the `SelectedMobile` package variable
and otherwise uses `mc.DefaultAccount`, then looks the mobile up in `Accounts`.
It returns one error when no default account is set and a different one when
the resolved mobile is not in the map. `Save` is a read-modify-write: it loads
the existing `MultiConfig`, inserts or replaces the entry for the mobile
number, sets `DefaultAccount` when it is still empty, and writes the whole file
back through `SaveMultiConfig`.
`SetDefaultAccount` validates that the account exists before reassigning the
default.

Selection is wired through the root command. `cmd/root.go` declares the
persistent `--mobile`/`-m` flag and a `PersistentPreRun` that copies its value
into `config.SelectedMobile`, so every subcommand resolves the same way without
each one declaring the flag. `flipkart-cli auth default <mobile>` calls
`SetDefaultAccount`. `auth setup` looks the account up in the existing
`MultiConfig` and edits that entry rather than starting from an empty `Config`,
which is what preserves fingerprint headers when only the cookies are
refreshed. `auth show` reports the resolved account and lists every saved
account with `(active)`, `(default)`, or `(active, default)` beside it.

## Consequences

Switching accounts is a global flag rather than a re-authentication, and the
default account survives restarts because it is written into the same file as
the credentials. Each account is a self-contained credential, so one expiring
session does not affect the others.

The mobile number is now a primary key and a piece of personal data sitting in
plaintext next to the tokens. It is also the only identifier in the file, so
two accounts are distinguishable by nothing else, and an account whose number
was mistyped at setup is reachable only by deleting the entry by hand.

`Save` is a read-modify-write with no locking, so two concurrent commands that
both save can lose one account's update. It also has a sharper edge: when
`LoadMultiConfig` fails for any reason, including a malformed or truncated
JSON file, `Save` starts from a fresh empty `MultiConfig` and writes over the
corrupt file. Accounts whose entries were unreadable are gone, and the
overwrite happens without a warning.

The `SelectedMobile` package variable couples `cmd` to `config` in a way that
makes the resolution order implicit at call sites: `Load` reads global state
that a `PersistentPreRun` happens to have set. Changing selection to an explicit
argument would touch every call site, which is the price now locked in by the
tests in `pkg/config/config_test.go` that set the variable directly.
