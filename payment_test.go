package pakasir

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newTestClient creates a Client pointed at the given httptest server URL.
func newTestClient(serverURL string) *Client {
	return New("slug", "key", WithBaseURL(serverURL))
}

// --- A. GetPaymentURL ---

func TestGetPaymentURL_BuildsCorrectly(t *testing.T) {
	t.Parallel()

	c := New("slug", "key", WithBaseURL("https://app.pakasir.com"))

	cases := []struct {
		name     string
		method   Method
		orderID  string
		amount   int64
		opts     []PaymentOption
		wantBase string
		wantQ    url.Values // expected query params (subset check)
		wantKeys []string   // params that MUST be absent
	}{
		{
			name:     "basic",
			method:   MethodQRIS,
			orderID:  "ORD-1",
			amount:   1000,
			wantBase: "https://app.pakasir.com/pay/slug/1000",
			wantQ:    url.Values{"order_id": {"ORD-1"}},
			wantKeys: []string{"redirect", "qris_only"},
		},
		{
			name:    "with-redirect",
			method:  MethodQRIS,
			orderID: "ORD-1",
			amount:  1000,
			opts:    []PaymentOption{WithRedirectURL("https://x.example/done")},
			wantBase: "https://app.pakasir.com/pay/slug/1000",
			wantQ:   url.Values{"order_id": {"ORD-1"}, "redirect": {"https://x.example/done"}},
		},
		{
			name:    "with-qris-only-true",
			method:  MethodQRIS,
			orderID: "ORD-1",
			amount:  1000,
			opts:    []PaymentOption{WithQRISOnly(true)},
			wantBase: "https://app.pakasir.com/pay/slug/1000",
			wantQ:   url.Values{"order_id": {"ORD-1"}, "qris_only": {"true"}},
		},
		{
			name:    "with-qris-only-false",
			method:  MethodQRIS,
			orderID: "ORD-1",
			amount:  1000,
			opts:    []PaymentOption{WithQRISOnly(false)},
			wantBase: "https://app.pakasir.com/pay/slug/1000",
			wantQ:   url.Values{"order_id": {"ORD-1"}},
			wantKeys: []string{"qris_only"},
		},
		{
			name:    "special-chars-in-order-id",
			method:  MethodQRIS,
			orderID: "ORD/1 2",
			amount:  500,
			wantBase: "https://app.pakasir.com/pay/slug/500",
			wantQ:   url.Values{"order_id": {"ORD/1 2"}},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := c.GetPaymentURL(tc.method, tc.orderID, tc.amount, tc.opts...)

			// Parse the result to validate parts independently.
			parsed, err := url.Parse(got)
			if err != nil {
				t.Fatalf("parse URL %q: %v", got, err)
			}

			// Verify base (scheme + host + path).
			base := parsed.Scheme + "://" + parsed.Host + parsed.Path
			if base != tc.wantBase {
				t.Errorf("base = %q, want %q", base, tc.wantBase)
			}

			// Verify expected query params are present and correct.
			q := parsed.Query()
			for k, vals := range tc.wantQ {
				if got := q.Get(k); got != vals[0] {
					t.Errorf("query %s = %q, want %q", k, got, vals[0])
				}
			}

			// Verify absent params.
			for _, k := range tc.wantKeys {
				if q.Has(k) {
					t.Errorf("query param %q should be absent, got %q", k, q.Get(k))
				}
			}
		})
	}
}

func TestGetPaymentURL_EmptyOrderID_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	c := New("slug", "key")
	if got := c.GetPaymentURL(MethodQRIS, "", 1000); got != "" {
		t.Errorf("GetPaymentURL(empty orderID) = %q, want empty", got)
	}
}

func TestGetPaymentURL_ZeroAmount_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	c := New("slug", "key")
	if got := c.GetPaymentURL(MethodQRIS, "ORD-1", 0); got != "" {
		t.Errorf("GetPaymentURL(zero amount) = %q, want empty", got)
	}
}

// --- B. CreatePayment_Success ---

