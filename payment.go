package pakasir

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Payment represents the merchant-facing view of a Pakasir transaction.
// Fields are populated as available — some are set only on CreatePayment
// (Fee, TotalPayment, PaymentNumber, ExpiredAt), others only after
// DetailPayment / Watch (Status, CompletedAt).
type Payment struct {
	// Project is the merchant project slug.
	Project string
	// OrderID is the merchant-assigned unique order identifier.
	OrderID string
	// Amount is the transaction amount in the smallest currency unit (IDR).
	Amount int64
	// Fee is the gateway fee charged by Pakasir. Set on CreatePayment response.
	Fee int64
	// TotalPayment is the total amount the customer pays (Amount + Fee).
	// Set on CreatePayment response.
	TotalPayment int64
	// Method is the selected payment method.
	Method Method
	// PaymentNumber is the VA number or QRIS string for the transaction.
	// Set on CreatePayment response.
	PaymentNumber string
	// Status is the current transaction status.
	// Populated by DetailPayment and Watch; may be empty on CreatePayment.
	Status Status
	// ExpiredAt is when the transaction window closes.
	// Set on CreatePayment response; nil when not returned by the API.
	ExpiredAt *time.Time
	// CompletedAt is when the transaction was paid.
	// Set when Status == StatusCompleted; nil otherwise.
	CompletedAt *time.Time
}

// paymentConfig holds resolved PaymentOption values.
type paymentConfig struct {
	redirectURL string
	qrisOnly    bool
	qrisOnlySet bool
}

// PaymentOption configures payment-related operations. Currently only
// affects GetPaymentURL (CreatePayment accepts opts for forward
// compatibility; no current options modify the create call).
type PaymentOption func(*paymentConfig)

// WithRedirectURL sets the post-payment redirect URL used by GetPaymentURL.
// The URL is included as the redirect query parameter. Empty strings are
// ignored.
func WithRedirectURL(u string) PaymentOption {
	return func(c *paymentConfig) {
		c.redirectURL = u
	}
}

// WithQRISOnly forces the hosted page to show QRIS only when only is true.
// When false, the param is omitted from the URL entirely.
func WithQRISOnly(only bool) PaymentOption {
	return func(c *paymentConfig) {
		c.qrisOnly = only
		c.qrisOnlySet = true
	}
}

// applyPaymentOpts applies PaymentOption funcs to a zero paymentConfig and
// returns the result.
func applyPaymentOpts(opts []PaymentOption) paymentConfig {
	var cfg paymentConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// --- Wire-level DTOs ---

// paymentDTO mirrors the JSON shape returned by Pakasir for both the
// "payment" and "transaction" envelope variants.
type paymentDTO struct {
	Project       string `json:"project"`
	OrderID       string `json:"order_id"`
	Amount        int64  `json:"amount"`
	Fee           int64  `json:"fee"`
	TotalPayment  int64  `json:"total_payment"`
	PaymentMethod string `json:"payment_method"`
	PaymentNumber string `json:"payment_number"`
	Status        string `json:"status"`
	ExpiredAt     string `json:"expired_at"`
	CompletedAt   string `json:"completed_at"`
}

// toPayment converts the wire DTO into a *Payment, parsing time fields where
// non-empty. Returns an error if any non-empty time string fails to parse.
func (d *paymentDTO) toPayment() (*Payment, error) {
	p := &Payment{
		Project:       d.Project,
		OrderID:       d.OrderID,
		Amount:        d.Amount,
		Fee:           d.Fee,
		TotalPayment:  d.TotalPayment,
		Method:        Method(d.PaymentMethod),
		PaymentNumber: d.PaymentNumber,
		Status:        Status(d.Status),
	}

	if d.ExpiredAt != "" {
		t, err := parseTime(d.ExpiredAt)
		if err != nil {
			return nil, fmt.Errorf("expired_at: %w", err)
		}
		p.ExpiredAt = &t
	}

	if d.CompletedAt != "" {
		t, err := parseTime(d.CompletedAt)
		if err != nil {
			return nil, fmt.Errorf("completed_at: %w", err)
		}
		p.CompletedAt = &t
	}

	return p, nil
}

// --- Response envelope types ---

// createPaymentResponse unwraps the {"payment": {...}} envelope from
// CreatePayment.
type createPaymentResponse struct {
	Payment paymentDTO `json:"payment"`
}

// transactionResponse unwraps the {"transaction": {...}} envelope from
// DetailPayment, CancelPayment, and SimulatePayment.
type transactionResponse struct {
	Transaction paymentDTO `json:"transaction"`
}

// --- POST request body ---

// postBody is the shared request body shape for all three POST endpoints:
// CreatePayment, CancelPayment, and SimulatePayment.
type postBody struct {
	Project string `json:"project"`
	OrderID string `json:"order_id"`
	Amount  int64  `json:"amount"`
	APIKey  string `json:"api_key"`
}

// --- Payment API methods ---

// CreatePayment initiates a transaction using the given payment method.
//
// POST /api/transactioncreate/{method}
// Body: {project, order_id, amount, api_key}
// Response envelope: {"payment": {...}}
//
// Returns ErrInvalidOrderID if orderID is empty, ErrInvalidAmount if
// amount is not positive, or ErrInvalidMethod if method is not a known
// Method constant. On HTTP failure, returns *APIError.
func (c *Client) CreatePayment(
	ctx context.Context,
	method Method,
	orderID string,
	amount int64,
	opts ...PaymentOption,
) (*Payment, error) {
	if orderID == "" {
		return nil, ErrInvalidOrderID
	}
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}
	if !method.IsValid() {
		return nil, ErrInvalidMethod
	}

	body := postBody{
		Project: c.slug,
		OrderID: orderID,
		Amount:  amount,
		APIKey:  c.apiKey,
	}

	var envelope createPaymentResponse
	if err := c.do(ctx, http.MethodPost, "/api/transactioncreate/"+string(method), nil, body, &envelope); err != nil {
		return nil, err
	}

	return envelope.Payment.toPayment()
}

