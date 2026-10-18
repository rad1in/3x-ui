package payment

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

const (
	nowPaymentsLive    = "https://api.nowpayments.io"
	nowPaymentsSandbox = "https://api-sandbox.nowpayments.io"
)

// NOWPayments is the name of the crypto gateway on orders.
const NOWPayments = "crypto"

// NOWPayments IPN payment statuses.
const (
	NPWaiting       = "waiting"
	NPConfirming    = "confirming"
	NPConfirmed     = "confirmed"
	NPSending       = "sending"
	NPPartiallyPaid = "partially_paid"
	NPFinished      = "finished"
	NPFailed        = "failed"
	NPRefunded      = "refunded"
	NPExpired       = "expired"
)

// NOWPaymentsGateway opens NOWPayments invoices. A paid invoice is reported by
// a signed IPN webhook, so there is no Verify call.
type NOWPaymentsGateway struct {
	APIKey string
	// BaseURL overrides the live or sandbox host.
	BaseURL string
	Sandbox bool
	Client  *http.Client
}

func (g *NOWPaymentsGateway) base() string {
	switch {
	case g.BaseURL != "":
		return g.BaseURL
	case g.Sandbox:
		return nowPaymentsSandbox
	default:
		return nowPaymentsLive
	}
}

type nowPaymentsInvoice struct {
	ID         string `json:"id"`
	InvoiceURL string `json:"invoice_url"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

// Start creates an invoice priced in req.Currency (a fiat code such as USD)
// and returns its hosted payment page; the customer picks the coin there.
func (g *NOWPaymentsGateway) Start(ctx context.Context, req StartRequest) (*StartResult, error) {
	var reply nowPaymentsInvoice
	status, err := postJSON(ctx, httpClient(g.Client), g.base()+"/v1/invoice", map[string]string{"x-api-key": g.APIKey}, map[string]any{
		"price_amount":      float64(req.Amount) / 100,
		"price_currency":    strings.ToLower(req.Currency),
		"order_id":          req.OrderToken,
		"order_description": req.Description,
		"ipn_callback_url":  req.CallbackURL,
		"success_url":       req.ReturnURL,
		"cancel_url":        req.ReturnURL,
	}, &reply)
	if err != nil {
		return nil, err
	}
	if status >= 400 || reply.ID == "" || reply.InvoiceURL == "" {
		return nil, fmt.Errorf("%w: nowpayments refused the invoice (HTTP %d, %s)", ErrGateway, status, reply.Code)
	}
	return &StartResult{Ref: reply.ID, PayURL: reply.InvoiceURL}, nil
}

// IPN is the part of a NOWPayments webhook the shop acts on.
type IPN struct {
	PaymentID     string
	Status        string
	OrderID       string
	PriceAmount   string
	PriceCurrency string
	ActuallyPaid  string
	PayAmount     string
}

// ParseIPN reads a webhook body. Amounts stay textual so nothing is rounded
// before the order amount is compared.
func ParseIPN(body []byte) (*IPN, error) {
	var raw map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%w: unreadable IPN body", ErrGateway)
	}
	text := func(key string) string {
		v, ok := raw[key]
		if !ok {
			return ""
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s
		}
		return strings.Trim(string(v), `"`)
	}
	ipn := &IPN{
		PaymentID:     text("payment_id"),
		Status:        text("payment_status"),
		OrderID:       text("order_id"),
		PriceAmount:   text("price_amount"),
		PriceCurrency: strings.ToUpper(text("price_currency")),
		ActuallyPaid:  text("actually_paid"),
		PayAmount:     text("pay_amount"),
	}
	if ipn.OrderID == "" || ipn.Status == "" {
		return nil, fmt.Errorf("%w: IPN without order_id or payment_status", ErrGateway)
	}
	return ipn, nil
}

// VerifyIPNSignature checks the x-nowpayments-sig header: the lower-case hex
// HMAC-SHA512, keyed with the IPN secret, of the body re-serialised with every
// object's keys sorted. Senders differ on escaping non-ASCII text, so both
// forms are accepted; the comparison is constant time.
func VerifyIPNSignature(secret string, body []byte, signature string) bool {
	signature = strings.ToLower(strings.TrimSpace(signature))
	if secret == "" || signature == "" {
		return false
	}
	want, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	for _, asciiOnly := range []bool{false, true} {
		canonical, err := canonicalJSON(body, asciiOnly)
		if err != nil {
			return false
		}
		mac := hmac.New(sha512.New, []byte(secret))
		mac.Write(canonical)
		if hmac.Equal(mac.Sum(nil), want) {
			return true
		}
	}
	return false
}

// canonicalJSON re-serialises body compactly with object keys sorted at every
// depth and numbers kept exactly as received.
func canonicalJSON(body []byte, asciiOnly bool) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := writeCanonical(&out, v, asciiOnly); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeCanonical(out *bytes.Buffer, v any, asciiOnly bool) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeString(out, k, asciiOnly); err != nil {
				return err
			}
			out.WriteByte(':')
			if err := writeCanonical(out, t[k], asciiOnly); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	case []any:
		out.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeCanonical(out, item, asciiOnly); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case string:
		return writeString(out, t, asciiOnly)
	case json.Number:
		out.WriteString(t.String())
	case bool:
		fmt.Fprintf(out, "%t", t)
	case nil:
		out.WriteString("null")
	default:
		return fmt.Errorf("unsupported JSON value %T", v)
	}
	return nil
}

// writeString writes s as a JSON string without HTML escaping; with asciiOnly
// every non-ASCII rune becomes a \uXXXX escape.
func writeString(out *bytes.Buffer, s string, asciiOnly bool) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return err
	}
	encoded := strings.TrimSuffix(buf.String(), "\n")
	if !asciiOnly {
		out.WriteString(encoded)
		return nil
	}
	for _, r := range encoded {
		switch {
		case r < 0x80:
			out.WriteRune(r)
		case r > 0xFFFF:
			r -= 0x10000
			fmt.Fprintf(out, `\u%04x\u%04x`, 0xD800+(r>>10), 0xDC00+(r&0x3FF))
		default:
			fmt.Fprintf(out, `\u%04x`, r)
		}
	}
	return nil
}
