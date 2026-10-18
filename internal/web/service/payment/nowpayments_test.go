package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"testing"
)

// sign is the reference the sender uses: HMAC-SHA512 over the canonical text.
func sign(secret, canonical string) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestNOWPaymentsStartCreatesAnInvoicePricedInDollars(t *testing.T) {
	f := newFake(t, map[string]fakeReply{
		"/v1/invoice": {200, `{"id":"4522625843","order_id":"tok","invoice_url":"https://nowpayments.io/payment/?iid=4522625843"}`},
	})
	g := &NOWPaymentsGateway{APIKey: "np-key", BaseURL: f.server.URL}

	got, err := g.Start(context.Background(), StartRequest{
		OrderToken: "tok", Amount: 1999, Currency: "USD", Description: "Order 7",
		CallbackURL: "https://shop.example.com/shop/api/ipn/nowpayments", ReturnURL: "https://shop.example.com/shop/?order=tok",
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got.Ref != "4522625843" || got.PayURL != "https://nowpayments.io/payment/?iid=4522625843" {
		t.Fatalf("result = %+v", got)
	}
	if f.headers["/v1/invoice"].Get("x-api-key") != "np-key" {
		t.Fatalf("headers = %v, want the API key", f.headers["/v1/invoice"])
	}
	body := f.got["/v1/invoice"]
	if body["price_amount"] != 19.99 || body["price_currency"] != "usd" || body["order_id"] != "tok" ||
		body["ipn_callback_url"] != "https://shop.example.com/shop/api/ipn/nowpayments" || body["success_url"] != "https://shop.example.com/shop/?order=tok" {
		t.Fatalf("invoice body = %v", body)
	}
}

func TestNOWPaymentsStartRefusalIsAGatewayError(t *testing.T) {
	f := newFake(t, map[string]fakeReply{"/v1/invoice": {403, `{"status":false,"statusCode":403,"code":"INVALID_API_KEY","message":"Invalid api key"}`}})
	g := &NOWPaymentsGateway{APIKey: "bad", BaseURL: f.server.URL}
	if _, err := g.Start(context.Background(), StartRequest{Amount: 100, Currency: "USD"}); !errors.Is(err, ErrGateway) {
		t.Fatalf("Start error = %v, want ErrGateway", err)
	}
}

func TestNOWPaymentsSandboxBase(t *testing.T) {
	if got := (&NOWPaymentsGateway{Sandbox: true}).base(); got != "https://api-sandbox.nowpayments.io" {
		t.Fatalf("sandbox base = %q", got)
	}
}

func TestIPNSignatureCoversTheKeySortedBody(t *testing.T) {
	// Keys deliberately out of order, with a nested object and a non-ASCII value.
	body := `{"payment_status":"finished","order_id":"tok","fee":{"currency":"btc","depositFee":0.0001},"price_amount":19.99,"description":"é <b>"}`
	secret := "ipn-secret"
	sortedRaw := `{"description":"é <b>","fee":{"currency":"btc","depositFee":0.0001},"order_id":"tok","payment_status":"finished","price_amount":19.99}`
	sortedAscii := `{"description":"\u00e9 <b>","fee":{"currency":"btc","depositFee":0.0001},"order_id":"tok","payment_status":"finished","price_amount":19.99}`

	for name, canonical := range map[string]string{"raw unicode": sortedRaw, "ascii escaped": sortedAscii} {
		t.Run(name, func(t *testing.T) {
			if !VerifyIPNSignature(secret, []byte(body), sign(secret, canonical)) {
				t.Fatalf("a correct %s signature was rejected", name)
			}
		})
	}
}

func TestIPNSignatureRejections(t *testing.T) {
	body := `{"payment_status":"finished","order_id":"tok","price_amount":19.99}`
	canonical := `{"order_id":"tok","payment_status":"finished","price_amount":19.99}`
	good := sign("secret", canonical)
	cases := []struct {
		name      string
		secret    string
		body      string
		signature string
	}{
		{"wrong secret", "other", body, good},
		{"tampered status", "secret", `{"payment_status":"waiting","order_id":"tok","price_amount":19.99}`, good},
		{"tampered amount", "secret", `{"payment_status":"finished","order_id":"tok","price_amount":0.01}`, good},
		{"no signature", "secret", body, ""},
		{"not hex", "secret", body, "zz"},
		{"empty secret", "", body, sign("", canonical)},
		{"not json", "secret", `nope`, good},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if VerifyIPNSignature(tc.secret, []byte(tc.body), tc.signature) {
				t.Fatal("signature accepted")
			}
		})
	}
	if !VerifyIPNSignature("secret", []byte(body), "  "+good+"  ") {
		t.Fatal("a correct signature with stray spaces was rejected")
	}
	if !VerifyIPNSignature("secret", []byte(body), "  "+upper(good)) {
		t.Fatal("hex case must not matter")
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'f' {
			b[i] = c - 32
		}
	}
	return string(b)
}

func TestParseIPN(t *testing.T) {
	ipn, err := ParseIPN([]byte(`{"payment_id":5077125051,"payment_status":"partially_paid","order_id":"tok","price_amount":19.99,"price_currency":"usd","actually_paid":10.5,"pay_amount":19.9}`))
	if err != nil {
		t.Fatalf("ParseIPN: %v", err)
	}
	if ipn.PaymentID != "5077125051" || ipn.Status != NPPartiallyPaid || ipn.OrderID != "tok" || ipn.PriceAmount != "19.99" || ipn.PriceCurrency != "USD" || ipn.ActuallyPaid != "10.5" {
		t.Fatalf("ipn = %+v, want numbers kept textual and the currency upper-cased", ipn)
	}
	for _, bad := range []string{`{}`, `{"order_id":"x"}`, `{"payment_status":"finished"}`, `[1]`, `x`} {
		if _, err := ParseIPN([]byte(bad)); !errors.Is(err, ErrGateway) {
			t.Fatalf("ParseIPN(%s) error = %v, want ErrGateway", bad, err)
		}
	}
}
