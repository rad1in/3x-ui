package service

import (
	"errors"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"

	"gorm.io/gorm"
)

// currencyDecimals maps each sellable currency to the digits after the decimal
// point; prices and amounts are stored as integers in the minor unit.
var currencyDecimals = map[string]int{"IRT": 0, "IRR": 0, "USD": 2, "EUR": 2, "USDT": 2}

// CurrencyDecimals reports the minor-unit digits of a supported currency.
func CurrencyDecimals(currency string) (int, bool) {
	d, ok := currencyDecimals[currency]
	return d, ok
}

// PlanService manages the sellable packages.
type PlanService struct{}

// ListPlans returns plans by sort order; onlyEnabled is the public shop view.
func (s *PlanService) ListPlans(onlyEnabled bool) ([]model.Plan, error) {
	q := database.GetDB().Order("sort_order asc, id asc")
	if onlyEnabled {
		q = q.Where("enabled = ?", true)
	}
	var rows []model.Plan
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *PlanService) GetPlan(id int) (*model.Plan, error) {
	plan := &model.Plan{}
	if err := database.GetDB().First(plan, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.NewError("plan not found")
		}
		return nil, err
	}
	return plan, nil
}

// SavePlan creates the plan, or replaces the one with plan.Id.
func (s *PlanService) SavePlan(plan *model.Plan) (*model.Plan, error) {
	if plan == nil {
		return nil, common.NewError("empty plan")
	}
	plan.Name = strings.TrimSpace(plan.Name)
	if plan.Name == "" {
		return nil, common.NewError("plan name is required")
	}
	plan.Currency = strings.ToUpper(strings.TrimSpace(plan.Currency))
	if _, ok := CurrencyDecimals(plan.Currency); !ok {
		return nil, common.NewError("unsupported currency:", plan.Currency)
	}
	if plan.Price < 0 {
		return nil, common.NewError("price cannot be negative")
	}
	if plan.TotalGB < 0 || plan.DurationDays < 0 || plan.LimitIP < 0 {
		return nil, common.NewError("quota, duration and IP limit cannot be negative")
	}
	if _, err := (&ClientService{}).GetClientTemplate(plan.TemplateId); err != nil {
		return nil, err
	}
	db := database.GetDB()
	var clash int64
	if err := db.Model(&model.Plan{}).Where("name = ? AND id <> ?", plan.Name, plan.Id).Count(&clash).Error; err != nil {
		return nil, err
	}
	if clash > 0 {
		return nil, common.NewError("plan name already exists")
	}
	if plan.Id == 0 {
		if err := db.Create(plan).Error; err != nil {
			return nil, err
		}
		return plan, nil
	}
	if _, err := s.GetPlan(plan.Id); err != nil {
		return nil, err
	}
	if err := db.Save(plan).Error; err != nil {
		return nil, err
	}
	return plan, nil
}

// DeletePlan refuses while an order is still awaiting payment or review, so a
// customer never pays for a plan that vanished.
func (s *PlanService) DeletePlan(id int) error {
	db := database.GetDB()
	var open int64
	err := db.Model(&model.Order{}).
		Where("plan_id = ? AND status IN ?", id, []string{model.OrderStatusPending, model.OrderStatusReview}).
		Count(&open).Error
	if err != nil {
		return err
	}
	if open > 0 {
		return common.NewError("plan has open orders")
	}
	res := db.Delete(&model.Plan{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return common.NewError("plan not found")
	}
	return nil
}
