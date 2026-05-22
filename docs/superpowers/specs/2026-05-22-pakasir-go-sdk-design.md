# Pakasir Go SDK — Design Spec

**Date:** 2026-05-22
**Status:** v2 — implemented in v0.1.0-alpha.1 (2026-05-22)
**Repo (target):** `github.com/ahnafudin/pakasir-go-sdk`
**Package:** `pakasir`
**License:** MIT
**Author:** Ahnafudin (`ngesrepbarat3@gmail.com`)

**Changelog:**
- v1 (2026-05-22): initial draft.
- v2 (2026-05-22): module rename (`pakasir-sdk` → `pakasir-go-sdk`); WithBaseURL
  scope clarified; Watch polling semantics made explicit; Status.IsKnown(),
  Method.IsValid(), Event.Verify() added; WithLogger added; webhook replay
  mitigation documented; Windows CI added; fuzz test for ParseWebhook.
- v2 implementation completed 2026-05-22 — tagged `v0.1.0-alpha.1`.
  Final state: 8 source files + 8 test files (incl. fuzz), 95.8% line
  coverage, 9 stdlib-only imports total (no external deps). Spec is now
  reference documentation; subsequent API changes should bump the spec
  date and module version per §11.

---

## 1. Background & Motivation

Pakasir is an Indonesian payment gateway. Official client support is REST + Node.js
only. Two community Go SDKs exist today:

| SDK | Strengths | Weaknesses |
|---|---|---|
| `zeative/pakasir-sdk` (Node/TS) | Clean class API; `watchPayment` polling helper | Not Go |
| `H0llyW00dzZ/pakasir-go-sdk` | Feature-rich (gRPC, QR, i18n, retries) | Heavy (13+ packages, `src/` subdir, `bytebufferpool` dep, Go 1.26 required, Service-pattern boilerplate) |

`dracin-go` integrates Pakasir directly in `internal/httpapi/handlers/billing.go`
today with two known bugs: invented HMAC verification (Pakasir does not sign
webhooks) and wrong checkout URL format. Migrating those concerns into a shared,
correct SDK is the catalyst for this work.

**Strategy:** ship a *third* Go SDK whose differentiator is **clarity & DX**, not
feature breadth. Target the 90% case: REST calls + webhook handler. Leave the 10%
edge cases (gRPC, QR rendering, framework adapters) to other libraries.

**Module naming decision:** module path is `github.com/ahnafudin/pakasir-go-sdk`
(not `pakasir-sdk`). The `-go-` suffix matches Go ecosystem convention
(`aws-sdk-go`, `H0llyW00dzZ/pakasir-go-sdk`), avoids name collision with the
Node SDK (`zeative/pakasir-sdk`), and matches the on-disk directory used during
development.

---

## 2. Goals & Non-Goals

### Goals

- **Single import** — `import "github.com/ahnafudin/pakasir-go-sdk"` exposes the
  full surface from one package.
- **Zero non-stdlib dependencies in the core** — only `net/http`,
  `encoding/json`, `context`, `time`, `errors`. Marketed in README.
- **Idiomatic Go** — `context.Context` first param; functional options;
  methods on `*Client`; typed errors with `errors.Is/As`; channels for streams.
