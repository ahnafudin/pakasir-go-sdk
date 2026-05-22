// Package pakasir is a Go client for the Pakasir payment gateway (https://pakasir.com).
//
// # Overview
//
// pakasir provides a lean, idiomatic, zero-dependency interface to Pakasir's
// REST API. It supports the full set of public endpoints (create / detail /
// cancel / simulate transactions, plus the hosted payment URL builder), a
// real-time polling helper for "waiting for payment" UIs, and a webhook
// validator that follows Pakasir's official security guidance.
//
// # Authentication
//
// Pakasir uses two credentials: a project slug (public identifier of the
// merchant project) and an API key (secret). Both are obtained from the
// Pakasir dashboard. Construct a Client with:
//
//	client := pakasir.New("your-slug", "your-api-key")
//
// The Client is safe for concurrent use by multiple goroutines.
//
// # Sandbox vs Production
//
// Pakasir does not publish a separate sandbox host. Sandbox is enabled
// per-project in the dashboard, and the webhook payload carries an
// IsSandbox flag. There is no need (and no option) to switch base URLs
// for sandbox testing — use a sandbox-enabled project with its own API key.
//
// # Security note on DetailPayment
//
// Per Pakasir's REST spec, DetailPayment transmits the api_key as a URL
// query parameter. The key may appear in HTTP access logs, reverse proxy
// logs, and browser history. Call DetailPayment only from trusted
// server-side code — never from frontends or untrusted environments.
//
// # Webhook handling
//
// Pakasir does not cryptographically sign webhooks. The SDK provides
// ParseWebhook and Event.Match / Event.Verify, matching Pakasir's official
// guidance: validate that order_id and amount match the merchant's local
// record of the transaction. Reject mismatches with HTTP 403. For
// defense-in-depth (replay / forgery), restrict the webhook endpoint at
// your edge to Pakasir's source IPs and enforce idempotency in your
// database.
//
// # Polling for status updates
//
// For "waiting for payment" UIs, Client.Watch returns a channel of
// WatchEvent that polls DetailPayment until the status becomes terminal,
// a permanent error occurs, or ctx is cancelled. The channel is closed
// exactly once. The first poll fires immediately; consecutive polls
// returning the same status are deduplicated. Bound the total wait time
// with context.WithTimeout on the parent ctx — there is no separate
// timeout option on Watch.
//
// # No retries by default
//
// The SDK does not retry failed requests. To add retries, inject a custom
// http.RoundTripper via WithHTTPClient. This keeps SDK behaviour
// deterministic and testable.
package pakasir
