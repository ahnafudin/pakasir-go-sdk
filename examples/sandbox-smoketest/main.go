// Command sandbox-smoketest validates the SDK against the live Pakasir API
// using a SANDBOX project. It exercises the full transaction lifecycle and
// asserts that every response field decodes into the SDK's typed structs as
// the docs describe.
//
// It requires a sandbox-enabled Pakasir project. Credentials are read from
// the environment and never hardcoded:
//
//	PAKASIR_SLUG     — your sandbox project slug
//	PAKASIR_API_KEY  — your sandbox project API key
//	PAKASIR_AMOUNT   — optional, transaction amount in IDR (default 10000)
//
// Run:
//
//	cd examples/sandbox-smoketest
//	PAKASIR_SLUG=... PAKASIR_API_KEY=... go run .
//
// Exit code is 0 only when every check passes.
package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	pakasir "github.com/ahnafudin/pakasir-go-sdk"
)

// checker accumulates pass/fail results so the whole lifecycle runs even if an
// individual assertion fails, then reports a single summary.
type checker struct {
	passed int
	failed int
}

func (c *checker) check(name string, ok bool, detail string) {
	if ok {
		c.passed++
		fmt.Printf("  PASS  %s\n", name)
		return
	}
	c.failed++
	fmt.Printf("  FAIL  %s — %s\n", name, detail)
}

func main() {
	slug := os.Getenv("PAKASIR_SLUG")
	apiKey := os.Getenv("PAKASIR_API_KEY")
	if slug == "" || apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: PAKASIR_SLUG and PAKASIR_API_KEY must be set")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  PAKASIR_SLUG=<sandbox-slug> PAKASIR_API_KEY=<sandbox-key> go run .")
		os.Exit(2)
	}

	amount := int64(10_000)
	if v := os.Getenv("PAKASIR_AMOUNT"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid PAKASIR_AMOUNT %q: %v\n", v, err)
			os.Exit(2)
		}
		amount = n
	}

	client := pakasir.New(slug, apiKey)
	orderID := fmt.Sprintf("SMOKE-%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c := &checker{}
	fmt.Printf("Pakasir sandbox smoke test\n  project=%s order_id=%s amount=%d\n\n", slug, orderID, amount)

	// --- GetPaymentURL (no network) ---
	fmt.Println("GetPaymentURL")
	gotURL := client.GetPaymentURL(pakasir.MethodQRIS, orderID, amount,
		pakasir.WithQRISOnly(true))
	wantPrefix := "https://app.pakasir.com/pay/" + slug + "/" + strconv.FormatInt(amount, 10)
	c.check("URL has correct base + amount path", hasPrefix(gotURL, wantPrefix), gotURL)
	c.check("URL carries order_id", contains(gotURL, "order_id="+orderID), gotURL)
	c.check("qris_only is the literal 1 (per docs B.2)", contains(gotURL, "qris_only=1"), gotURL)

	// --- CreatePayment ---
	fmt.Println("\nCreatePayment (qris)")
	created, err := client.CreatePayment(ctx, pakasir.MethodQRIS, orderID, amount)
	if err != nil {
		c.check("CreatePayment succeeds", false, err.Error())
		c.summary()
		return
	}
	c.check("CreatePayment succeeds", true, "")
	c.check("order_id echoed", created.OrderID == orderID, created.OrderID)
	c.check("amount echoed", created.Amount == amount, fmt.Sprintf("%d", created.Amount))
	c.check("payment_number present", created.PaymentNumber != "", "empty")
	c.check("total_payment >= amount", created.TotalPayment >= amount,
		fmt.Sprintf("total=%d amount=%d", created.TotalPayment, amount))
	c.check("expired_at parsed (RFC3339)", created.ExpiredAt != nil, "nil")
	c.check("method round-trips to qris", created.Method == pakasir.MethodQRIS, string(created.Method))

	// --- DetailPayment (before payment) ---
	fmt.Println("\nDetailPayment (pre-payment)")
	detail, err := client.DetailPayment(ctx, orderID, amount)
	if err != nil {
		c.check("DetailPayment succeeds", false, err.Error())
	} else {
		c.check("DetailPayment succeeds", true, "")
		c.check("status is a known value", detail.Status.IsKnown(), string(detail.Status))
		c.check("order_id matches", detail.OrderID == orderID, detail.OrderID)
	}

	// --- SimulatePayment (sandbox only) ---
	fmt.Println("\nSimulatePayment (sandbox)")
	if _, err := client.SimulatePayment(ctx, orderID, amount); err != nil {
		c.check("SimulatePayment succeeds", false, err.Error())
	} else {
		c.check("SimulatePayment succeeds", true, "")
	}

	// Give Pakasir a moment to settle the simulated payment.
	time.Sleep(2 * time.Second)

	// --- DetailPayment (after simulation) ---
	fmt.Println("\nDetailPayment (post-simulation)")
	final, err := client.DetailPayment(ctx, orderID, amount)
	if err != nil {
		c.check("DetailPayment succeeds", false, err.Error())
	} else {
		c.check("DetailPayment succeeds", true, "")
		c.check("status == completed after simulation",
			final.Status == pakasir.StatusCompleted, string(final.Status))
		c.check("completed_at parsed when completed",
			final.Status != pakasir.StatusCompleted || final.CompletedAt != nil, "nil")
		c.check("status is terminal", final.Status.IsTerminal(), string(final.Status))
	}

	c.summary()
}

func (c *checker) summary() {
	fmt.Printf("\n──────────────────────────────\n%d passed, %d failed\n", c.passed, c.failed)
	if c.failed > 0 {
		os.Exit(1)
	}
	fmt.Println("All wire-format checks passed against the live sandbox.")
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