- **Wide compatibility** — Go 1.22+ (covers current and previous Go release per
  Google's support window; works on enterprise distros & older CI).
- **Wire correctness** — match Pakasir REST spec exactly; pre-parse RFC3339
  timestamps; unwrap `{payment: ...}` / `{transaction: ...}` envelopes.
- **Webhook helpers** — `ParseWebhook(body)` + `ParseWebhookRequest(r)` +
  `Event.Match(orderID, expectedAmount)`. Matches Pakasir's published security
  guidance literally.
- **Polling helper** — `Watch(ctx) <-chan WatchEvent` for "waiting for payment"
  UIs. Cancellation via `ctx.Cancel()`. No separate `StopWatch` method.
- **Test coverage ≥ 80%** — table-driven tests + `httptest.NewServer` for HTTP
  paths.

### Non-Goals

- ❌ gRPC server stubs — not a payment SDK concern.
- ❌ QR PNG generation — Pakasir already returns `payment_url` for QRIS; FE can
  use any QR library to render that URL.
- ❌ Framework adapters (Fiber/Echo/Gin middleware) — `net/http` is the
  lingua franca; converting takes ≤3 lines in any framework.
- ❌ i18n / localized error messages — errors are for developers; end-user
  copy is the application layer's job.
- ❌ Built-in retry/backoff — caller injects via custom `http.RoundTripper`.
  Keeps SDK behaviour deterministic.
- ❌ Buffer pooling — premature optimization for ~1 KB JSON payloads at
  low call volume.
- ❌ Multi-version SDK in one repo — when Pakasir releases breaking changes,
  bump SDK major version.

---

## 3. Positioning Statement

> **`pakasir-go-sdk`** — Lean, idiomatic, zero-dep Go client for Pakasir.
> Single package, real-time `Watch` helper, webhook validator that follows
> Pakasir's official security guidance.

README hero bullets:

- ✅ **Zero non-stdlib dependencies**
- ✅ **Single package, one import**
- ✅ **Go 1.22+ (broader compatibility than competitors)**
- ✅ **Real-time `Watch` helper out-of-the-box**
- ✅ **Webhook validator matching official Pakasir docs**

---

## 4. Repository Layout

```
pakasir-go-sdk/
├── client.go              # *Client, New(), Option, With*
├── payment.go             # CreatePayment, GetPaymentURL, Detail, Cancel, Simulate
├── watch.go               # Watch(ctx) <-chan WatchEvent
├── webhook.go             # ParseWebhook, ParseWebhookRequest, Event, Event.Match
├── method.go              # type Method + const enum
├── status.go              # type Status + const enum
├── errors.go              # *APIError, sentinel errors
├── doc.go                 # package-level godoc + module overview
├── client_test.go
├── payment_test.go
├── watch_test.go
├── webhook_test.go
├── method_test.go
├── examples/
│   ├── basic/
│   │   └── main.go        # create payment, get URL, detail
│   └── webhook/
│       └── main.go        # net/http handler with Event.Match
├── .github/
│   └── workflows/
│       └── ci.yml         # go test, go vet, staticcheck on push/PR
├── go.mod                 # module github.com/ahnafudin/pakasir-go-sdk; go 1.22
├── LICENSE                # MIT
├── README.md              # single-page; install → quickstart → API → examples
└── CHANGELOG.md           # semver-tracked
```

**Explicit non-choices:**

- No `src/` subdirectory (non-idiomatic in Go).
- No `cmd/`, `internal/`, `pkg/`, or sub-packages in v1 — flat keeps `godoc`
  navigation trivial.
- `examples/` is *not* a Go module member; each example has its own `go.mod`
  to avoid pulling test/example deps into consumers' graphs.

---

## 5. Public API

### 5.1 Client

```go
package pakasir

type Client struct { /* unexported */ }

// New creates a Client. slug + apiKey are required.
func New(slug, apiKey string, opts ...Option) *Client

type Option func(*config)

// WithHTTPClient overrides the default *http.Client (which has a 30s timeout).
// Use this to inject custom Transport (retries, tracing, proxy).
func WithHTTPClient(c *http.Client) Option

// WithBaseURL overrides the API base. Default: "https://app.pakasir.com".
//
// NOT FOR SANDBOX SWITCHING. Pakasir does not publish a separate sandbox
// host; sandbox is enabled per-project in the Pakasir dashboard, and the
// webhook payload carries an is_sandbox flag. Use this option only for
// proxying, mock servers (httptest.NewServer in tests), or self-hosted
// gateways.
func WithBaseURL(url string) Option

// WithTimeout sets the default HTTP timeout (default 30s).
// Ignored if WithHTTPClient is also provided.
func WithTimeout(d time.Duration) Option

// WithUserAgent overrides the User-Agent header
// (default: "pakasir-go-sdk/<version>").
func WithUserAgent(ua string) Option

// WithLogger sets a structured logger used by the SDK for non-fatal events
// (Watch poll failures, transient HTTP errors, body-size truncation).
// Defaults to slog.New(slog.DiscardHandler{}) so the SDK is silent unless
// explicitly opted in. Zero-dep: log/slog is stdlib in Go 1.21+.
//
// The SDK does not log request/response bodies — never api_key or PII.
func WithLogger(l *slog.Logger) Option
```

### 5.2 Payment Methods

```go
// CreatePayment initiates a transaction.
// POST /api/transactioncreate/{method}
func (c *Client) CreatePayment(
    ctx context.Context,
    method Method,
    orderID string,
    amount int64,
    opts ...PaymentOption,
) (*Payment, error)

// GetPaymentURL builds the hosted payment URL (no API call).
// Format: https://app.pakasir.com/pay/{slug}/{amount}?order_id=...
func (c *Client) GetPaymentURL(
    method Method,
    orderID string,
    amount int64,
    opts ...PaymentOption,
) string

// DetailPayment fetches current transaction status.
// GET /api/transactiondetail?project=&amount=&order_id=&api_key=
//
// SECURITY: This endpoint transmits api_key as a URL query parameter, per
// Pakasir's spec. The key may appear in HTTP access logs and proxies. Avoid
// calling DetailPayment from frontends. Use server-side only.
func (c *Client) DetailPayment(
    ctx context.Context,
    orderID string,
    amount int64,
) (*Payment, error)

// CancelPayment terminates a pending transaction.
// POST /api/transactioncancel
func (c *Client) CancelPayment(
    ctx context.Context,
    orderID string,
    amount int64,
) (*Payment, error)

// SimulatePayment triggers a webhook callback as if the payment completed.
// POST /api/paymentsimulation
//
// Sandbox-only. Calling in production will fail with *APIError.
func (c *Client) SimulatePayment(
    ctx context.Context,
    orderID string,
    amount int64,
) (*Payment, error)

type PaymentOption func(*paymentConfig)

// WithRedirectURL sets the post-payment redirect for GetPaymentURL.
func WithRedirectURL(url string) PaymentOption

// WithQRISOnly forces the hosted page to show QRIS only (true/false).
func WithQRISOnly(only bool) PaymentOption
```

### 5.3 Watch (polling)

```go
// Watch polls DetailPayment until status is terminal (completed/cancelled/expired)
// or ctx is cancelled. Default interval 3s. Events stream on the returned channel;
// channel closes when polling stops.
//
// Cancellation: caller cancels ctx. There is no separate StopWatch method.
// Bound polling duration via context.WithTimeout(parent, d) — there is no
// WithTimeout WatchOption (keeps cancellation semantics uniform with the
// rest of the SDK).
func (c *Client) Watch(
    ctx context.Context,
    orderID string,
    amount int64,
    opts ...WatchOption,
) <-chan WatchEvent

type WatchOption func(*watchConfig)

// WithInterval sets the poll cadence (default 3s, min 1s).
func WithInterval(d time.Duration) WatchOption

type WatchEvent struct {
    Payment *Payment   // nil on Err
    Err     error      // non-nil → permanent failure; channel closes after this event
                        // (transient failures are not surfaced — see semantics table below)
}
```

**Polling semantics (contract):**

| Aspect | Behaviour |
|---|---|
| Initial poll | Immediate at t=0; subsequent polls every `interval` thereafter. |
| Channel buffer | Size 1. |
| Status dedup | Emit `WatchEvent{Payment}` only when status changes from the prior poll. First successful poll always emits. Suppresses duplicate `pending` events. |
| Transient errors (HTTP 5xx, `net.Error`, `context.DeadlineExceeded` from a single request) | Log via `slog.Logger`, do not emit `WatchEvent`, continue at next interval. |
| Permanent errors (4xx other than 404, malformed response) | Emit one `WatchEvent{Err: <APIError or wrapped>}`, then close channel. |
| 404 on detail | Treated as transient — Pakasir may not have indexed the transaction yet. Same behaviour as 5xx. |
| Send semantics — intermediate events | Best-effort: `select { case ch <- evt: default }`. Dropped if consumer hasn't read the previous event yet. Polling never blocks on consumer. |
| Send semantics — terminal event | Guaranteed: blocking send (with `ctx.Done()` escape). The final status (or permanent error) is delivered before the channel closes, unless the caller has already cancelled ctx. |
| Channel close | Always closed exactly once, on any exit path: terminal status, parent ctx cancelled, or permanent error. Range loops over the channel are safe. |
| Caller dropped channel | Poller observes `ctx.Done()`; there is no way to leak a goroutine if caller cancels `ctx`. |

A slow consumer may miss intermediate status changes (e.g., never sees a brief
`pending` if it lingered on a previous event during the poll), but always
receives the terminal event or a permanent error. For full history, persist
events to the merchant's database from the consumer side.

### 5.4 Types

```go
type Method string

const (
    MethodQRIS         Method = "qris"
    MethodBNIVA        Method = "bni_va"
    MethodBRIVA        Method = "bri_va"
    MethodCIMBVA       Method = "cimb_niaga_va"
    MethodMaybankVA    Method = "maybank_va"
    MethodPermataVA    Method = "permata_va"
    MethodBNCVA        Method = "bnc_va"
    MethodSampoernaVA  Method = "sampoerna_va"
    MethodATMBersamaVA Method = "atm_bersama_va"
    MethodArthaGrahaVA Method = "artha_graha_va"
)

// IsValid reports whether m is a known payment method. O(1) map lookup.
func (m Method) IsValid() bool

// AllMethods returns every known method (helpful for validation/UX).
// Returns a new slice on each call; callers may safely modify the result.
func AllMethods() []Method

type Status string

const (
    StatusPending   Status = "pending"
    StatusCompleted Status = "completed"
    StatusCancelled Status = "cancelled"
    StatusExpired   Status = "expired"
)

// IsTerminal reports whether s indicates a final outcome (no further changes).
// Returns false for unknown statuses — when Status.IsKnown() is false, the
// caller must treat the value defensively (e.g., log + continue polling).
func (s Status) IsTerminal() bool

// IsKnown reports whether s is one of the documented status values.
// Returns false for statuses Pakasir may add in the future.
//
// Note: only StatusCompleted is officially documented by Pakasir; the other
// three (pending, cancelled, expired) are inferred from community SDKs
// (zeative/pakasir-sdk, H0llyW00dzZ/pakasir-go-sdk) and field observation.
// They are expected to be stable but may evolve.
func (s Status) IsKnown() bool

type Payment struct {
    Project       string
    OrderID       string
    Amount        int64
    Fee           int64       // set on CreatePayment response
    TotalPayment  int64       // set on CreatePayment response
    Method        Method
    PaymentNumber string      // VA number or QRIS string
    Status        Status      // set on Detail/Watch response
    ExpiredAt     *time.Time  // set on CreatePayment response
    CompletedAt   *time.Time  // set when Status == Completed
}
```

### 5.5 Webhook

```go
// ParseWebhook decodes a webhook body. Caller is responsible for reading
// the body from the request (e.g., from c.Request().Body() in Fiber).
func ParseWebhook(body []byte) (*Event, error)

// ParseWebhookRequest decodes from *http.Request. Closes r.Body.
// Enforces a 1 MiB body size limit (overridable via WithMaxBodySize).
func ParseWebhookRequest(r *http.Request, opts ...WebhookOption) (*Event, error)

type WebhookOption func(*webhookConfig)

// WithMaxBodySize sets the body cap for ParseWebhookRequest (default 1 MiB).
func WithMaxBodySize(bytes int64) WebhookOption

type Event struct {
    Project       string
    OrderID       string
    Amount        int64
    Status        Status
    PaymentMethod Method
    CompletedAt   *time.Time
    IsSandbox     bool        // distinguishes sandbox callbacks
}

// Match implements Pakasir's recommended validation: order_id and amount
// must match the merchant's record of the transaction. Returns true only
// when both match exactly.
//
// Use this on every received webhook. Reject mismatches with HTTP 403.
func (e *Event) Match(orderID string, expectedAmount int64) bool

// Verify is the error-returning companion to Match. Returns ErrOrderIDMismatch
// or ErrAmountMismatch (wrappable, check with errors.Is) on failure, or nil
// when both fields match. Useful for structured logging or metrics where the
// specific cause of rejection matters.
//
// Order of checks: order_id first, then amount. This means a payload with
// BOTH fields mismatched returns ErrOrderIDMismatch only.
func (e *Event) Verify(orderID string, expectedAmount int64) error
```

### 5.6 Errors

```go
// APIError represents a non-2xx HTTP response from Pakasir.
type APIError struct {
    StatusCode int       // HTTP status (e.g., 400, 404, 500)
    Status     string    // HTTP status text
    Message    string    // best-effort decoded error message from body
    Raw        []byte    // raw response body for debugging
}

func (e *APIError) Error() string

// Sentinel errors. All wrappable; use errors.Is to check.
var (
    ErrInvalidOrderID     = errors.New("pakasir: invalid order_id")
    ErrInvalidAmount      = errors.New("pakasir: invalid amount")
    ErrInvalidMethod      = errors.New("pakasir: invalid payment method")
    ErrEmptyBody          = errors.New("pakasir: empty webhook body")
    ErrBodyTooLarge       = errors.New("pakasir: webhook body too large")
    ErrAmountMismatch     = errors.New("pakasir: amount mismatch")     // returned by Event.Verify
    ErrOrderIDMismatch    = errors.New("pakasir: order_id mismatch")   // returned by Event.Verify
)
```

---

## 6. Wire-Level Behaviour

| Operation | Method | Path | Body / Query | Response unwrapping |
|---|---|---|---|---|
| CreatePayment | POST | `/api/transactioncreate/{method}` | `{project, order_id, amount, api_key}` JSON | unwrap `{payment: {...}}` → `*Payment` |
| GetPaymentURL | — | `/pay/{slug}/{amount}` | query: `order_id`, `redirect`, `qris_only` | n/a (returns string) |
| DetailPayment | GET | `/api/transactiondetail` | query: `project, amount, order_id, api_key` | unwrap `{transaction: {...}}` → `*Payment` |
| CancelPayment | POST | `/api/transactioncancel` | `{project, order_id, amount, api_key}` JSON | unwrap `{transaction: {...}}` → `*Payment` |
| SimulatePayment | POST | `/api/paymentsimulation` | `{project, order_id, amount, api_key}` JSON | unwrap `{transaction: {...}}` → `*Payment` |

**Time parsing:** all `expired_at` / `completed_at` strings parsed with
`time.Parse(time.RFC3339Nano, s)` first, fallback to `time.RFC3339`. Empty
strings yield `nil *time.Time`. Failures yield a wrapped parse error.

**Headers (all requests):**

- `Accept: application/json`
- `Content-Type: application/json` (when body present)
- `User-Agent: pakasir-go-sdk/<version>` (overridable)

**No retries in SDK.** Callers inject retry via `http.RoundTripper`.

---

## 7. Webhook Handling Contract

Pakasir does *not* sign webhooks. The published security guidance is:

> "Saat menerima webhook pastikan amount dan order_id sesuai."

The SDK reflects this literally:

1. `ParseWebhook` / `ParseWebhookRequest` decode the JSON body into `*Event`.
2. Caller looks up the local record for `event.OrderID`.
3. Caller calls `event.Match(localOrderID, localAmount)`.
4. If `Match` returns false → respond 403 Forbidden. Do NOT process.
5. If true → process side effects in a transaction (idempotent on completion).

The SDK explicitly does **not** track idempotency — the merchant's database
is the source of truth. `Event` is plain data; no state.

### Replay & forgery mitigation

Because Pakasir does not sign webhooks, the SDK cannot cryptographically
verify origin. The full mitigation chain is the merchant's responsibility:

1. **IP allowlist at the edge.** Restrict the webhook endpoint at your CDN /
   reverse proxy / API gateway to the Pakasir source IP range. The SDK does
   not do this — `net/http` cannot reliably observe the originating client
   IP from behind a load balancer or proxy without trusting headers
   (`X-Forwarded-For`), which an attacker can spoof.
2. **Application-level idempotency.** Key on `order_id` plus terminal
   `status` in the merchant's database. A second webhook arriving for an
   `order_id` already marked `completed` must be a no-op — never trigger
   side effects (provisioning, emails, ledger entries) twice.
3. **Reject mismatch with HTTP 403, not 400 or 422.** Pakasir's retry policy
   for non-403 failures is not publicly documented; 403 signals "rejected,
   do not retry" most clearly.
4. **Constant-time `Match`/`Verify`.** Not strictly needed (no secret in
   the comparison), but the SDK still uses straightforward `==` comparison
   — there is no timing-attack surface on plain order_id/amount equality.

---

## 8. Testing Strategy

- **Unit tests** — every public function. Table-driven where possible.
- **HTTP tests** — `httptest.NewServer` returning canned Pakasir-shaped JSON
  for each endpoint. Validates request body/headers/method/path.
- **Watch tests** — fake clock + injected `time.Ticker` channel to test
  polling behaviour without sleeping.
- **Webhook tests** — golden-file JSON payloads (pending, completed,
  cancelled, sandbox) decoded and `Match`/`Verify`-validated.
- **Fuzz test** — `FuzzParseWebhook` (stdlib `testing.F`) seeded with golden
  payloads plus edge cases (empty body, malformed JSON, oversized body,
  non-UTF8, extra unknown fields). Catches panics on adversarial input.
  CI runs `go test -fuzz=FuzzParseWebhook -fuzztime=30s` on push.
- **Coverage gate** — CI fails below 80% (`go test -coverprofile=...`).
- **Race detector** — `go test -race ./...` in CI.

---

## 9. Repository Hygiene

- **`go.mod`:** `module github.com/ahnafudin/pakasir-go-sdk` / `go 1.22`
- **License:** MIT (matches Node SDK; permissive; OSS-friendly).
- **CI:** GitHub Actions matrix on OS = [ubuntu-latest, macos-latest,
  windows-latest] × Go = [1.22, 1.23]. (Windows is included because the
  author's development environment is Windows and a non-trivial portion of
  the Indonesian Go developer audience is Windows-based.)
  Steps: `go vet`, `staticcheck`, `go test -race -cover`, fuzz smoke
  (`-fuzz=FuzzParseWebhook -fuzztime=30s`), fail < 80%.
- **Releases:** SemVer; tags `vMAJOR.MINOR.PATCH`. Start at `v0.1.0`,
  promote to `v1.0.0` after dracin-go integration validates.
- **README:** single page, 7 sections (Install, Quickstart, Create payment,
  Get URL, Watch, Webhook handler, API reference link). Copy-paste examples.
- **godoc:** every exported symbol commented; package-level `doc.go` covers
  authentication, sandbox vs prod, security note on DetailPayment.
- **CHANGELOG:** Keep-a-Changelog format.

---

## 10. dracin-go Integration Plan (downstream)

After SDK ships:

1. `go get github.com/ahnafudin/pakasir-go-sdk@v0.1.0` in `dracin-go`.
2. Replace inline checkout URL builder in `billing.go:134` with
   `client.GetPaymentURL(...)`.
3. Replace `verifySignature` (currently invented HMAC) and `webhookPayload`
   struct with `pakasir.ParseWebhookRequest` + `event.Match(...)`.
4. Add `dramain.id` webhook URL to Pakasir dashboard:
   `https://api.dramain.id/api/v1/billing/webhook/pakasir`.
5. Smoke-test using `client.SimulatePayment(...)` against sandbox.

Out of scope for the SDK spec; tracked in a follow-up dracin-go change.

---

## 11. Versioning & Compatibility Promise

- **v0.x.x** — public API may change. Goal: validate via dracin-go integration.
- **v1.0.0** — API stable. SemVer enforced thereafter; breaking changes only
  on major bumps.
- **Go support window:** track Go's official policy (current + previous
  minor). Drop oldest when Google does.
