// Package main demonstrates a minimal Pakasir webhook receiver.
//
// Run:
//
//	cd examples/webhook && go run .
//
// Test locally:
//
//	curl -X POST -H 'Content-Type: application/json' \
//	  -d '{"order_id":"DEMO-1","amount":15000,"status":"completed","payment_method":"QRIS","is_sandbox":true}' \
//	  http://localhost:8080/webhook/pakasir
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pakasir "github.com/ahnafudin/pakasir-go-sdk"
)

func main() {
	// Seed in-memory order "database": order_id → expected amount (IDR cents).
	orders := map[string]int64{
		"DEMO-1": 15000,
		"DEMO-2": 25000,
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /webhook/pakasir", webhookHandler(orders))

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("server starting", "addr", srv.Addr)

	// Start server in background; capture any immediate bind error.
	startErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			startErr <- err
		}
	}()

	// Wait for OS signal or a bind error.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-startErr:
		slog.Error("server failed to start", "err", err)
		os.Exit(1)
	case sig := <-quit:
		slog.Info("shutdown signal received", "signal", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}

	slog.Info("server stopped")
}

// healthHandler responds 200 for liveness checks.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// webhookHandler returns an http.HandlerFunc that validates and processes
// Pakasir webhook events. orders is captured by closure; no global state.
func webhookHandler(orders map[string]int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		event, err := pakasir.ParseWebhookRequest(r)
		if err != nil {
			slog.Error("failed to parse webhook", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		expectedAmount, ok := orders[event.OrderID]
		if !ok {
			slog.Warn("unknown order_id", "order_id", event.OrderID)
			http.Error(w, "unknown order_id", http.StatusNotFound)
			return
		}

		if err := event.Verify(event.OrderID, expectedAmount); err != nil {
			switch {
			case errors.Is(err, pakasir.ErrOrderIDMismatch),
				errors.Is(err, pakasir.ErrAmountMismatch):
				slog.Warn("webhook verification failed", "err", err)
				http.Error(w, err.Error(), http.StatusForbidden)
			default:
				slog.Error("unexpected verification error", "err", err)
				http.Error(w, err.Error(), http.StatusBadRequest)
			}
			return
		}

		slog.Info("processing payment",
			"order_id", event.OrderID,
			"status", event.Status,
			"is_sandbox", event.IsSandbox,
		)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}
