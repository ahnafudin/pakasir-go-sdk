// Package main demonstrates the pakasir-go-sdk with a QRIS payment flow.
//
// Required environment variables:
//
//	PAKASIR_SLUG    — your Pakasir project slug (e.g. "my-store")
//	PAKASIR_API_KEY — your Pakasir API key
//
// How to run:
//
//	cd examples/basic && go run .
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	pakasir "github.com/ahnafudin/pakasir-go-sdk"
)

func main() {
	slug := os.Getenv("PAKASIR_SLUG")
	apiKey := os.Getenv("PAKASIR_API_KEY")
	if slug == "" || apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: PAKASIR_SLUG and PAKASIR_API_KEY must be set")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  export PAKASIR_SLUG=<your-project-slug>")
		fmt.Fprintln(os.Stderr, "  export PAKASIR_API_KEY=<your-api-key>")
		fmt.Fprintln(os.Stderr, "  go run .")
		os.Exit(1)
	}

	client := pakasir.New(slug, apiKey)

	const amount int64 = 15_000
	orderID := fmt.Sprintf("DEMO-%d", time.Now().UnixNano())

	ctx := context.Background()

	// --- Step 1: Create payment ---
	fmt.Printf("\n==> Creating QRIS payment for %s (%s)\n", orderID, formatRupiah(amount))

	payment, err := client.CreatePayment(ctx, pakasir.MethodQRIS, orderID, amount)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: CreatePayment: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("payment_number: %s\n", payment.PaymentNumber)
	if payment.ExpiredAt != nil {
		fmt.Printf("expired_at:     %s\n", payment.ExpiredAt.Format(time.RFC3339))
	}

	// --- Step 2: Print hosted URL ---
	hostedURL := client.GetPaymentURL(pakasir.MethodQRIS, orderID, amount)
	fmt.Printf("hosted_url:     %s\n", hostedURL)

	// --- Step 3: Watch for status updates ---
	fmt.Println("\n==> Watching for status updates (max 10 min, polling every 3s)")

	watchCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	events := client.Watch(watchCtx, orderID, amount, pakasir.WithInterval(3*time.Second))

	var finalPayment *pakasir.Payment
	for evt := range events {
		if evt.Err != nil {
			fmt.Fprintf(os.Stderr, "error: Watch: %v\n", evt.Err)
			os.Exit(1)
		}
		fmt.Printf("status: %s\n", evt.Payment.Status)
		finalPayment = evt.Payment
	}

	// --- Step 4: Final outcome ---
	fmt.Println()
	if finalPayment == nil {
		fmt.Println("==> Final: watch ended without a status (context may have expired)")
		return
	}

	switch finalPayment.Status {
	case pakasir.StatusCompleted:
		ts := ""
		if finalPayment.CompletedAt != nil {
			ts = " at " + finalPayment.CompletedAt.Format(time.RFC3339)
		}
		fmt.Printf("==> Final: payment completed%s\n", ts)
	case pakasir.StatusCancelled:
		fmt.Println("==> Final: payment was cancelled")
	case pakasir.StatusExpired:
		fmt.Println("==> Final: payment expired without completion")
	default:
		fmt.Printf("==> Final: status %q (context expired or unknown terminal state)\n", finalPayment.Status)
	}
}

// formatRupiah formats an IDR integer as "Rp X,XXX".
func formatRupiah(n int64) string {
	s := fmt.Sprintf("%d", n)
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range s {
		rem := len(s) - i
		if i > 0 && rem%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return "Rp " + string(out)
}
