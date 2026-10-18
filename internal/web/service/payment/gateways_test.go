package payment

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// fakeGateway serves canned replies per path and records what it was sent.
type fakeGateway struct {
	t       *testing.T
	server  *httptest.Server
	replies map[string]fakeReply
	got     map[string]map[string]any
	headers map[string]http.Header
}

type fakeReply struct {
	status int
	body   string
}

func newFake(t *testing.T, replies map[string]fakeReply) *fakeGateway {
	t.Helper()
	f := &fakeGateway{t: t, replies: replies, got: map[string]map[string]any{}, headers: map[string]http.Header{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		f.got[r.URL.Path] = body
		f.headers[r.URL.Path] = r.Header.Clone()
		reply, ok := f.replies[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.status)
		_, _ = w.Write([]byte(reply.body))
	}))
	t.Cleanup(f.server.Close)
	return f
}

const (
	zpRequest = "/pg/v4/payment/request.json"
	zpVerify  = "/pg/v4/payment/verify.json"
)

func TestZarinpalStartSendsRialsAndReturnsTheStartPayURL(t *testing.T) {
	f := newFake(t, map[string]fakeReply{
		zpRequest: {200, `{"data":{"code":100,"message":"Success","authority":"A00000000000000000000000000123456789","fee":10},"errors":[]}`},
	})
	g := &ZarinpalGateway{MerchantID: "merchant-uuid", BaseURL: f.server.URL}

	got, err := g.Start(context.Background(), StartRequest{OrderToken: "tok", Amount: 2500000, Description: "Monthly", CallbackURL: "https://shop.example.com/cb?order=tok"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got.Ref != "A00000000000000000000000000123456789" || got.PayURL != f.server.URL+"/pg/StartPay/A00000000000000000000000000123456789" {
		t.Fatalf("result = %+v", got)
	}
	body := f.got[zpRequest]
	if body["merchant_id"] != "merchant-uuid" || body["amount"] != float64(2500000) || body["currency"] != "IRR" || body["callback_url"] != "https://shop.example.com/cb?order=tok" {
		t.Fatalf("request body = %v", body)
	}
}

func TestZarinpalStartRefusalIsAGatewayError(t *testing.T) {
	f := newFake(t, map[string]fakeReply{
		zpRequest: {200, `{"data":[],"errors":{"code":-9,"message":"Validation error","validations":[]}}`},
	})
	g := &ZarinpalGateway{MerchantID: "m", BaseURL: f.server.URL}
	if _, err := g.Start(context.Background(), StartRequest{Amount: 1}); !errors.Is(err, ErrGateway) {
		t.Fatalf("Start error = %v, want ErrGateway", err)
	}
}

func TestZarinpalSandboxUsesTheSandboxHost(t *testing.T) {
	if got := (&ZarinpalGateway{Sandbox: true}).base(); got != "https://sandbox.zarinpal.com" {
		t.Fatalf("sandbox base = %q", got)
	}
	if got := (&ZarinpalGateway{}).base(); got != "https://payment.zarinpal.com" {
		t.Fatalf("live base = %q", got)
	}
}

func TestZarinpalVerifyOutcomes(t *testing.T) {
	cases := []struct {
		name     string
		reply    fakeReply
		wantPaid bool
		wantRef  string
		wantErr  bool
	}{
		{"verified", fakeReply{200, `{"data":{"code":100,"message":"Verified","ref_id":201,"card_pan":"603799******5678"},"errors":[]}`}, true, "201", false},
		{"already verified", fakeReply{200, `{"data":{"code":101,"message":"Verified","ref_id":201},"errors":[]}`}, true, "201", false},
		{"payment failed", fakeReply{200, `{"data":[],"errors":{"code":-51,"message":"Session is not valid"}}`}, false, "", false},
		{"amount mismatch", fakeReply{200, `{"data":[],"errors":{"code":-50,"message":"amount mismatch"}}`}, false, "", false},
		{"unknown failure stays retryable", fakeReply{200, `{"data":[],"errors":{"code":-9,"message":"validation"}}`}, false, "", true},
		{"server error stays retryable", fakeReply{502, `bad gateway`}, false, "", true},
		{"garbage stays retryable", fakeReply{200, `<html>`}, false, "", true},
		{"a 500 is never read as not paid", fakeReply{500, `{"data":[],"errors":{"code":-51,"message":"Session is not valid"}}`}, false, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t, map[string]fakeReply{zpVerify: tc.reply})
			g := &ZarinpalGateway{MerchantID: "m", BaseURL: f.server.URL}
			got, err := g.Verify(context.Background(), VerifyRequest{Ref: "A1", OrderToken: "tok", Amount: 2500000})
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				if !errors.Is(err, ErrGateway) {
					t.Fatalf("error = %v, want ErrGateway", err)
				}
				return
			}
			if got.Paid != tc.wantPaid || got.TrackingRef != tc.wantRef {
				t.Fatalf("result = %+v, want paid=%v ref=%q", got, tc.wantPaid, tc.wantRef)
			}
			body := f.got[zpVerify]
			if body["authority"] != "A1" || body["amount"] != float64(2500000) || body["merchant_id"] != "m" {
				t.Fatalf("verify body = %v", body)
			}
		})
	}
}

