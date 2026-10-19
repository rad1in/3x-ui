package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const stripeLive = "https://api.stripe.com"

// Stripe webhook events that report a paid Checkout Session.
const (
	StripeSessionCompleted    = "checkout.session.completed"
	StripeAsyncPaymentSucceed = "checkout.session.async_payment_succeeded"
)

// stripeSignatureTolerance is how far a webhook timestamp may be from now;
// it bounds how long a captured webhook can be replayed.
const stripeSignatureTolerance = 5 * time.Minute

// StripeGateway is Stripe Checkout: a hosted payment page that is verified by
// reading the Checkout Session back with the secret key.
type StripeGateway struct {
	SecretKey string
	// BaseURL overrides the API host.
	BaseURL string
	Client  *http.Client
}

func (g *StripeGateway) base() string {
	if g.BaseURL != "" {
		return g.BaseURL
	}
	return stripeLive
}

func (g *StripeGateway) headers() map[string]string {
	return map[string]string{"Authorization": "Bearer " + g.SecretKey}
}

type stripeSession struct {
	ID                string `json:"id"`
	URL               string `json:"url"`
	Status            string `json:"status"`
	PaymentStatus     string `json:"payment_status"`
	AmountTotal       int64  `json:"amount_total"`
	Currency          string `json:"currency"`
	ClientReferenceID string `json:"client_reference_id"`
	PaymentIntent     string `json:"payment_intent"`
	Error             *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Start opens a Checkout Session for req.Amount minor units of req.Currency.
// The order token doubles as the idempotency key, so a retried start cannot
// open a second session.
func (g *StripeGateway) Start(ctx context.Context, req StartRequest) (*StartResult, error) {
	form := url.Values{
		"mode":                                   {"payment"},
		"client_reference_id":                    {req.OrderToken},
		"success_url":                            {req.CallbackURL},
		"cancel_url":                             {req.CancelURL},
		"line_items[0][quantity]":                {"1"},
		"line_items[0][price_data][currency]":    {strings.ToLower(req.Currency)},
		"line_items[0][price_data][unit_amount]": {strconv.FormatInt(req.Amount, 10)},
		"line_items[0][price_data][product_data][name]": {req.Description},
	}
	headers := g.headers()
	headers["Idempotency-Key"] = "checkout-" + req.OrderToken
	var reply stripeSession
	status, err := doJSON(ctx, httpClient(g.Client), http.MethodPost, g.base()+"/v1/checkout/sessions", "application/x-www-form-urlencoded", headers, []byte(form.Encode()), &reply)
	if err != nil {
		return nil, err
	}
	if status >= 400 || reply.ID == "" || reply.URL == "" {
		return nil, fmt.Errorf("%w: stripe refused the session (HTTP %d)", ErrGateway, status)
	}
	return &StartResult{Ref: reply.ID, PayURL: reply.URL}, nil
}

// Verify reads the session back. It is paid only when Stripe says so for the
// order's own reference, amount and currency; money for anything else is a
// Mismatch for a human to settle.
func (g *StripeGateway) Verify(ctx context.Context, req VerifyRequest) (*VerifyResult, error) {
	var s stripeSession
	status, err := doJSON(ctx, httpClient(g.Client), http.MethodGet, g.base()+"/v1/checkout/sessions/"+url.PathEscape(req.Ref), "", g.headers(), nil, &s)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("%w: stripe could not read the session (HTTP %d)", ErrGateway, status)
	}
	switch {
	case s.PaymentStatus == "paid":
		if s.ClientReferenceID != req.OrderToken || s.AmountTotal != req.Amount || !strings.EqualFold(s.Currency, req.Currency) {
			return &VerifyResult{Mismatch: true}, nil
		}
		ref := s.PaymentIntent
		if ref == "" {
			ref = s.ID
		}
		return &VerifyResult{Paid: true, TrackingRef: ref}, nil
	case s.Status == "expired":
		return &VerifyResult{}, nil
	default:
		return &VerifyResult{Pending: true}, nil
	}
}

// VerifyStripeSignature checks a Stripe-Signature header: an HMAC-SHA256,
// keyed with the endpoint secret, of "timestamp.body". Any listed v1 value may
// match; an old timestamp is refused so a captured webhook cannot be replayed.
func VerifyStripeSignature(secret string, body []byte, header string, now time.Time) bool {
	if secret == "" || header == "" {
		return false
	}
	var stamp string
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		key, val, ok := strings.Cut(strings.TrimSpace(part), "=")
		switch {
		case !ok:
		case key == "t":
			stamp = val
		case key == "v1":
			sigs = append(sigs, val)
		}
	}
	secs, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return false
	}
	age := now.Sub(time.Unix(secs, 0))
	if age < -stripeSignatureTolerance || age > stripeSignatureTolerance {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stamp + "."))
	mac.Write(body)
	want := mac.Sum(nil)
	for _, sig := range sigs {
		got, err := hex.DecodeString(sig)
		if err == nil && hmac.Equal(got, want) {
			return true
		}
	}
	return false
}

// StripeEvent is the part of a webhook the shop acts on.
type StripeEvent struct {
	Type       string
	SessionID  string
	OrderToken string
}

// ParseStripeEvent reads a webhook body of a Checkout Session event.
func ParseStripeEvent(body []byte) (*StripeEvent, error) {
	var raw struct {
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID                string `json:"id"`
				ClientReferenceID string `json:"client_reference_id"`
			} `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("%w: unreadable stripe event", ErrGateway)
	}
	return &StripeEvent{Type: raw.Type, SessionID: raw.Data.Object.ID, OrderToken: raw.Data.Object.ClientReferenceID}, nil
}
