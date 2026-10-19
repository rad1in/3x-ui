package service

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"

	"gorm.io/gorm"
)

var discountCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{2,31}$`)

// Orders in these statuses no longer hold a code use.
var closedOrderStatuses = []string{model.OrderStatusFailed, model.OrderStatusCanceled, model.OrderStatusRejected}

// DiscountService manages coupons.
type DiscountService struct{}

func normalizeCode(code string) string { return strings.ToUpper(strings.TrimSpace(code)) }

// ListDiscounts returns every code with how many orders hold a use of it.
func (s *DiscountService) ListDiscounts() ([]model.DiscountCode, error) {
	var rows []model.DiscountCode
	if err := database.GetDB().Order("id desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		uses, err := countDiscountUses(rows[i].Id, "")
		if err != nil {
			return nil, err
		}
		rows[i].Uses = uses
	}
	return rows, nil
}

// SaveDiscount creates the code, or replaces the one with code.Id.
func (s *DiscountService) SaveDiscount(code *model.DiscountCode) (*model.DiscountCode, error) {
	if code == nil {
		return nil, common.NewError("empty discount code")
	}
	code.Code = normalizeCode(code.Code)
	if !discountCodePattern.MatchString(code.Code) {
		return nil, common.NewError("discount code must be 3 to 32 letters, digits, dash or underscore")
	}
	switch code.Kind {
	case model.DiscountPercent:
		if code.Value < 1 || code.Value > 99 {
			return nil, common.NewError("a percent discount must be between 1 and 99")
		}
		code.Currency = ""
	case model.DiscountFixed:
		if code.Value < 1 {
			return nil, common.NewError("a fixed discount must be positive")
		}
		code.Currency = strings.ToUpper(strings.TrimSpace(code.Currency))
		if _, ok := CurrencyDecimals(code.Currency); !ok {
			return nil, common.NewError("unsupported currency:", code.Currency)
		}
	default:
		return nil, common.NewError("discount kind must be percent or fixed")
	}
	if code.MaxUses < 0 || code.ExpiresAt < 0 {
		return nil, common.NewError("uses and expiry cannot be negative")
	}
	if code.PlanIds == nil {
		code.PlanIds = []int{}
	}
	db := database.GetDB()
	var clash int64
	if err := db.Model(&model.DiscountCode{}).Where("code = ? AND id <> ?", code.Code, code.Id).Count(&clash).Error; err != nil {
		return nil, err
	}
	if clash > 0 {
		return nil, common.NewError("discount code already exists")
	}
	if code.Id == 0 {
		return code, db.Create(code).Error
	}
	if err := db.First(&model.DiscountCode{}, code.Id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.NewError("discount code not found")
		}
		return nil, err
	}
	return code, db.Save(code).Error
}

func (s *DiscountService) DeleteDiscount(id int) error {
	res := database.GetDB().Delete(&model.DiscountCode{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return common.NewError("discount code not found")
	}
	return nil
}

// countDiscountUses counts the orders holding a use of the code, optionally of
// one customer.
func countDiscountUses(codeID int, email string) (int64, error) {
	q := database.GetDB().Model(&model.Order{}).Where("discount_id = ? AND status NOT IN ?", codeID, closedOrderStatuses)
	if email != "" {
		q = q.Where("client_email = ?", email)
	}
	var n int64
	return n, q.Count(&n).Error
}

// pricing is what an order costs after any coupon.
type pricing struct {
	List     int64
	Discount int64
	Final    int64
	Code     *model.DiscountCode
}

// priceOrder applies a coupon to a plan for a customer. An empty code is the
// list price. The caller holds orderCreateMu so the use count cannot race.
func priceOrder(plan *model.Plan, codeText, email string) (pricing, error) {
	p := pricing{List: plan.Price, Final: plan.Price}
	codeText = normalizeCode(codeText)
	if codeText == "" {
		return p, nil
	}
	var code model.DiscountCode
	err := database.GetDB().Where("code = ?", codeText).First(&code).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return p, salesErr(ErrDiscountInvalid)
	}
	if err != nil {
		return p, err
	}
	if !code.Enabled || (code.ExpiresAt > 0 && code.ExpiresAt < time.Now().UnixMilli()) ||
		(len(code.PlanIds) > 0 && !slices.Contains(code.PlanIds, plan.Id)) ||
		(code.Kind == model.DiscountFixed && code.Currency != plan.Currency) {
		return p, salesErr(ErrDiscountInvalid)
	}
	if code.MaxUses > 0 {
		uses, err := countDiscountUses(code.Id, "")
		if err != nil {
			return p, err
		}
		if uses >= int64(code.MaxUses) {
			return p, salesErr(ErrDiscountUsed)
		}
	}
	if code.OncePerCustomer && email != "" {
		uses, err := countDiscountUses(code.Id, email)
		if err != nil {
			return p, err
		}
		if uses > 0 {
			return p, salesErr(ErrDiscountUsed)
		}
	}
	p.Discount = code.Value
	if code.Kind == model.DiscountPercent {
		p.Discount = plan.Price * code.Value / 100
	}
	p.Final = plan.Price - p.Discount
	if p.Final <= 0 {
		return p, salesErr(ErrDiscountInvalid)
	}
	p.Code = &code
	return p, nil
}
