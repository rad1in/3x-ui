package controller

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
)

type signedSubLinkBody struct {
	TTLSeconds int64 `json:"ttlSeconds"`
}

// signedSubLink issues a signed URL for a subscription id; ttlSeconds 0 never expires.
func (a *ClientController) signedSubLink(c *gin.Context) {
	var body signedSubLinkBody
	if err := c.ShouldBindJSON(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), err)
		return
	}
	if body.TTLSeconds < 0 {
		jsonMsg(c, I18nWeb(c, "somethingWentWrong"), errors.New("ttlSeconds cannot be negative"))
		return
	}
	link, err := a.subSigning.SignedURL(c.Param("subId"), time.Duration(body.TTLSeconds)*time.Second, c.Request.Host)
	jsonObj(c, link, err)
}
