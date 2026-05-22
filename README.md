# pakasir-go-sdk

> Lean, idiomatic, zero-dep Go client for the [Pakasir](https://pakasir.com) payment gateway.

**Status:** pre-release (v0.1.0 in development).

## Highlights

- Zero non-stdlib dependencies.
- Single package, one import.
- Go 1.22+ — broader compatibility than competitors.
- Real-time `Watch` helper out-of-the-box.
- Webhook validator matching official Pakasir docs.

## Install

```bash
go get github.com/ahnafudin/pakasir-go-sdk
```

## Quickstart

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    pakasir "github.com/ahnafudin/pakasir-go-sdk"
)

func main() {
    client := pakasir.New("your-project-slug", "your-api-key")

    payment, err := client.CreatePayment(
        context.Background(),
        pakasir.MethodQRIS,
        "ORDER-12345",
        100_000,
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("payment_number:", payment.PaymentNumber)

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
    defer cancel()
    for evt := range client.Watch(ctx, payment.OrderID, payment.Amount) {
        if evt.Err != nil {
            log.Fatal(evt.Err)
        }
        fmt.Println("status:", evt.Payment.Status)
        if evt.Payment.Status.IsTerminal() {
            break
        }
    }
}
```

## Webhook handler

```go
http.HandleFunc("/webhook/pakasir", func(w http.ResponseWriter, r *http.Request) {
    event, err := pakasir.ParseWebhookRequest(r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    expectedAmount := lookupAmount(event.OrderID) // your DB lookup
    if !event.Match(event.OrderID, expectedAmount) {
        http.Error(w, "mismatch", http.StatusForbidden)
        return
    }

    // process side effects idempotently on event.Status
    w.WriteHeader(http.StatusOK)
})
```

See `examples/basic/` and `examples/webhook/` for runnable code.

## Watch polling semantics

`client.Watch` returns a `<-chan WatchEvent` that follows a small, well-defined
contract — keeping consumer loops simple and predictable:

- Initial poll fires immediately at `t=0`; subsequent polls every interval (default 3s).
- Status dedup: consecutive polls returning the same status are collapsed; only
  the first emission of a new status is delivered.
- Channel is buffered (size 1). Intermediate events use a best-effort send and
  may be dropped if the consumer is slow; terminal events and permanent errors
  use a blocking send with `ctx.Done()` escape — guaranteed delivery unless ctx
  is already cancelled.
- Channel closes exactly once when `ctx` is done, the status becomes terminal,
  or a permanent error occurs.
- To bound the watch duration, wrap the parent context with
  `context.WithTimeout(ctx, d)` — there is no separate `WithTimeout` watch option.

See the design spec for the full state table and error-classification rules.

## Documentation

- **Design spec:** `docs/superpowers/specs/2026-05-22-pakasir-go-sdk-design.md`
- **API reference:** [pkg.go.dev/github.com/ahnafudin/pakasir-go-sdk](https://pkg.go.dev/github.com/ahnafudin/pakasir-go-sdk) (after public release)

## Why another Pakasir SDK?

| SDK | Lang | Strengths | Weaknesses |
|---|---|---|---|
| [`zeative/pakasir-sdk`](https://github.com/zeative/pakasir-sdk) | Node/TS | Clean class API; `watchPayment` polling | No Go; no webhook helper |
| [`H0llyW00dzZ/pakasir-go-sdk`](https://github.com/H0llyW00dzZ/pakasir-go-sdk) | Go | Feature-rich: gRPC, QR rendering, i18n, retries | Heavy (13+ packages, `src/` subdir, `bytebufferpool` dep, Go 1.26 required) |
| **`ahnafudin/pakasir-go-sdk`** (this) | Go | Zero non-stdlib deps, single package, Go 1.22+, official-spec webhook, real-time `Watch` | No gRPC/QR/i18n |

Pick the one that fits your needs. Honest comparison.

## Security

Pakasir does **not** sign webhooks. The SDK exposes `Event.Match` / `Event.Verify`
following Pakasir's official guidance ("pastikan amount dan order_id sesuai").
For replay/forgery defense-in-depth, see the spec's "Replay & forgery mitigation"
section.

`DetailPayment` transmits the API key as a URL query parameter (Pakasir's spec).
The key may surface in HTTP access logs — call only from trusted server-side
code, never frontends.

## License

[MIT](./LICENSE) — Copyright (c) 2026 Ahnafudin.
