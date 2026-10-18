package service

import (
	"errors"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"

	"gorm.io/gorm"
)

const millisPerDay = int64(24 * 3600 * 1000)

// ClientTemplateResult reports which emails a template run created and why
// each other one was refused.
type ClientTemplateResult struct {
	Created     []string          `json:"created"`
	Failed      map[string]string `json:"failed"`
	NeedRestart bool              `json:"needRestart"`
}

func (s *ClientService) ListClientTemplates() ([]model.ClientTemplate, error) {
	var rows []model.ClientTemplate
	if err := database.GetDB().Order("name asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *ClientService) GetClientTemplate(id int) (*model.ClientTemplate, error) {
	tpl := &model.ClientTemplate{}
	if err := database.GetDB().First(tpl, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.NewError("client template not found")
		}
		return nil, err
	}
	return tpl, nil
}

// SaveClientTemplate creates the template, or replaces the one with tpl.Id.
func (s *ClientService) SaveClientTemplate(tpl *model.ClientTemplate) (*model.ClientTemplate, error) {
	if tpl == nil {
		return nil, common.NewError("empty template")
	}
	tpl.Name = strings.TrimSpace(tpl.Name)
	if tpl.Name == "" {
		return nil, common.NewError("template name is required")
	}
	if len(tpl.InboundIds) == 0 {
		return nil, common.NewError("at least one inbound is required")
	}
	if tpl.TotalGB < 0 || tpl.ExpiryDays < 0 || tpl.LimitIP < 0 {
		return nil, common.NewError("quota, duration and IP limit cannot be negative")
	}
	if err := validateClientTrafficReset(tpl.TrafficReset, tpl.TrafficResetDay); err != nil {
		return nil, err
	}
	db := database.GetDB()
	var clash int64
	if err := db.Model(&model.ClientTemplate{}).Where("name = ? AND id <> ?", tpl.Name, tpl.Id).Count(&clash).Error; err != nil {
		return nil, err
	}
	if clash > 0 {
		return nil, common.NewError("template name already exists")
	}
	if tpl.Id == 0 {
		if err := db.Create(tpl).Error; err != nil {
			return nil, err
		}
		return tpl, nil
	}
	if _, err := s.GetClientTemplate(tpl.Id); err != nil {
		return nil, err
	}
	if err := db.Save(tpl).Error; err != nil {
		return nil, err
	}
	return tpl, nil
}

func (s *ClientService) DeleteClientTemplate(id int) error {
	res := database.GetDB().Delete(&model.ClientTemplate{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return common.NewError("client template not found")
	}
	return nil
}

// templateExpiry is a negative duration for a delayed start, which the
// traffic job turns into an absolute date on first use.
func templateExpiry(tpl *model.ClientTemplate, now int64) int64 {
	if tpl.ExpiryDays == 0 {
		return 0
	}
	span := int64(tpl.ExpiryDays) * millisPerDay
	if tpl.DelayedStart {
		return -span
	}
	return now + span
}

// CreateClientsFromTemplate creates one client per email from the preset; an
// email that fails is reported in Failed without stopping the others.
func (s *ClientService) CreateClientsFromTemplate(inboundSvc *InboundService, templateID int, emails []string) (*ClientTemplateResult, error) {
	tpl, err := s.GetClientTemplate(templateID)
	if err != nil {
		return nil, err
	}
	res := &ClientTemplateResult{Created: []string{}, Failed: map[string]string{}}
	now := time.Now().UnixMilli()
	for _, raw := range emails {
		email := strings.TrimSpace(raw)
		if email == "" {
			continue
		}
		needRestart, err := s.Create(inboundSvc, &ClientCreatePayload{
			Client: model.Client{
				Email:           email,
				Enable:          true,
				TotalGB:         tpl.TotalGB,
				ExpiryTime:      templateExpiry(tpl, now),
				LimitIP:         tpl.LimitIP,
				Flow:            tpl.Flow,
				Group:           tpl.Group,
				Comment:         tpl.Comment,
				Reset:           tpl.Reset,
				TrafficReset:    tpl.TrafficReset,
				TrafficResetDay: tpl.TrafficResetDay,
			},
			InboundIds: append([]int(nil), tpl.InboundIds...),
		})
		res.NeedRestart = res.NeedRestart || needRestart
		if err != nil {
			res.Failed[email] = err.Error()
			continue
		}
		res.Created = append(res.Created, email)
	}
	return res, nil
}
