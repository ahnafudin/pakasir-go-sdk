package pakasir

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const goldenCompletedPayload = `{"project":"slug","order_id":"ORD-1","amount":15000,"status":"completed","payment_method":"qris","completed_at":"2024-09-10T08:07:02.819+07:00","is_sandbox":false}`

func TestParseWebhook_CompletedPayload(t *testing.T) {
	evt, err := ParseWebhook([]byte(goldenCompletedPayload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Project != "slug" {
		t.Errorf("Project = %q, want %q", evt.Project, "slug")
	}
	if evt.OrderID != "ORD-1" {
		t.Errorf("OrderID = %q, want %q", evt.OrderID, "ORD-1")
	}
	if evt.Amount != 15000 {
		t.Errorf("Amount = %d, want 15000", evt.Amount)
	}
	if evt.Status != StatusCompleted {
		t.Errorf("Status = %q, want %q", evt.Status, StatusCompleted)
	}
	if evt.PaymentMethod != MethodQRIS {
		t.Errorf("PaymentMethod = %q, want %q", evt.PaymentMethod, MethodQRIS)
	}
	if evt.IsSandbox {
		t.Error("IsSandbox = true, want false")
	}
	if evt.CompletedAt == nil {
		t.Fatal("CompletedAt is nil, want non-nil")
	}
	// "2024-09-10T08:07:02.819+07:00"
	wantTime, _ := time.Parse(time.RFC3339Nano, "2024-09-10T08:07:02.819+07:00")
	if !evt.CompletedAt.Equal(wantTime) {
		t.Errorf("CompletedAt = %v, want %v", evt.CompletedAt, wantTime)
	}
}

func TestParseWebhook_PendingPayload(t *testing.T) {
	payload := `{"project":"slug","order_id":"ORD-2","amount":5000,"status":"pending","payment_method":"qris","completed_at":"","is_sandbox":false}`
	evt, err := ParseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Status != StatusPending {
		t.Errorf("Status = %q, want %q", evt.Status, StatusPending)
	}
	if evt.CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil", evt.CompletedAt)
	}
}

func TestParseWebhook_SandboxPayload(t *testing.T) {
	payload := `{"project":"slug","order_id":"ORD-3","amount":1000,"status":"pending","payment_method":"qris","completed_at":"","is_sandbox":true}`
	evt, err := ParseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !evt.IsSandbox {
		t.Error("IsSandbox = false, want true")
	}
}

func TestParseWebhook_EmptyBody(t *testing.T) {
	cases := [][]byte{
		nil,
		[]byte(""),
		[]byte("   \n\t  "),
	}
	for _, body := range cases {
		_, err := ParseWebhook(body)
		if !errors.Is(err, ErrEmptyBody) {
			t.Errorf("body=%q: got err %v, want ErrEmptyBody", body, err)
		}
	}
}

func TestParseWebhook_MalformedJSON(t *testing.T) {
	_, err := ParseWebhook([]byte("{not json"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrEmptyBody) {
		t.Error("expected non-ErrEmptyBody error for malformed JSON")
	}
}

func TestParseWebhook_MalformedTime(t *testing.T) {
	payload := `{"project":"slug","order_id":"ORD-4","amount":1000,"status":"completed","payment_method":"qris","completed_at":"not-a-date","is_sandbox":false}`
	_, err := ParseWebhook([]byte(payload))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "completed_at") {
		t.Errorf("expected error to mention completed_at, got: %v", err)
	}
}

func TestParseWebhookRequest_ReadsBody(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(goldenCompletedPayload))
	evt, err := ParseWebhookRequest(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.OrderID != "ORD-1" {
		t.Errorf("OrderID = %q, want %q", evt.OrderID, "ORD-1")
	}
}

func TestParseWebhookRequest_BodyTooLarge(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 2<<20) // 2 MiB > default 1 MiB
	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	_, err := ParseWebhookRequest(r)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Errorf("got err %v, want ErrBodyTooLarge", err)
	}
}

func TestParseWebhookRequest_WithMaxBodySize(t *testing.T) {
	body := bytes.Repeat([]byte("a"), 100)

	// body 100 bytes, limit 50 → ErrBodyTooLarge
	r1 := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	_, err := ParseWebhookRequest(r1, WithMaxBodySize(50))
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Errorf("limit=50: got err %v, want ErrBodyTooLarge", err)
	}

	// body 100 bytes, limit 1000 → no size error (may fail JSON parse, not body-too-large)
	r2 := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	_, err = ParseWebhookRequest(r2, WithMaxBodySize(1000))
	if errors.Is(err, ErrBodyTooLarge) {
		t.Error("limit=1000: got unexpected ErrBodyTooLarge")
	}
}

// trackingBody wraps an io.ReadCloser and records whether Close was called.
type trackingBody struct {
	io.ReadCloser
	closed int
}

func (b *trackingBody) Close() error {
	b.closed++
	return b.ReadCloser.Close()
}

func TestParseWebhookRequest_ClosesBody(t *testing.T) {
	inner := io.NopCloser(strings.NewReader(goldenCompletedPayload))
	tb := &trackingBody{ReadCloser: inner}

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Body = tb

	_, _ = ParseWebhookRequest(r)

	if tb.closed == 0 {
		t.Error("expected r.Body.Close() to be called at least once")
	}
}

func TestMatch_HappyPath(t *testing.T) {
	e := &Event{OrderID: "X", Amount: 100}
	if !e.Match("X", 100) {
		t.Error("Match returned false, want true")
	}
}

func TestMatch_OrderIDMismatch(t *testing.T) {
	e := &Event{OrderID: "X", Amount: 100}
	if e.Match("Y", 100) {
		t.Error("Match returned true, want false")
	}
}

func TestMatch_AmountMismatch(t *testing.T) {
	e := &Event{OrderID: "X", Amount: 100}
	if e.Match("X", 99) {
		t.Error("Match returned true, want false")
	}
}

func TestVerify_HappyPath(t *testing.T) {
	e := &Event{OrderID: "X", Amount: 100}
	if err := e.Verify("X", 100); err != nil {
		t.Errorf("Verify returned error %v, want nil", err)
	}
}

func TestVerify_OrderIDMismatch(t *testing.T) {
	e := &Event{OrderID: "X", Amount: 100}
	err := e.Verify("Y", 100)
	if !errors.Is(err, ErrOrderIDMismatch) {
		t.Errorf("got err %v, want errors.Is ErrOrderIDMismatch", err)
	}
	if errors.Is(err, ErrAmountMismatch) {
		t.Error("got unexpected ErrAmountMismatch in order_id mismatch case")
	}
}

func TestVerify_AmountMismatch(t *testing.T) {
	e := &Event{OrderID: "X", Amount: 100}
	err := e.Verify("X", 99)
	if !errors.Is(err, ErrAmountMismatch) {
		t.Errorf("got err %v, want errors.Is ErrAmountMismatch", err)
	}
}

func TestVerify_BothMismatch_ReturnsOrderIDOnly(t *testing.T) {
	e := &Event{OrderID: "X", Amount: 100}
	err := e.Verify("Y", 99)
	if !errors.Is(err, ErrOrderIDMismatch) {
		t.Errorf("got err %v, want ErrOrderIDMismatch", err)
	}
	if errors.Is(err, ErrAmountMismatch) {
		t.Error("both-mismatch must return ErrOrderIDMismatch only, not ErrAmountMismatch")
	}
}
