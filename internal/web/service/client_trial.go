package service

import (
	"errors"
	"strconv"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"

	"gorm.io/gorm/clause"
)

// ErrTrialAlreadyUsed means this Telegram user already got their free trial.
var ErrTrialAlreadyUsed = errors.New("free trial already used")

// CreateTrialClient gives a Telegram user the template's account, bound to
// their Telegram id. The grant row is claimed first, so concurrent requests
// create one client; a failed creation releases it.
func (s *ClientService) CreateTrialClient(inboundSvc *InboundService, templateID int, tgID int64) (string, bool, error) {
	if tgID <= 0 {
		return "", false, common.NewError("invalid telegram id")
	}
	tpl, err := s.GetClientTemplate(templateID)
	if err != nil {
		return "", false, err
	}
	email := "trial-" + strconv.FormatInt(tgID, 10)

	db := database.GetDB()
	claim := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.ClientTrial{TgID: tgID, Email: email})
	if claim.Error != nil {
		return "", false, claim.Error
	}
	if claim.RowsAffected == 0 {
		return "", false, ErrTrialAlreadyUsed
	}

	client := templateClient(tpl, email, time.Now().UnixMilli())
	client.TgID = tgID
	needRestart, err := s.Create(inboundSvc, &ClientCreatePayload{Client: client, InboundIds: append([]int(nil), tpl.InboundIds...)})
	if err != nil {
		if delErr := db.Where("tg_id = ?", tgID).Delete(&model.ClientTrial{}).Error; delErr != nil {
			return "", needRestart, errors.Join(err, delErr)
		}
		return "", needRestart, err
	}
	return email, needRestart, nil
}
