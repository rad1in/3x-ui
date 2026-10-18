package payment

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

const idpayBase = "https://api.idpay.ir"

// idpayVerified and idpayAlreadyVerified are the payment statuses IDPay
// reports once a payment has been verified.
const (
	idpayVerified        = 100
	idpayAlreadyVerified = 101
)

// IDPay verify error codes 51 to 54 say the payment was not created, has no
// result or can no longer be verified: the customer did not pay.
const (
	idpayNoPaymentMin = 51
	idpayNoPaymentMax = 54
)

// IDPayGateway is IDPay's v1.1 REST API.
type IDPayGateway struct {
	APIKey string
	// BaseURL overrides the API host.
	BaseURL string
	Sandbox bool
	Client  *http.Client
}

func (g *IDPayGateway) base() string {
	if g.BaseURL != "" {
		return g.BaseURL
	}
	return idpayBase
}

func (g *IDPayGateway) headers() map[string]string {
	sandbox := "0"
	if g.Sandbox {
		sandbox = "1"
	}
	return map[string]string{"X-API-KEY": g.APIKey, "X-SANDBOX": sandbox}
}

type idpayStartReply struct {
	ID           string `json:"id"`
	Link         string `json:"link"`
	ErrorCode    int    `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

type idpayVerifyReply struct {
	Status       int    `json:"status"`
	TrackID      any    `json:"track_id"`
	Amount       int64  `json:"amount"`
	OrderID      string `json:"order_id"`
	ErrorCode    int    `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

func (g *IDPayGateway) Start(ctx context.Context, req StartRequest) (*StartResult, error) {
	var reply idpayStartReply
	_, err := postJSON(ctx, httpClient(g.Client), g.base()+"/v1.1/payment", g.headers(), map[string]any{
		"order_id": req.OrderToken,
		"amount":   req.Amount,
		"desc":     req.Description,
		"callback": req.CallbackURL,
	}, &reply)
	if err != nil {
		return nil, err
	}
	if reply.ID == "" || reply.Link == "" {
		return nil, fmt.Errorf("%w: idpay refused the payment (error %d)", ErrGateway, reply.ErrorCode)
	}
	return &StartResult{Ref: reply.ID, PayURL: reply.Link}, nil
}

func (g *IDPayGateway) Verify(ctx context.Context, req VerifyRequest) (*VerifyResult, error) {
	var reply idpayVerifyReply
	status, err := postJSON(ctx, httpClient(g.Client), g.base()+"/v1.1/payment/verify", g.headers(), map[string]any{
		"id":       req.Ref,
		"order_id": req.OrderToken,
	}, &reply)
	if err != nil {
		return nil, err
	}
	if status == http.StatusOK && (reply.Status == idpayVerified || reply.Status == idpayAlreadyVerified) {
		if reply.Amount != req.Amount || reply.OrderID != req.OrderToken {
			return &VerifyResult{}, nil
		}
		return &VerifyResult{Paid: true, TrackingRef: idpayTrack(reply.TrackID)}, nil
	}
	if status == http.StatusOK && reply.Status != 0 {
		return &VerifyResult{}, nil
	}
	if reply.ErrorCode >= idpayNoPaymentMin && reply.ErrorCode <= idpayNoPaymentMax {
		return &VerifyResult{}, nil
	}
	return nil, fmt.Errorf("%w: idpay could not verify the payment (error %d)", ErrGateway, reply.ErrorCode)
}

func idpayTrack(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	default:
		return ""
	}
}

// IDPayCallbackPaid reports whether IDPay's redirect says the customer paid
// (status 10, awaiting verification). It is only a hint: Verify decides.
func IDPayCallbackPaid(form url.Values) bool { return form.Get("status") == "10" }