// GetPaymentURL builds the hosted payment URL without making an API call.
//
// Format: {baseURL}/pay/{slug}/{amount}?order_id=...
//
// The method argument is accepted for API symmetry with CreatePayment but
// is not included in the URL — Pakasir's hosted page at /pay/{slug}/{amount}
// is method-agnostic. Optional PaymentOption values control additional query
// parameters: WithRedirectURL sets the redirect param, WithQRISOnly(true)
// sets qris_only=true.
//
// Returns ErrInvalidOrderID if orderID is empty or ErrInvalidAmount if
// amount is not positive (those cases return an empty string).
func (c *Client) GetPaymentURL(
	method Method,
	orderID string,
	amount int64,
	opts ...PaymentOption,
) string {
	if orderID == "" || amount <= 0 {
		return ""
	}
	// method is accepted for API symmetry; not used in the URL.
	_ = method

	cfg := applyPaymentOpts(opts)

	base := c.baseURL + "/pay/" + c.slug + "/" + strconv.FormatInt(amount, 10)

	q := url.Values{}
	q.Set("order_id", orderID)
	if cfg.redirectURL != "" {
		q.Set("redirect", cfg.redirectURL)
	}
	if cfg.qrisOnlySet && cfg.qrisOnly {
		q.Set("qris_only", "true")
	}

	return base + "?" + q.Encode()
}

// DetailPayment fetches the current transaction status.
//
// GET /api/transactiondetail?project=&amount=&order_id=&api_key=
// Response envelope: {"transaction": {...}}
//
// Returns ErrInvalidOrderID if orderID is empty, ErrInvalidAmount if
// amount is not positive. On HTTP failure, returns *APIError.
//
// SECURITY: api_key is transmitted as a URL query parameter per Pakasir's
// spec. The key may appear in HTTP access logs and proxies. Call from
// server-side code only — never from frontends or untrusted environments.
func (c *Client) DetailPayment(
	ctx context.Context,
	orderID string,
	amount int64,
) (*Payment, error) {
	if orderID == "" {
		return nil, ErrInvalidOrderID
	}
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}

	q := url.Values{}
	q.Set("project", c.slug)
	q.Set("amount", strconv.FormatInt(amount, 10))
	q.Set("order_id", orderID)
	q.Set("api_key", c.apiKey)

	var envelope transactionResponse
	if err := c.do(ctx, http.MethodGet, "/api/transactiondetail", q, nil, &envelope); err != nil {
		return nil, err
	}

	return envelope.Transaction.toPayment()
}

// CancelPayment terminates a pending transaction.
//
// POST /api/transactioncancel
// Body: {project, order_id, amount, api_key}
// Response envelope: {"transaction": {...}}
//
// Returns ErrInvalidOrderID if orderID is empty or ErrInvalidAmount if
// amount is not positive. On HTTP failure, returns *APIError.
func (c *Client) CancelPayment(
	ctx context.Context,
	orderID string,
	amount int64,
) (*Payment, error) {
	if orderID == "" {
		return nil, ErrInvalidOrderID
	}
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}

	body := postBody{
		Project: c.slug,
		OrderID: orderID,
		Amount:  amount,
		APIKey:  c.apiKey,
	}

	var envelope transactionResponse
	if err := c.do(ctx, http.MethodPost, "/api/transactioncancel", nil, body, &envelope); err != nil {
		return nil, err
	}

	return envelope.Transaction.toPayment()
}

// SimulatePayment triggers a webhook callback as if the payment completed.
// Sandbox-only on Pakasir's side; calling in production returns *APIError.
//
// POST /api/paymentsimulation
// Body: {project, order_id, amount, api_key}
// Response envelope: {"transaction": {...}}
//
// Returns ErrInvalidOrderID if orderID is empty or ErrInvalidAmount if
// amount is not positive. On HTTP failure, returns *APIError.
func (c *Client) SimulatePayment(
	ctx context.Context,
	orderID string,
	amount int64,
) (*Payment, error) {
	if orderID == "" {
		return nil, ErrInvalidOrderID
	}
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}

	body := postBody{
		Project: c.slug,
		OrderID: orderID,
		Amount:  amount,
		APIKey:  c.apiKey,
	}

	var envelope transactionResponse
	if err := c.do(ctx, http.MethodPost, "/api/paymentsimulation", nil, body, &envelope); err != nil {
		return nil, err
	}

	return envelope.Transaction.toPayment()
}
