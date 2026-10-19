# 2. Key accounts by mobile number in a multi-account config

Date: 2026-10-19
Status: accepted

## Why
Gift cards belong to a Flipkart account, so cookies are only meaningful with 
the account they came from, and a single flat config let a second `auth setup` 
destroy the first account. One file with an explicit default keeps every 
command to a single read.

## How
`pkg/config/config.go` defines `MultiConfig` with an `Accounts` map keyed by 
mobile number and a `DefaultAccount` string; `Load` resolves `SelectedMobile` 
first, then `DefaultAccount`, and errors separately on a missing default and an 
unknown mobile. `Save` is a read-modify-write that sets `DefaultAccount` when 
empty; `SetDefaultAccount` validates the account exists. `cmd/root.go` copies 
the persistent `--mobile`/`-m` flag into `config.SelectedMobile` in 
`PersistentPreRun`.

## Consequences
Switching accounts is a flag, not a re-authentication, but the mobile number is 
plaintext personal data and the only identifier in the file. `Save` takes no 
lock, and when `LoadMultiConfig` fails, including on corrupt JSON, it writes a 
fresh empty `MultiConfig` over the old file without warning.
