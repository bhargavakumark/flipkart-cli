# 3. Treat the Flipkart wallet as the record of linked gift cards

Date: 2026-10-19
Status: accepted

## Why
The wallet is the only authority on whether a card is linked. A link response 
can report a transport or HTTP error after the wallet has accepted the card, 
and a local ledger could only disagree with the wallet.

## How
`pkg/config/config.go` holds credentials and fingerprint headers only, with no 
gift card field. `cmd/giftcard.go` POSTs `{"cardNumber", "pin"}` to 
`cfg.GiftcardAPI()` (`/api/2/wallet/egv/link`) and routes transport, read, HTTP 
400+, unmarshal, and non-success `status` failures through 
`verifyGiftcardAdded`, which polls `fetchActiveGiftcards` on 
`/api/2/wallet/egv/active` up to three attempts five seconds apart and returns 
the original error unless the card number appears. `giftcard list` uses the 
same read path.

## Consequences
No local state drifts and no plaintext PIN is on disk, but every read needs a 
live session. `parseGiftcards` returns an empty slice with a nil error when no 
known shape matches, so a changed response looks like an empty wallet, and a 
failed link costs up to two five-second sleeps before the error appears.
