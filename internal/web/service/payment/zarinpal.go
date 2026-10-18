package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const (
	zarinpalLive    = "https://payment.zarinpal.com"
	zarinpalSandbox = "https://sandbox.zarinpal.com"
)

const (
	zarinpalOK              = 100
	zarinpalAlreadyVerified = 101
)

// ZarinpalGateway is Zarinpal's v4 REST API.
type ZarinpalGateway struct {
	MerchantID string
	// BaseURL overrides the live or sandbox host.
	BaseURL string
	Sandbox bool
	Client  *http.Client
}

func (g *ZarinpalGateway) base() string {
	switch {
	case g.BaseURL != "":
		return g.BaseURL
	case g.Sandbox:
		return zarinpalSandbox
	default:
		return zarinpalLive
	}
}

type zarinpalReply struct {
	Data   json.RawMessage `json:"data"`
	Errors json.RawMessage `json:"errors"`
}

type zarinpalError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type zarinpalData struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Authority string          `json:"authority"`
	RefID     json.RawMessage `json:"ref_id"`
}

// decode splits a reply into its data object or error: Zarinpal sends data as
// an empty array and errors as an object on failure, and the reverse on success.
func (r zarinpalReply) decode() (*zarinpalData, *zarinpalError) {
	var d zarinpalData
	if len(r.Data) > 0 && r.Data[0] == '{' && json.Unmarshal(r.Data, &d) == nil && d.Code != 0 {
		return &d, nil
	}
	var e zarinpalError
	if len(r.Errors) > 0 && r.Errors[0] == '{' && json.Unmarshal(r.Errors, &e) == nil {
		return nil, &e
	}
	return nil, nil
}

func (g *ZarinpalGateway) Start(ctx context.Context, req StartRequest) (*StartResult, error) {
	var reply zarinpalReply
	_, err := postJSON(ctx, httpClient(g.Client), g.base()+"/pg/v4/payment/request.json", nil, map[string]any{
		"merchant_id":  g.MerchantID,
		"amount":       req.Amount,
		"currency":     "IRR",
		"description":  req.Description,
		"callback_url": req.CallbackURL,
	}, &reply)
	if err != nil {
		return nil, err
	}
	data, apiErr := reply.decode()
	if data == nil || data.Code != zarinpalOK || data.Authority == "" {
		return nil, fmt.Errorf("%w: zarinpal refused the payment%s", ErrGateway, zarinpalDetail(data, apiErr))
	}
	return &StartResult{Ref: data.Authority, PayURL: g.base() + "/pg/StartPay/" + data.Authority}, nil
}

// zarinpalNotPaid lists the verify codes that mean the payment definitively
// did not happen; any other failure is treated as unknown and retried.
var zarinpalNotPaid = map[int]bool{-50: true, -51: true, -52: true, -53: true, -54: true}

func (g *ZarinpalGateway) Verify(ctx context.Context, req VerifyRequest) (*VerifyResult, error) {
	var reply zarinpalReply
	_, err := postJSON(ctx, httpClient(g.Client), g.base()+"/pg/v4/payment/verify.json", nil, map[string]any{
		"merchant_id": g.MerchantID,
		"amount":      req.Amount,
		"authority":   req.Ref,
	}, &reply)
	if err != nil {
		return nil, err
	}
	data, apiErr := reply.decode()
	if data != nil && (data.Code == zarinpalOK || data.Code == zarinpalAlreadyVerified) {
		return &VerifyResult{Paid: true, TrackingRef: zarinpalRef(data.RefID)}, nil
	}
	if apiErr != nil && zarinpalNotPaid[apiErr.Code] {
		return &VerifyResult{}, nil
	}
	return nil, fmt.Errorf("%w: zarinpal could not verify the payment%s", ErrGateway, zarinpalDetail(data, apiErr))
}

func zarinpalRef(raw json.RawMessage) string {
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return strconv.FormatInt(n, 10)
	}
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

func zarinpalDetail(data *zarinpalData, apiErr *zarinpalError) string {
	switch {
	case apiErr != nil:
		return fmt.Sprintf(" (code %d)", apiErr.Code)
	case data != nil:
		return fmt.Sprintf(" (code %d)", data.Code)
	default:
		return ""
	}
}

// ZarinpalCallbackPaid reports whether Zarinpal's redirect says the customer
// paid (Status=OK). It is only a hint: Verify decides.
func ZarinpalCallbackPaid(query url.Values) bool { return query.Get("Status") == "OK" }
