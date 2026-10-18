// Package payment talks to online payment gateways. It knows nothing about
// orders or the database: a gateway starts a payment and later confirms it
// server to server, which is the only thing a sale may rely on.
package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Gateway names, as stored on orders.
const (
	Zarinpal = "zarinpal"
	IDPay    = "idpay"
)

const (
	requestTimeout  = 15 * time.Second
	maxResponseSize = 1 << 20
)

// ErrGateway marks a gateway call that failed or answered nonsense; the
// payment state is unknown and the caller may retry.
var ErrGateway = errors.New("payment gateway error")

// StartRequest opens a payment. Amount is in rials.
type StartRequest struct {
	OrderToken  string
	Amount      int64
	Description string
	CallbackURL string
}

// StartResult is where to send the customer and the id to verify later.
type StartResult struct {
	Ref    string
	PayURL string
}

// VerifyRequest asks whether Ref was paid; Amount is in rials.
type VerifyRequest struct {
	Ref        string
	OrderToken string
	Amount     int64
}

// VerifyResult is the gateway's final word. Paid false with a nil error means
// the customer did not pay (or the amount did not match).
type VerifyResult struct {
	Paid        bool
	TrackingRef string
}

// Gateway is one payment provider.
type Gateway interface {
	Start(ctx context.Context, req StartRequest) (*StartResult, error)
	Verify(ctx context.Context, req VerifyRequest) (*VerifyResult, error)
}

// RialAmount converts a plan price to rials; only IRT and IRR are payable.
func RialAmount(minor int64, currency string) (int64, bool) {
	switch currency {
	case "IRR":
		return minor, true
	case "IRT":
		return minor * 10, true
	default:
		return 0, false
	}
}

// postJSON sends body and decodes a JSON reply into out. It returns the HTTP
// status so callers can read a provider's error body on 4xx.
func postJSON(ctx context.Context, client *http.Client, url string, headers map[string]string, body, out any) (int, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrGateway, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("%w: %v", ErrGateway, err)
	}
	if resp.StatusCode >= 500 {
		return resp.StatusCode, fmt.Errorf("%w: HTTP %d", ErrGateway, resp.StatusCode)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return resp.StatusCode, fmt.Errorf("%w: unreadable reply (HTTP %d)", ErrGateway, resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func httpClient(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: requestTimeout}
}
