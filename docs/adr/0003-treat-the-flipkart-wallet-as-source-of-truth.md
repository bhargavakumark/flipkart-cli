# 3. Treat the Flipkart wallet as the record of linked gift cards

Date: 2026-10-19
Status: accepted

## Why

Linking a gift card is a write to Flipkart's wallet, and the wallet is the only
place that decides whether the card is actually there. The first version of the
tool kept its own ledger anyway: `Config` carried a `[]GiftcardRecord` with the
card number, the PIN, the time it was linked, and the raw response body, and
`giftcard add` appended to it after a call that it treated as successful. That
ledger could only ever disagree with the server. It recorded the PIN in
plaintext, it went stale the moment the card was spent or removed in the
browser, and its idea of success was the API response rather than the state of
the account.

The API response turned out to be the weak part. A gateway error, an incomplete
body, or a non-2xx status after the wallet had already accepted the card all
looked like failures from the client side. Reporting them as failures is worse
than reporting nothing, because the natural response is to try to link the same
card again. Two things were therefore rejected: trusting the response, and
keeping a local record that could be consulted instead.

## How

The ledger is gone. `pkg/config/config.go` no longer has a gift card type or a
gift card field, and `Config` holds credentials and fingerprint headers only.
`giftcard add` in `cmd/giftcard.go` validates the card number and PIN as
numeric, builds `{"cardNumber", "pin"}` and POSTs it to `cfg.GiftcardAPI()`,
which is `https://<site>/api/2/wallet/egv/link`.

Every path that would otherwise be a failure is routed through
`verifyGiftcardAdded` instead of being returned directly: transport errors,
response read errors, HTTP status 400 and above, JSON unmarshal errors, and a
`status` field that is not 2xx and not `true`. That function fetches the active
card list with `fetchActiveGiftcards`, up to three attempts five seconds apart,
and looks for the card number in the result. Finding it prints
`linked successfully (verified)` and returns nil. Not finding it, or failing all
three reads, returns the original error, so the error is suppressed only after
the server has been observed to hold the card.

The read side is `giftcard list`, which fetches
`https://<site>/api/2/wallet/egv/active` and unwraps it with `parseGiftcards`.
The parser tries the `data` field, then common list keys such as `giftCards`,
`list`, and `items`, then the body itself, then keys like `giftCardDetails` and
`RESPONSE`, and finally the nested `RESPONSE.giftCardDetails[].giftCard` shape
the API actually returns, via `unwrapGiftCardDetails`. Output is a table by
default with a total footer computed by `totalBalance`, or JSON under
`--format json` with a `totalBalance` field beside the `cards` array.
`parseListFormat` rejects any other value before a request is made.

## Consequences

There is no local state to drift, nothing to reconcile, and no plaintext PIN on
disk. Every question about gift cards is answered by one authenticated read, so
`giftcard list` is always current and always requires a live session.

The cost is that the tool is fully dependent on that read. `giftcard list`
cannot work from cache, offline, or after the cookies expire, and the
`totalBalance` it prints is only as good as the last response. Verification
costs latency too: when a link genuinely fails, the user waits through up to
three verification attempts before the error appears, which is two five-second
sleeps plus the time of three extra reads.

The lenient parser is the other cost. It accepts many shapes because the
response is undocumented, and when none of them match it returns an empty slice
with a nil error rather than reporting that it could not read the response.
`giftcard list` then prints `No active gift cards found.`, which is
indistinguishable from an account that really has none. Any future change to
the response shape will therefore look like an empty wallet first, not like a
parse failure, and the total will silently read zero.

Reporting a link as successful after an API error is a deliberate asymmetry
that is now hard to undo: a script cannot tell from the exit code whether the
server answered correctly, only whether the card is present. That is the right
question for this tool, but it means the response and the wallet are no longer
interchangeable sources of truth anywhere in the code, and reintroducing a
local record would immediately raise the reconciliation problem this decision
removed.
