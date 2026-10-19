package payment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// paypalServer is a PayPal whose order reads and captures answer as set.
type paypalServer struct {
	server   *httptest.Server
	mu       sync.Mutex
	calls    []string
	created  map[string]any
	headers  map[string]http.Header
	tokenErr bool
	order    string // reply to GET /v2/checkout/orders/ORD-1
	capture  string // reply to POST .../capture
	capCode  int
}

func newPayPalServer(t *testing.T) *paypalServer {
	t.Helper()
	p := &paypalServer{headers: map[string]http.Header{}, capCode: http.StatusCreated}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		key := r.Method + " " + r.URL.Path
		p.calls = append(p.calls, key)
		p.headers[key] = r.Header.Clone()
		raw, _ := io.ReadAll(r.Body)
		switch key {
		case "POST /v1/oauth2/token":
			if p.tokenErr {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"A21-token","expires_in":32400}`))
		case "POST /v2/checkout/orders":
			_ = json.Unmarshal(raw, &p.created)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"ORD-1","status":"PAYER_ACTION_REQUIRED","links":[{"href":"https://api.paypal.test/self","rel":"self"},{"href":"https://www.paypal.test/checkoutnow?token=ORD-1","rel":"payer-action"}]}`))
		case "GET /v2/checkout/orders/ORD-1":
			_, _ = w.Write([]byte(p.order))
		case "POST /v2/checkout/orders/ORD-1/capture":
			w.WriteHeader(p.capCode)
			_, _ = w.Write([]byte(p.capture))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(p.server.Close)
	return p
}

func (p *paypalServer) called(key string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.calls {
		if c == key {
			return true
		}
	}
	return false
}

func paypalOrderJSON(status, custom, value, currency, captureStatus string) string {
	caps := ""
	if captureStatus != "" {
		caps = `,"payments":{"captures":[{"id":"CAP-9","status":"` + captureStatus + `","amount":{"currency_code":"` + currency + `","value":"` + value + `"}}]}`
	}
	return `{"id":"ORD-1","status":"` + status + `","purchase_units":[{"custom_id":"` + custom + `","amount":{"currency_code":"` + currency + `","value":"` + value + `"}` + caps + `}]}`
}

