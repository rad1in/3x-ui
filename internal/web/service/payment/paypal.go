package payment

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	paypalLive    = "https://api-m.paypal.com"
	paypalSandbox = "https://api-m.sandbox.paypal.com"
)

// maxPayPalDescription is PayPal's limit on a purchase unit description.
const maxPayPalDescription = 127

// PayPalGateway is PayPal Orders v2: the customer approves the order at
// PayPal, and the shop captures it only after checking it is the order it
// created, for the right price.
type PayPalGateway struct {
	ClientID     string
	ClientSecret string
	// BaseURL overrides the live or sandbox host.
	BaseURL string
	Sandbox bool
	Client  *http.Client
}

func (g *PayPalGateway) base() string {
	switch {
	case g.BaseURL != "":
		return g.BaseURL
	case g.Sandbox:
		return paypalSandbox
	default:
		return paypalLive
	}
}

type paypalAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type paypalOrder struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	PurchaseUnits []struct {
		CustomID string       `json:"custom_id"`
		Amount   paypalAmount `json:"amount"`
		Payments struct {
			Captures []struct {
				ID     string       `json:"id"`
				Status string       `json:"status"`
				Amount paypalAmount `json:"amount"`
			} `json:"captures"`
		} `json:"payments"`
	} `json:"purchase_units"`
	Links []struct {
		Href string `json:"href"`
		Rel  string `json:"rel"`
	} `json:"links"`
	Name    string `json:"name"`
	Details []struct {
		Issue string `json:"issue"`
	} `json:"details"`
}

// accessToken exchanges the client credentials for a short-lived bearer token.
func (g *PayPalGateway) accessToken(ctx context.Context) (string, error) {
	var reply struct {
		AccessToken string `json:"access_token"`
	}
	basic := base64.StdEncoding.EncodeToString([]byte(g.ClientID + ":" + g.ClientSecret))
	status, err := doJSON(ctx, httpClient(g.Client), http.MethodPost, g.base()+"/v1/oauth2/token", "application/x-www-form-urlencoded",
		map[string]string{"Authorization": "Basic " + basic}, []byte(url.Values{"grant_type": {"client_credentials"}}.Encode()), &reply)
	if err != nil {
		return "", err
	}
	if status >= 400 || reply.AccessToken == "" {
		return "", fmt.Errorf("%w: paypal refused the credentials (HTTP %d)", ErrGateway, status)
	}
	return reply.AccessToken, nil
}

// Start creates a CAPTURE order for req.Amount minor units of req.Currency.
// The order token is the request id, so a retried start returns the same order.
func (g *PayPalGateway) Start(ctx context.Context, req StartRequest) (*StartResult, error) {
	token, err := g.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	description := req.Description
	if len(description) > maxPayPalDescription {
		description = description[:maxPayPalDescription]
	}
	body := map[string]any{
		"intent": "CAPTURE",
		"purchase_units": []any{map[string]any{
			"custom_id":   req.OrderToken,
			"description": description,
			"amount":      paypalAmount{CurrencyCode: strings.ToUpper(req.Currency), Value: decimalFromMinor(req.Amount)},
		}},
		"payment_source": map[string]any{"paypal": map[string]any{"experience_context": map[string]any{
			"return_url":          req.CallbackURL,
			"cancel_url":          req.CancelURL,
			"user_action":         "PAY_NOW",
			"shipping_preference": "NO_SHIPPING",
		}}},
	}
	var reply paypalOrder
	status, err := postJSON(ctx, httpClient(g.Client), g.base()+"/v2/checkout/orders",
		map[string]string{"Authorization": "Bearer " + token, "PayPal-Request-Id": "order-" + req.OrderToken}, body, &reply)
	if err != nil {
		return nil, err
	}
	payURL := ""
	for _, l := range reply.Links {
		if l.Rel == "payer-action" || (l.Rel == "approve" && payURL == "") {
			payURL = l.Href
		}
	}
	if status >= 400 || reply.ID == "" || payURL == "" {
		return nil, fmt.Errorf("%w: paypal refused the order (HTTP %d)", ErrGateway, status)
	}
	return &StartResult{Ref: reply.ID, PayURL: payURL}, nil
}