func TestCreatePayment_Success(t *testing.T) {
	t.Parallel()

	var gotMethod, gotPath string
	var gotBody postBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path

		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{
			"payment": {
				"project": "slug",
				"order_id": "O1",
				"amount": 1000,
				"fee": 50,
				"total_payment": 1050,
				"payment_method": "qris",
				"payment_number": "00012345",
				"expired_at": "2024-12-31T23:59:00+07:00"
			}
		}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	p, err := c.CreatePayment(context.Background(), MethodQRIS, "O1", 1000)
	if err != nil {
		t.Fatalf("CreatePayment err: %v", err)
	}

	// Verify request shape.
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/transactioncreate/qris" {
		t.Errorf("path = %q, want /api/transactioncreate/qris", gotPath)
	}
	if gotBody.Project != "slug" || gotBody.OrderID != "O1" || gotBody.Amount != 1000 || gotBody.APIKey != "key" {
		t.Errorf("body = %+v", gotBody)
	}

	// Verify returned Payment.
	if p.Project != "slug" {
		t.Errorf("Project = %q", p.Project)
	}
	if p.OrderID != "O1" {
		t.Errorf("OrderID = %q", p.OrderID)
	}
	if p.Amount != 1000 {
		t.Errorf("Amount = %d", p.Amount)
	}
	if p.Fee != 50 {
		t.Errorf("Fee = %d", p.Fee)
	}
	if p.TotalPayment != 1050 {
		t.Errorf("TotalPayment = %d", p.TotalPayment)
	}
	if p.Method != MethodQRIS {
		t.Errorf("Method = %q", p.Method)
	}
	if p.PaymentNumber != "00012345" {
		t.Errorf("PaymentNumber = %q", p.PaymentNumber)
	}
	if p.ExpiredAt == nil {
		t.Fatal("ExpiredAt is nil, want non-nil")
	}
	// Verify the parsed time is correct (2024-12-31T23:59:00+07:00).
	if p.ExpiredAt.Hour() != 23 || p.ExpiredAt.Minute() != 59 {
		t.Errorf("ExpiredAt = %v; want 23:59 local", p.ExpiredAt)
	}
}

// --- C. CreatePayment_ValidatesInputs ---

func TestCreatePayment_ValidatesInputs(t *testing.T) {
	t.Parallel()

	// Use a server that should never be reached.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected request to server")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	cases := []struct {
		name    string
		method  Method
		orderID string
		amount  int64
		wantErr error
	}{
		{"empty-order-id", MethodQRIS, "", 1000, ErrInvalidOrderID},
		{"negative-amount", MethodQRIS, "O1", -1, ErrInvalidAmount},
		{"zero-amount", MethodQRIS, "O1", 0, ErrInvalidAmount},
		{"invalid-method", Method("unknown"), "O1", 1000, ErrInvalidMethod},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := c.CreatePayment(context.Background(), tc.method, tc.orderID, tc.amount)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// --- D. CreatePayment_APIError ---

func TestCreatePayment_APIError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"message":"bad"}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.CreatePayment(context.Background(), MethodQRIS, "O1", 1000)
	if err == nil {
		t.Fatal("want error, got nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err %v is not *APIError", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
	if apiErr.Message != "bad" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "bad")
	}
}

// --- E. DetailPayment_Success ---

func TestDetailPayment_Success(t *testing.T) {
	t.Parallel()

	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		gotQuery = r.URL.Query()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{
			"transaction": {
				"project": "slug",
				"order_id": "O1",
				"amount": 1000,
				"status": "completed",
				"payment_method": "qris",
				"completed_at": "2024-12-31T23:00:00+07:00"
			}
		}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	p, err := c.DetailPayment(context.Background(), "O1", 1000)
	if err != nil {
		t.Fatalf("DetailPayment err: %v", err)
	}

	// Verify request query params.
	if gotQuery.Get("project") != "slug" {
		t.Errorf("query project = %q", gotQuery.Get("project"))
	}
	if gotQuery.Get("amount") != "1000" {
		t.Errorf("query amount = %q", gotQuery.Get("amount"))
	}
	if gotQuery.Get("order_id") != "O1" {
		t.Errorf("query order_id = %q", gotQuery.Get("order_id"))
	}
	if gotQuery.Get("api_key") != "key" {
		t.Errorf("query api_key = %q", gotQuery.Get("api_key"))
	}

	// Verify returned Payment.
	if p.Status != StatusCompleted {
		t.Errorf("Status = %q, want %q", p.Status, StatusCompleted)
	}
	if p.CompletedAt == nil {
		t.Fatal("CompletedAt is nil, want non-nil")
	}
	if p.Method != MethodQRIS {
		t.Errorf("Method = %q, want %q", p.Method, MethodQRIS)
	}
}

// --- F. DetailPayment_ValidatesInputs ---