func TestPayPalStartCreatesACaptureOrderAndReturnsThePayerActionLink(t *testing.T) {
	p := newPayPalServer(t)
	g := &PayPalGateway{ClientID: "cid", ClientSecret: "sec", BaseURL: p.server.URL}

	got, err := g.Start(context.Background(), StartRequest{
		OrderToken: "tok", Amount: 1999, Currency: "USD", Description: "Monthly",
		CallbackURL: "https://shop.example.com/shop/api/callback/paypal?order=tok",
		CancelURL:   "https://shop.example.com/shop/api/callback/paypal?order=tok&cancel=1",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got.Ref != "ORD-1" || got.PayURL != "https://www.paypal.test/checkoutnow?token=ORD-1" {
		t.Fatalf("result = %+v", got)
	}
	if h := p.headers["POST /v1/oauth2/token"].Get("Authorization"); h != "Basic Y2lkOnNlYw==" {
		t.Fatalf("token auth = %q, want basic cid:sec", h)
	}
	h := p.headers["POST /v2/checkout/orders"]
	if h.Get("Authorization") != "Bearer A21-token" || h.Get("PayPal-Request-Id") != "order-tok" {
		t.Fatalf("order headers = %v", h)
	}
	raw, _ := json.Marshal(p.created)
	for _, want := range []string{`"intent":"CAPTURE"`, `"custom_id":"tok"`, `"currency_code":"USD"`, `"value":"19.99"`,
		`"return_url":"https://shop.example.com/shop/api/callback/paypal?order=tok"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("order body %s lacks %s", raw, want)
		}
	}
	ctx := p.created["payment_source"].(map[string]any)["paypal"].(map[string]any)["experience_context"].(map[string]any)
	if ctx["cancel_url"] != "https://shop.example.com/shop/api/callback/paypal?order=tok&cancel=1" {
		t.Errorf("cancel_url = %v", ctx["cancel_url"])
	}
}

func TestPayPalBadCredentialsAreAGatewayError(t *testing.T) {
	p := newPayPalServer(t)
	p.tokenErr = true
	g := &PayPalGateway{ClientID: "x", ClientSecret: "y", BaseURL: p.server.URL}
	if _, err := g.Start(context.Background(), StartRequest{Amount: 100, Currency: "USD"}); !errors.Is(err, ErrGateway) {
		t.Fatalf("Start error = %v, want ErrGateway", err)
	}
}

func TestPayPalVerifyCapturesAnApprovedOrderOnlyWhenItMatches(t *testing.T) {
	req := VerifyRequest{Ref: "ORD-1", OrderToken: "tok", Amount: 1999, Currency: "USD"}
	tests := []struct {
		name        string
		order       string
		capture     string
		capCode     int
		want        VerifyResult
		wantCapture bool
	}{
		{"approved then captured", paypalOrderJSON("APPROVED", "tok", "19.99", "USD", ""), paypalOrderJSON("COMPLETED", "tok", "19.99", "USD", "COMPLETED"), http.StatusCreated, VerifyResult{Paid: true, TrackingRef: "CAP-9"}, true},
		{"already completed is not captured again", paypalOrderJSON("COMPLETED", "tok", "19.99", "USD", "COMPLETED"), "", 0, VerifyResult{Paid: true, TrackingRef: "CAP-9"}, false},
		{"order for a lower price is never captured", paypalOrderJSON("APPROVED", "tok", "0.99", "USD", ""), "", 0, VerifyResult{Mismatch: true}, false},
		{"order of another currency is never captured", paypalOrderJSON("APPROVED", "tok", "19.99", "EUR", ""), "", 0, VerifyResult{Mismatch: true}, false},
		{"order of another customer is never captured", paypalOrderJSON("APPROVED", "other", "19.99", "USD", ""), "", 0, VerifyResult{Mismatch: true}, false},
		{"captured for less than the order", paypalOrderJSON("APPROVED", "tok", "19.99", "USD", ""), `{"status":"COMPLETED","purchase_units":[{"custom_id":"tok","amount":{"currency_code":"USD","value":"19.99"},"payments":{"captures":[{"id":"CAP-9","status":"COMPLETED","amount":{"currency_code":"USD","value":"1.00"}}]}}]}`, http.StatusCreated, VerifyResult{Mismatch: true}, true},
		{"customer has not approved yet", paypalOrderJSON("CREATED", "tok", "19.99", "USD", ""), "", 0, VerifyResult{Pending: true}, false},
		{"capture still pending review", paypalOrderJSON("COMPLETED", "tok", "19.99", "USD", "PENDING"), "", 0, VerifyResult{Pending: true}, false},
		{"capture declined", paypalOrderJSON("COMPLETED", "tok", "19.99", "USD", "DECLINED"), "", 0, VerifyResult{}, false},
		{"voided", paypalOrderJSON("VOIDED", "tok", "19.99", "USD", ""), "", 0, VerifyResult{}, false},
		{"instrument declined at capture", paypalOrderJSON("APPROVED", "tok", "19.99", "USD", ""), `{"name":"UNPROCESSABLE_ENTITY","details":[{"issue":"INSTRUMENT_DECLINED"}]}`, http.StatusUnprocessableEntity, VerifyResult{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newPayPalServer(t)
			p.order, p.capture = tc.order, tc.capture
			if tc.capCode != 0 {
				p.capCode = tc.capCode
			}
			g := &PayPalGateway{ClientID: "c", ClientSecret: "s", BaseURL: p.server.URL}
			got, err := g.Verify(context.Background(), req)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if *got != tc.want {
				t.Fatalf("Verify = %+v, want %+v", *got, tc.want)
			}
			if p.called("POST /v2/checkout/orders/ORD-1/capture") != tc.wantCapture {
				t.Fatalf("capture called = %v, want %v", !tc.wantCapture, tc.wantCapture)
			}
		})
	}
}

func TestPayPalCaptureIsIdempotentPerOrder(t *testing.T) {
	p := newPayPalServer(t)
	p.order = paypalOrderJSON("APPROVED", "tok", "19.99", "USD", "")
	p.capture = paypalOrderJSON("COMPLETED", "tok", "19.99", "USD", "COMPLETED")
	g := &PayPalGateway{ClientID: "c", ClientSecret: "s", BaseURL: p.server.URL}
	if _, err := g.Verify(context.Background(), VerifyRequest{Ref: "ORD-1", OrderToken: "tok", Amount: 1999, Currency: "USD"}); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got := p.headers["POST /v2/checkout/orders/ORD-1/capture"].Get("PayPal-Request-Id"); got != "capture-tok" {
		t.Fatalf("capture PayPal-Request-Id = %q, want one stable key per order", got)
	}
}

func TestPayPalOtherCaptureFailuresAreGatewayErrorsNotVerdicts(t *testing.T) {
	p := newPayPalServer(t)
	p.order = paypalOrderJSON("APPROVED", "tok", "19.99", "USD", "")
	p.capture, p.capCode = `{"name":"INTERNAL_SERVER_ERROR"}`, http.StatusInternalServerError
	g := &PayPalGateway{ClientID: "c", ClientSecret: "s", BaseURL: p.server.URL}
	if _, err := g.Verify(context.Background(), VerifyRequest{Ref: "ORD-1", OrderToken: "tok", Amount: 1999, Currency: "USD"}); !errors.Is(err, ErrGateway) {
		t.Fatalf("Verify error = %v, want ErrGateway", err)
	}
}

func TestPayPalSandboxBase(t *testing.T) {
	if got := (&PayPalGateway{Sandbox: true}).base(); got != "https://api-m.sandbox.paypal.com" {
		t.Fatalf("sandbox base = %q", got)
	}
}

func TestMinorFromDecimalIsStrict(t *testing.T) {
	tests := map[string]struct {
		n  int64
		ok bool
	}{"19.99": {1999, true}, "20": {2000, true}, "0.5": {50, true}, "19.999": {}, "-1.00": {}, "1e2": {}, "": {}, ".5": {}}
	for in, want := range tests {
		if n, ok := minorFromDecimal(in); n != want.n || ok != want.ok {
			t.Errorf("minorFromDecimal(%q) = %d,%v want %d,%v", in, n, ok, want.n, want.ok)
		}
	}
}