func TestZarinpalCallbackPaidOnlyForStatusOK(t *testing.T) {
	if !ZarinpalCallbackPaid(url.Values{"Status": {"OK"}}) || ZarinpalCallbackPaid(url.Values{"Status": {"NOK"}}) || ZarinpalCallbackPaid(url.Values{}) {
		t.Fatal("Status=OK is the only paid callback")
	}
}

const (
	idpayStart  = "/v1.1/payment"
	idpayVerify = "/v1.1/payment/verify"
)

func TestIDPayStartAuthenticatesAndReturnsTheLink(t *testing.T) {
	f := newFake(t, map[string]fakeReply{
		idpayStart: {201, `{"id":"d2e353189823079e1e4181772cff5292","link":"https://idpay.ir/p/ws/d2e353189823079e1e4181772cff5292"}`},
	})
	g := &IDPayGateway{APIKey: "key-123", Sandbox: true, BaseURL: f.server.URL}

	got, err := g.Start(context.Background(), StartRequest{OrderToken: "tok", Amount: 2500000, Description: "Monthly", CallbackURL: "https://shop.example.com/cb?order=tok"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got.Ref != "d2e353189823079e1e4181772cff5292" || got.PayURL != "https://idpay.ir/p/ws/d2e353189823079e1e4181772cff5292" {
		t.Fatalf("result = %+v", got)
	}
	h := f.headers[idpayStart]
	if h.Get("X-API-KEY") != "key-123" || h.Get("X-SANDBOX") != "1" {
		t.Fatalf("headers = %v, want the API key and sandbox flag", h)
	}
	body := f.got[idpayStart]
	if body["order_id"] != "tok" || body["amount"] != float64(2500000) || body["callback"] != "https://shop.example.com/cb?order=tok" {
		t.Fatalf("body = %v", body)
	}
}

func TestIDPayStartRefusalIsAGatewayError(t *testing.T) {
	f := newFake(t, map[string]fakeReply{idpayStart: {403, `{"error_code":11,"error_message":"user not verified"}`}})
	g := &IDPayGateway{APIKey: "k", BaseURL: f.server.URL}
	if _, err := g.Start(context.Background(), StartRequest{Amount: 1}); !errors.Is(err, ErrGateway) {
		t.Fatalf("Start error = %v, want ErrGateway", err)
	}
	if got := f.headers[idpayStart].Get("X-SANDBOX"); got != "0" {
		t.Fatalf("X-SANDBOX = %q on a live gateway, want 0", got)
	}
}

func TestIDPayVerifyOutcomes(t *testing.T) {
	cases := []struct {
		name     string
		reply    fakeReply
		wantPaid bool
		wantRef  string
		wantErr  bool
	}{
		{"verified", fakeReply{200, `{"status":100,"track_id":"10012","order_id":"tok","amount":2500000}`}, true, "10012", false},
		{"numeric track id", fakeReply{200, `{"status":101,"track_id":10012,"order_id":"tok","amount":2500000}`}, true, "10012", false},
		{"wrong amount", fakeReply{200, `{"status":100,"track_id":"1","order_id":"tok","amount":1000}`}, false, "", false},
		{"someone else's order", fakeReply{200, `{"status":100,"track_id":"1","order_id":"other","amount":2500000}`}, false, "", false},
		{"not paid", fakeReply{200, `{"status":2,"track_id":"1","order_id":"tok","amount":2500000}`}, false, "", false},
		{"no such payment", fakeReply{405, `{"error_code":52,"error_message":"no result"}`}, false, "", false},
		{"other api error stays retryable", fakeReply{405, `{"error_code":13,"error_message":"limit"}`}, false, "", true},
		{"server error stays retryable", fakeReply{503, `down`}, false, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFake(t, map[string]fakeReply{idpayVerify: tc.reply})
			g := &IDPayGateway{APIKey: "k", BaseURL: f.server.URL}
			got, err := g.Verify(context.Background(), VerifyRequest{Ref: "gw-id", OrderToken: "tok", Amount: 2500000})
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if got.Paid != tc.wantPaid || got.TrackingRef != tc.wantRef {
				t.Fatalf("result = %+v, want paid=%v ref=%q", got, tc.wantPaid, tc.wantRef)
			}
			if body := f.got[idpayVerify]; body["id"] != "gw-id" || body["order_id"] != "tok" {
				t.Fatalf("verify body = %v", body)
			}
		})
	}
}

func TestRialAmount(t *testing.T) {
	if got, ok := RialAmount(250000, "IRT"); !ok || got != 2500000 {
		t.Fatalf("IRT 250000 = %d,%v want 2500000 rials", got, ok)
	}
	if got, ok := RialAmount(250000, "IRR"); !ok || got != 250000 {
		t.Fatalf("IRR 250000 = %d,%v", got, ok)
	}
	if _, ok := RialAmount(100, "USD"); ok {
		t.Fatal("USD must not be payable through an Iranian gateway")
	}
}
