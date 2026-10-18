package controller

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

func termsOf(cl model.Client) service.ClientTerms {
	return service.ClientTerms{TotalGB: cl.TotalGB, ExpiryTime: cl.ExpiryTime}
}

func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func createChanges(payloads []service.ClientCreatePayload) []service.TermsChange {
	out := make([]service.TermsChange, 0, len(payloads))
	for _, p := range payloads {
		out = append(out, service.TermsChange{Email: strings.TrimSpace(p.Client.Email), After: termsOf(p.Client)})
	}
	return out
}

// reserveCredit takes the acting reseller's credit for the changes. ok is false
// once the refusal has been answered; the reservation is nil for any other admin.
func (a *ClientController) reserveCredit(c *gin.Context, changes func() ([]service.TermsChange, error)) (res *service.ResellerReservation, ok bool) {
	if role, _ := c.Get(adminRoleKey); role != model.AdminRoleOperator {
		return nil, true
	}
	actor := session.GetLoginUser(c)
	if actor == nil {
		return nil, true
	}
	planned, err := changes()
	if err == nil {
		res, err = a.reseller.Reserve(actor, planned)
	}
	if err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return nil, false
	}
	return res, true
}

func settleCredit(res *service.ResellerReservation) {
	if err := res.Settle(); err != nil {
		logger.Warning("reseller credit settle:", err)
	}
}