- **Pakasir API changes:** non-breaking additions (new methods, new optional
  response fields) ship as minor versions. Breaking Pakasir changes ship as
  major version with a migration guide.

---

## 12. Out of Scope (deferred)

- gRPC server (revisit only if real demand surfaces).
- QR PNG rendering (delegate to `github.com/skip2/go-qrcode` if needed).
- Framework adapters (Fiber/Echo/Gin) — add as separate modules in a
  `pakasir-go-sdk-contrib` org if community asks.
- Webhook signature verification — depends on Pakasir adding signatures
  upstream first.
- Built-in idempotency tracking (database-backed) — merchant concern.

---

## 13. Risks

| Risk | Mitigation |
|---|---|
| Pakasir changes wire format silently | Integration test against live sandbox in CI (separate workflow, not blocking PRs). |
| Pakasir adds new status values (e.g. `failed`, `refunded`) | `Status.IsKnown()` lets callers detect; `IsTerminal()` returns false for unknown values, forcing explicit handling. Patch release adds the constant. |
| Two competitor SDKs dilute discoverability | Strong README + "why another SDK" section comparing all three options honestly. |
| `Watch` polling abuses API | Document min interval (1s); default 3s matches Node SDK. |
| API key leak via `DetailPayment` query string | Loud doc warning + README security note. |
| Maintenance burden post-launch | Tight scope (no gRPC/QR/i18n) limits surface area; aim for ≤10 dependent files. |

---

## 14. Success Criteria

1. `pakasir-go-sdk` v0.1.0 published with all 6 + Watch + webhook helpers.
2. `dracin-go` consumes the SDK, replacing inline Pakasir code; both bugs
   from `billing.go` are fixed in the process.
3. ≥ 80% test coverage; CI green on Go 1.22 and 1.23.
4. README example for webhook handling runs unmodified against a Pakasir
   sandbox simulation.
5. At least one external user (community star, issue, or PR) within 90 days
   of public announcement.
