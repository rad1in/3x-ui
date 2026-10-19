package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

// stripeServer records the form it was sent and answers canned sessions.
type stripeServer struct {
	server  *httptest.Server
	form    url.Values
	auth    string
	idemKey string
	reply   string
	status  int
}

func newStripeServer(t *testing.T) *stripeServer {
	t.Helper()
	s := &stripeServer{status: http.StatusOK}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		s.form, _ = url.ParseQuery(string(raw))
		s.auth = r.Header.Get("Authorization")
		s.idemKey = r.Header.Get("Idempotency-Key")
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.reply))
	}))
	t.Cleanup(s.server.Close)
	return s
}

func stripeSign(secret string, ts time.Time, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	stamp := strconv.FormatInt(ts.Unix(), 10)
	mac.Write([]byte(stamp + "." + body))
	return "t=" + stamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func TestStripeStartCreatesACheckoutSessionPricedInMinorUnits(t *testing.T) {
	s := newStripeServer(t)
	s.reply = `{"id":"cs_test_1","url":"https://checkout.stripe.com/c/pay/cs_test_1"}`
	g := &StripeGateway{SecretKey: "sk_test_x", BaseURL: s.server.URL}

	got, err := g.Start(context.Background(), StartRequest{
		OrderToken: "tok", Amount: 1999, Currency: "USD", Description: "Monthly 50 GB",
		CallbackURL: "https://shop.example.com/shop/api/callback/stripe?order=tok",
		CancelURL:   "https://shop.example.com/shop/api/callback/stripe?order=tok&cancel=1",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got.Ref != "cs_test_1" || got.PayURL != "https://checkout.stripe.com/c/pay/cs_test_1" {
		t.Fatalf("result = %+v", got)
	}
	if s.auth != "Bearer sk_test_x" || s.idemKey != "checkout-tok" {
		t.Fatalf("auth = %q, idempotency key = %q", s.auth, s.idemKey)
	}
	want := map[string]string{
		"mode":                                          "payment",
		"client_reference_id":                           "tok",
		"line_items[0][quantity]":                       "1",
		"line_items[0][price_data][currency]":           "usd",
		"line_items[0][price_data][unit_amount]":        "1999",
		"line_items[0][price_data][product_data][name]": "Monthly 50 GB",
		"success_url":                                   "https://shop.example.com/shop/api/callback/stripe?order=tok",
		"cancel_url":                                    "https://shop.example.com/shop/api/callback/stripe?order=tok&cancel=1",
	}
	for k, v := range want {
		if s.form.Get(k) != v {
			t.Errorf("form[%s] = %q, want %q", k, s.form.Get(k), v)
		}
	}
}

func TestStripeStartRefusalIsAGatewayError(t *testing.T) {
	s := newStripeServer(t)
	s.status = http.StatusUnauthorized
	s.reply = `{"error":{"type":"invalid_request_error","message":"Invalid API Key provided"}}`
	g := &StripeGateway{SecretKey: "bad", BaseURL: s.server.URL}
	if _, err := g.Start(context.Background(), StartRequest{Amount: 100, Currency: "USD"}); !errors.Is(err, ErrGateway) {
		t.Fatalf("Start error = %v, want ErrGateway", err)
	}
}

func TestStripeVerifyDecidesFromTheSessionItself(t *testing.T) {
	const paid = `{"id":"cs_1","status":"complete","payment_status":"paid","amount_total":1999,"currency":"usd","client_reference_id":"tok","payment_intent":"pi_77"}`
	req := VerifyRequest{Ref: "cs_1", OrderToken: "tok", Amount: 1999, Currency: "USD"}
	tests := []struct {
		name  string
		reply string
		want  VerifyResult
	}{
		{"paid", paid, VerifyResult{Paid: true, TrackingRef: "pi_77"}},
		{"cheaper than the order", `{"status":"complete","payment_status":"paid","amount_total":199,"currency":"usd","client_reference_id":"tok"}`, VerifyResult{Mismatch: true}},
		{"other currency", `{"status":"complete","payment_status":"paid","amount_total":1999,"currency":"eur","client_reference_id":"tok"}`, VerifyResult{Mismatch: true}},
		{"another order's session", `{"status":"complete","payment_status":"paid","amount_total":1999,"currency":"usd","client_reference_id":"other"}`, VerifyResult{Mismatch: true}},
		{"still open", `{"status":"open","payment_status":"unpaid","amount_total":1999,"currency":"usd","client_reference_id":"tok"}`, VerifyResult{Pending: true}},
		{"async payment in flight", `{"status":"complete","payment_status":"unpaid","amount_total":1999,"currency":"usd","client_reference_id":"tok"}`, VerifyResult{Pending: true}},
		{"expired", `{"status":"expired","payment_status":"unpaid","amount_total":1999,"currency":"usd","client_reference_id":"tok"}`, VerifyResult{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newStripeServer(t)
			s.reply = tc.reply
			g := &StripeGateway{SecretKey: "sk", BaseURL: s.server.URL}
			got, err := g.Verify(context.Background(), req)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if *got != tc.want {
				t.Fatalf("Verify = %+v, want %+v", *got, tc.want)
			}
		})
	}
}

func TestStripeVerifyOutageIsAGatewayErrorNotAVerdict(t *testing.T) {
	s := newStripeServer(t)
	s.status = http.StatusBadGateway
	g := &StripeGateway{SecretKey: "sk", BaseURL: s.server.URL}
	if _, err := g.Verify(context.Background(), VerifyRequest{Ref: "cs_1", Amount: 1, Currency: "USD"}); !errors.Is(err, ErrGateway) {
		t.Fatalf("Verify error = %v, want ErrGateway", err)
	}
}

func TestStripeSignatureAcceptsOnlyAFreshCorrectlyKeyedBody(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	const body = `{"id":"evt_1"}`
	good := stripeSign("whsec_a", now, body)
	tests := []struct {
		name   string
		secret string
		body   string
		header string
		at     time.Time
		want   bool
	}{
		{"valid", "whsec_a", body, good, now, true},
		{"wrong secret", "whsec_b", body, good, now, false},
		{"tampered body", "whsec_a", `{"id":"evt_2"}`, good, now, false},
		{"stale replay", "whsec_a", body, good, now.Add(10 * time.Minute), false},
		{"empty secret", "", body, stripeSign("", now, body), now, false},
		{"no header", "whsec_a", body, "", now, false},
		{"second v1 matches", "whsec_a", body, "t=" + strconv.FormatInt(now.Unix(), 10) + ",v1=00," + good[len("t="+strconv.FormatInt(now.Unix(), 10)+","):], now, true},
		{"unparsable timestamp", "whsec_a", body, "t=abc,v1=00", now, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := VerifyStripeSignature(tc.secret, []byte(tc.body), tc.header, tc.at); got != tc.want {
				t.Fatalf("VerifyStripeSignature = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseStripeEventReadsTheSessionOfACheckoutEvent(t *testing.T) {
	ev, err := ParseStripeEvent([]byte(`{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"id":"cs_1","client_reference_id":"tok","payment_status":"paid"}}}`))
	if err != nil {
		t.Fatalf("ParseStripeEvent: %v", err)
	}
	if ev.Type != "checkout.session.completed" || ev.SessionID != "cs_1" || ev.OrderToken != "tok" {
		t.Fatalf("event = %+v", ev)
	}
	if _, err := ParseStripeEvent([]byte(`not json`)); !errors.Is(err, ErrGateway) {
		t.Fatalf("garbage error = %v, want ErrGateway", err)
	}
}