func TestDetailPayment_ValidatesInputs(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected request to server")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	cases := []struct {
		name    string
		orderID string
		amount  int64
		wantErr error
	}{
		{"empty-order-id", "", 1000, ErrInvalidOrderID},
		{"negative-amount", "O1", -1, ErrInvalidAmount},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := c.DetailPayment(context.Background(), tc.orderID, tc.amount)
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// --- G. CancelPayment_Success ---

func TestCancelPayment_Success(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotBody postBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		gotPath = r.URL.Path

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{
			"transaction": {
				"project": "slug",
				"order_id": "O1",
				"amount": 1000,
				"status": "cancelled",
				"payment_method": "qris"
			}
		}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	p, err := c.CancelPayment(context.Background(), "O1", 1000)
	if err != nil {
		t.Fatalf("CancelPayment err: %v", err)
	}

	if gotPath != "/api/transactioncancel" {
		t.Errorf("path = %q, want /api/transactioncancel", gotPath)
	}
	if gotBody.Project != "slug" || gotBody.OrderID != "O1" || gotBody.Amount != 1000 || gotBody.APIKey != "key" {
		t.Errorf("body = %+v", gotBody)
	}
	if p.Status != StatusCancelled {
		t.Errorf("Status = %q, want %q", p.Status, StatusCancelled)
	}
}

// --- H. SimulatePayment_Success ---

func TestSimulatePayment_Success(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotBody postBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		gotPath = r.URL.Path

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := json.Unmarshal(raw, &gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{
			"transaction": {
				"project": "slug",
				"order_id": "O1",
				"amount": 1000,
				"status": "completed",
				"payment_method": "bri_va",
				"completed_at": "2024-12-31T20:00:00+07:00"
			}
		}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	p, err := c.SimulatePayment(context.Background(), "O1", 1000)
	if err != nil {
		t.Fatalf("SimulatePayment err: %v", err)
	}

	if gotPath != "/api/paymentsimulation" {
		t.Errorf("path = %q, want /api/paymentsimulation", gotPath)
	}
	if gotBody.Project != "slug" || gotBody.OrderID != "O1" || gotBody.Amount != 1000 || gotBody.APIKey != "key" {
		t.Errorf("body = %+v", gotBody)
	}
	if p.Status != StatusCompleted {
		t.Errorf("Status = %q, want completed", p.Status)
	}
	if p.CompletedAt == nil {
		t.Error("CompletedAt is nil, want non-nil")
	}
	if p.Method != MethodBRIVA {
		t.Errorf("Method = %q, want %q", p.Method, MethodBRIVA)
	}
}

// --- I. TimeFieldsEmpty_AreNil ---

func TestTimeFieldsEmpty_AreNil(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{
			"payment": {
				"project": "slug",
				"order_id": "O1",
				"amount": 1000,
				"payment_method": "qris",
				"expired_at": "",
				"completed_at": ""
			}
		}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	p, err := c.CreatePayment(context.Background(), MethodQRIS, "O1", 1000)
	if err != nil {
		t.Fatalf("CreatePayment err: %v", err)
	}
	if p.ExpiredAt != nil {
		t.Errorf("ExpiredAt = %v, want nil", p.ExpiredAt)
	}
	if p.CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil", p.CompletedAt)
	}
}

// --- J. TimeFieldsMalformed_ReturnError ---

func TestTimeFieldsMalformed_ReturnError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{
			"payment": {
				"project": "slug",
				"order_id": "O1",
				"amount": 1000,
				"payment_method": "qris",
				"expired_at": "not-a-date"
			}
		}`)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.CreatePayment(context.Background(), MethodQRIS, "O1", 1000)
	if err == nil {
		t.Fatal("want error for malformed expired_at, got nil")
	}
	if !strings.Contains(err.Error(), "expired_at") {
		t.Errorf("error should mention field name 'expired_at': %v", err)
	}
}

// --- Additional: CancelPayment / SimulatePayment validation ---

func TestCancelPayment_ValidatesInputs(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected request")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	cases := []struct {
		name    string
		orderID string
		amount  int64
		wantErr error
	}{
		{"empty-order-id", "", 100, ErrInvalidOrderID},
		{"zero-amount", "O1", 0, ErrInvalidAmount},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := c.CancelPayment(context.Background(), tc.orderID, tc.amount)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestSimulatePayment_ValidatesInputs(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected request")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	cases := []struct {
		name    string
		orderID string
		amount  int64
		wantErr error
	}{
		{"empty-order-id", "", 100, ErrInvalidOrderID},
		{"negative-amount", "O1", -5, ErrInvalidAmount},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := c.SimulatePayment(context.Background(), tc.orderID, tc.amount)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
