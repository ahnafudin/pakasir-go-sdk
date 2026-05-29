# Pakasir Go SDK

> A lean, idiomatic, zero-dependency Go client for the [Pakasir](https://pakasir.com) payment gateway.

[![Go Reference](https://pkg.go.dev/badge/github.com/ahnafudin/pakasir-go-sdk.svg)](https://pkg.go.dev/github.com/ahnafudin/pakasir-go-sdk)
[![Go Report Card](https://goreportcard.com/badge/github.com/ahnafudin/pakasir-go-sdk)](https://goreportcard.com/report/github.com/ahnafudin/pakasir-go-sdk)
[![Go Version](https://img.shields.io/badge/go-1.26.3%2B-00ADD8?logo=go)](https://go.dev/dl/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](./LICENSE)

This SDK supports QRIS and multi-bank Virtual Accounts (BNI, BRI, CIMB Niaga,
Maybank, Permata, BNC, Sampoerna, ATM Bersama, Artha Graha) with a type-safe
API, a real-time status-polling helper (`Watch`), and a webhook validator that
follows Pakasir's official security guidance — all in a single package with no
third-party dependencies.

---

## ⚡ Features

- **Zero non-stdlib dependencies.** Standard library only — nothing to audit,
  no supply-chain risk.
- **One package, one import.** The entire surface lives in `package pakasir`;
  `godoc` fits on a single page.
- **Type-safe & IDE-friendly.** Every API response maps to a typed `Payment` or
  `Event` struct; payment methods and statuses are enums.
- **Real-time `Watch` helper.** Track transaction status over a Go channel for
  "waiting for payment" UIs — with status dedup and `context`-based cancellation.
- **Webhook protection.** `Event.Match` / `Event.Verify` validate `order_id` and
  `amount` per Pakasir's official guidance; reject suspicious payloads with 403.
- **Idiomatic.** `context.Context` first, functional options, typed errors
  (`errors.Is` / `errors.As`), and a pluggable `http.Client`.
- **Deterministic.** No hidden retries; plug in your own `http.RoundTripper`
  for retry/backoff behaviour.

## 📦 Installation

Requires **Go 1.26.3 or later**:

```bash
go get github.com/ahnafudin/pakasir-go-sdk
```

Import the package:

```go
import pakasir "github.com/ahnafudin/pakasir-go-sdk"
```

## 🛠️ Configuration

Create a client with your project slug and API key from the Pakasir dashboard:

```go
client := pakasir.New("your-project-slug", "your-secret-api-key")
```

The client is safe for concurrent use by multiple goroutines. Tune behaviour
with functional options:

```go
client := pakasir.New("your-project-slug", "your-secret-api-key",
    pakasir.WithTimeout(15*time.Second),        // HTTP timeout (default 30s)
    pakasir.WithUserAgent("my-app/1.0"),        // custom User-Agent
    pakasir.WithLogger(slog.Default()),         // non-fatal logging (default: silent)
    pakasir.WithHTTPClient(customClient),        // custom *http.Client (retry/proxy/tracing)
    pakasir.WithBaseURL("http://localhost:8080"),// for mock servers / proxies (NOT sandbox)
)
```

> **Sandbox vs Production:** Pakasir has no separate sandbox host. Sandbox is
> enabled per-project in the dashboard, and the webhook payload carries an
> `IsSandbox` flag. Use a sandbox-enabled project with its own API key — not
> `WithBaseURL`.

> ⚠️ **Security:** `DetailPayment` sends the `api_key` as a URL query parameter
> per Pakasir's spec. The key may appear in HTTP access logs — call it from
> server-side code only, never from a frontend.

## 🚀 Quick Start

### 1. Create a payment

Use the `Method` constants for validation and autocompletion:

```go
ctx := context.Background()

payment, err := client.CreatePayment(
    ctx,
    pakasir.MethodQRIS,        // payment method
    "INV-1700000000",          // your unique order_id
    50_000,                    // amount (IDR)
)
if err != nil {
    log.Fatal(err)
}

fmt.Println(payment.PaymentNumber) // QRIS code or VA number
fmt.Println(payment.Fee)           // gateway fee (from API response)
fmt.Println(payment.TotalPayment)  // amount + fee
if payment.ExpiredAt != nil {
    fmt.Println(payment.ExpiredAt)  // payment deadline
}
```

### 2. Build a payment URL (no API call)

`GetPaymentURL` assembles the hosted payment-page URL without a network request:

```go
url := client.GetPaymentURL(
    pakasir.MethodQRIS,
    "INV-1700000000",
    50_000,
    pakasir.WithRedirectURL("https://your-site.com/invoice/done"), // optional
    pakasir.WithQRISOnly(true),                                    // optional
)
// https://app.pakasir.com/pay/{slug}/50000?order_id=INV-1700000000&...
```

### 3. Watch status in real time

`Watch` polls until the status is terminal, the `context` is cancelled, or a
permanent error occurs. Ideal for "waiting for payment" UIs:

```go
watchCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
defer cancel()

for evt := range client.Watch(watchCtx, "INV-1700000000", 50_000) {
    if evt.Err != nil {
        log.Printf("watch error: %v", evt.Err)
        break
    }
    fmt.Println("status:", evt.Payment.Status)
    if evt.Payment.Status.IsTerminal() {
        break
    }
}
```

The channel is closed exactly once. To bound the duration, wrap the `context`
with `context.WithTimeout` — there is no separate timeout option.

### 4. Process and verify webhooks securely

Pakasir does not sign webhooks. Validate every callback with `Verify` (the
error-returning variant) or `Match` (the boolean variant):

```go
func handleWebhook(w http.ResponseWriter, r *http.Request) {
    event, err := pakasir.ParseWebhookRequest(r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    // Look up the real amount from your database by event.OrderID.
    expectedAmount := lookupAmount(event.OrderID)

    if err := event.Verify(event.OrderID, expectedAmount); err != nil {
        // order_id / amount mismatch → reject with 403.
        http.Error(w, err.Error(), http.StatusForbidden)
        return
    }

    // Verified → process side effects idempotently on event.Status.
    switch event.Status {
    case pakasir.StatusCompleted:
        markOrderPaid(event.OrderID, event.CompletedAt)
    }

    w.WriteHeader(http.StatusOK)
}
```

## 💳 Payment Methods

The SDK exposes typed `Method` constants. `Method.IsValid()` checks for unknown
values, and `AllMethods()` returns the full list:

| Method          | Go constant                  | Wire code        | Fee¹             |
|-----------------|------------------------------|------------------|------------------|
| QRIS            | `pakasir.MethodQRIS`         | `qris`           | 0.7% + Rp 310²   |
| BNI VA          | `pakasir.MethodBNIVA`        | `bni_va`         | Rp 3,500         |
| BRI VA          | `pakasir.MethodBRIVA`        | `bri_va`         | Rp 3,500         |
| CIMB Niaga VA   | `pakasir.MethodCIMBVA`       | `cimb_niaga_va`  | Rp 3,500         |
| Maybank VA      | `pakasir.MethodMaybankVA`    | `maybank_va`     | Rp 3,500         |
| Permata VA      | `pakasir.MethodPermataVA`    | `permata_va`     | Rp 3,500         |
| BNC VA          | `pakasir.MethodBNCVA`        | `bnc_va`         | Rp 3,500         |
| ATM Bersama VA  | `pakasir.MethodATMBersamaVA` | `atm_bersama_va` | Rp 3,500         |
| Sampoerna VA    | `pakasir.MethodSampoernaVA`  | `sampoerna_va`   | Rp 2,000         |
| Artha Graha VA  | `pakasir.MethodArthaGrahaVA` | `artha_graha_va` | Rp 2,000         |

> ¹ Fees per the [official Pakasir pricing](https://pakasir.com/p/pricing)
> (updated 22 May 2026); subject to change — always consult the official source.
> The SDK does not compute fees: `Fee` and `TotalPayment` are read from the
> `CreatePayment` response returned by Pakasir's server.
> ² For amounts above Rp 105,000, the QRIS fee becomes 1% + Rp 0.

## 🔄 Transaction Status

| Status     | Go constant               | Terminal? |
|------------|---------------------------|-----------|
| Pending    | `pakasir.StatusPending`   | no        |
| Completed  | `pakasir.StatusCompleted` | yes       |
| Cancelled  | `pakasir.StatusCancelled` | yes       |
| Expired    | `pakasir.StatusExpired`   | yes       |

- `Status.IsTerminal()` — `true` when no further changes are expected.
- `Status.IsKnown()` — `false` for statuses Pakasir may add in the future, so
  you can handle them defensively.

## 📖 API Reference

```go
// Constructor
func New(slug, apiKey string, opts ...Option) *Client

// Payment operations
func (c *Client) CreatePayment(ctx context.Context, method Method, orderID string, amount int64, opts ...PaymentOption) (*Payment, error)
func (c *Client) GetPaymentURL(method Method, orderID string, amount int64, opts ...PaymentOption) string
func (c *Client) DetailPayment(ctx context.Context, orderID string, amount int64) (*Payment, error)
func (c *Client) CancelPayment(ctx context.Context, orderID string, amount int64) (*Payment, error)
func (c *Client) SimulatePayment(ctx context.Context, orderID string, amount int64) (*Payment, error)

// Real-time polling
func (c *Client) Watch(ctx context.Context, orderID string, amount int64, opts ...WatchOption) <-chan WatchEvent

// Webhook
func ParseWebhook(body []byte) (*Event, error)
func ParseWebhookRequest(r *http.Request, opts ...WebhookOption) (*Event, error)
func (e *Event) Match(orderID string, expectedAmount int64) bool
func (e *Event) Verify(orderID string, expectedAmount int64) error
```

| Function              | Description |
|-----------------------|-------------|
| `CreatePayment`       | Creates a new transaction in real time. Returns `*Payment`. |
| `GetPaymentURL`       | Builds the hosted payment URL (no API call). |
| `DetailPayment`       | Fetches the current transaction status. **Server-side only** (api_key in URL). |
| `CancelPayment`       | Cancels a pending transaction. |
| `SimulatePayment`     | Simulates a successful payment (sandbox mode only). |
| `Watch`               | Polls status until terminal / `context` cancelled; streams via a channel. |
| `ParseWebhook`        | Parses a webhook body (`[]byte`) into `*Event`. |
| `ParseWebhookRequest` | Parses from an `*http.Request` with a body-size limit (default 1 MiB). |
| `Event.Match`         | `true` if `order_id` and `amount` match. |
| `Event.Verify`        | `nil` if matched; `ErrOrderIDMismatch` / `ErrAmountMismatch` otherwise. |

Full per-symbol documentation is available on
[pkg.go.dev](https://pkg.go.dev/github.com/ahnafudin/pakasir-go-sdk).

## ⚠️ Error Handling

Non-2xx HTTP responses are returned as `*APIError` (use `errors.As`):

```go
payment, err := client.DetailPayment(ctx, "INV-1", 50_000)
var apiErr *pakasir.APIError
if errors.As(err, &apiErr) {
    log.Printf("HTTP %d: %s", apiErr.StatusCode, apiErr.Message)
}
```

Sentinel errors matchable with `errors.Is`:

| Sentinel               | Cause |
|------------------------|-------|
| `ErrInvalidOrderID`    | `order_id` is empty before the request is sent. |
| `ErrInvalidAmount`     | `amount` is not a positive integer. |
| `ErrInvalidMethod`     | Unknown payment method. |
| `ErrEmptyBody`         | Empty webhook body. |
| `ErrBodyTooLarge`      | Webhook body exceeds the size limit. |
| `ErrOrderIDMismatch`   | Webhook `order_id` mismatch (`Event.Verify`). |
| `ErrAmountMismatch`    | Webhook `amount` mismatch (`Event.Verify`). |

## 🔐 Security

Pakasir does **not** sign webhooks. The SDK provides `Event.Match` /
`Event.Verify` following Pakasir's official guidance ("pastikan amount dan
order_id sesuai") — validate every webhook against your own records and reject
mismatches with HTTP 403. For defense-in-depth against replay or forgery,
restrict the webhook endpoint at your edge (CDN / reverse proxy) to Pakasir's
source IPs, and enforce idempotency on `order_id` in your database.

`DetailPayment` sends the `api_key` as a URL query parameter (per Pakasir's
spec). The key may appear in HTTP access logs — call it only from trusted
server-side code, never from a frontend.

## 🤝 Contributing

Contributions are welcome. Please open an issue to discuss significant changes
before submitting a pull request, and make sure `go vet`, `go test -race ./...`,
and `staticcheck` all pass.

## 📜 License

Distributed under the [MIT License](./LICENSE) — Copyright (c) 2026 Ahnafudin.