// Verify reads the order back and, once the customer approved it and it is the
// order's own reference, amount and currency, captures it. A completed capture
// for the exact amount is the only Paid answer; the capture request id is
// stable per order, so asking twice never charges twice.
func (g *PayPalGateway) Verify(ctx context.Context, req VerifyRequest) (*VerifyResult, error) {
	token, err := g.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	auth := map[string]string{"Authorization": "Bearer " + token}
	var order paypalOrder
	status, err := doJSON(ctx, httpClient(g.Client), http.MethodGet, g.base()+"/v2/checkout/orders/"+url.PathEscape(req.Ref), "", auth, nil, &order)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("%w: paypal could not read the order (HTTP %d)", ErrGateway, status)
	}
	if !order.matches(req) {
		return &VerifyResult{Mismatch: true}, nil
	}
	if order.Status == "APPROVED" {
		order, err = g.capture(ctx, auth, req)
		if err != nil {
			return nil, err
		}
		if order.Name == "INSTRUMENT_DECLINED" {
			return &VerifyResult{}, nil
		}
	}
	return order.outcome(req), nil
}

// capture takes the approved money. A declined instrument is a verdict, any
// other failure leaves the payment undecided for a retry.
func (g *PayPalGateway) capture(ctx context.Context, auth map[string]string, req VerifyRequest) (paypalOrder, error) {
	headers := map[string]string{"Authorization": auth["Authorization"], "PayPal-Request-Id": "capture-" + req.OrderToken}
	var order paypalOrder
	status, err := doJSON(ctx, httpClient(g.Client), http.MethodPost, g.base()+"/v2/checkout/orders/"+url.PathEscape(req.Ref)+"/capture", "application/json", headers, []byte("{}"), &order)
	if err != nil {
		return order, err
	}
	if status == http.StatusUnprocessableEntity {
		for _, d := range order.Details {
			if d.Issue == "INSTRUMENT_DECLINED" {
				return paypalOrder{Name: "INSTRUMENT_DECLINED"}, nil
			}
		}
	}
	if status >= 400 {
		return order, fmt.Errorf("%w: paypal could not capture the order (HTTP %d)", ErrGateway, status)
	}
	return order, nil
}

// matches reports whether the order is the one the shop created for req.
func (o *paypalOrder) matches(req VerifyRequest) bool {
	if len(o.PurchaseUnits) != 1 {
		return false
	}
	unit := o.PurchaseUnits[0]
	minor, ok := minorFromDecimal(unit.Amount.Value)
	return ok && unit.CustomID == req.OrderToken && minor == req.Amount && strings.EqualFold(unit.Amount.CurrencyCode, req.Currency)
}

// outcome reads the verdict off an order that has been (or is being) captured.
func (o *paypalOrder) outcome(req VerifyRequest) *VerifyResult {
	switch o.Status {
	case "COMPLETED":
	case "VOIDED":
		return &VerifyResult{}
	default:
		return &VerifyResult{Pending: true}
	}
	if len(o.PurchaseUnits) != 1 {
		return &VerifyResult{Mismatch: true}
	}
	for _, c := range o.PurchaseUnits[0].Payments.Captures {
		minor, ok := minorFromDecimal(c.Amount.Value)
		switch {
		case c.Status == "COMPLETED" && ok && minor == req.Amount && strings.EqualFold(c.Amount.CurrencyCode, req.Currency):
			return &VerifyResult{Paid: true, TrackingRef: c.ID}
		case c.Status == "COMPLETED":
			return &VerifyResult{Mismatch: true}
		case c.Status == "PENDING":
			return &VerifyResult{Pending: true}
		}
	}
	return &VerifyResult{}
}

// minorFromDecimal reads a price such as "19.99" as 1999 minor units. More
// than two decimals, signs and exponents are refused rather than rounded.
func minorFromDecimal(text string) (int64, bool) {
	whole, frac, _ := strings.Cut(strings.TrimSpace(text), ".")
	if whole == "" || len(frac) > 2 {
		return 0, false
	}
	for _, part := range []string{whole, frac} {
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, false
			}
		}
	}
	n, err := strconv.ParseInt(whole+frac+strings.Repeat("0", 2-len(frac)), 10, 64)
	return n, err == nil
}

// decimalFromMinor writes minor units as a two-decimal price.
func decimalFromMinor(minor int64) string {
	return fmt.Sprintf("%d.%02d", minor/100, minor%100)
}
