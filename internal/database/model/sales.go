package model

// Order statuses. An order moves pending -> review (card receipt sent) -> paid
// or rejected; a gateway order goes pending -> paid or failed.
const (
	OrderStatusPending  = "pending"
	OrderStatusReview   = "review"
	OrderStatusPaid     = "paid"
	OrderStatusRejected = "rejected"
	OrderStatusFailed   = "failed"
	OrderStatusCanceled = "canceled"
)

// Order kinds: a purchase creates the client, a renewal extends an existing one.
const (
	OrderKindNew   = "new"
	OrderKindRenew = "renew"
)

// Plan is a sellable package. Price is in the currency's minor unit (see
// service.CurrencyDecimals); TotalGB is in bytes; DurationDays 0 means no
// expiry. TemplateId supplies the inbounds and the rest of the client preset.
type Plan struct {
	Id           int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	Name         string `json:"name" gorm:"uniqueIndex;not null" example:"Monthly 50 GB"`
	Description  string `json:"description" gorm:"default:''" example:"Valid for 30 days"`
	Price        int64  `json:"price" gorm:"not null" example:"250000"`
	Currency     string `json:"currency" gorm:"not null" example:"IRT"`
	TotalGB      int64  `json:"totalGB" gorm:"column:total_gb;default:0" example:"53687091200"`
	DurationDays int    `json:"durationDays" gorm:"column:duration_days;default:0" example:"30"`
	LimitIP      int    `json:"limitIp" gorm:"column:limit_ip;default:0" example:"2"`
	TemplateId   int    `json:"templateId" gorm:"column:template_id;not null" example:"1"`
	Enabled      bool   `json:"enabled" gorm:"not null" example:"true"`
	SortOrder    int    `json:"sortOrder" gorm:"column:sort_order;default:0" example:"0"`
	CreatedAt    int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
	UpdatedAt    int64  `json:"updatedAt" gorm:"autoUpdateTime:milli"`
}

func (Plan) TableName() string { return "plans" }

// Order is one purchase attempt of a plan. Amount is what the customer pays in
// Currency's minor unit; the plan name and terms are snapshots so editing a
// plan never changes what a customer already paid for. Token is the
// unguessable handle the customer uses. PaidAt set with FulfilledAt unset is a
// paid order whose client has not been created or renewed yet.
type Order struct {
	Id           int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	Token        string `json:"token" gorm:"uniqueIndex;not null" example:"3f9c2a7e5b1d4c8a9e0f6b2d7a1c4e8f"`
	PlanId       int    `json:"planId" gorm:"column:plan_id;index" example:"1"`
	PlanName     string `json:"planName" gorm:"column:plan_name" example:"Monthly 50 GB"`
	PlanTemplate int    `json:"-" gorm:"column:plan_template"`
	TotalGB      int64  `json:"totalGB" gorm:"column:total_gb;default:0" example:"53687091200"`
	DurationDays int    `json:"durationDays" gorm:"column:duration_days;default:0" example:"30"`
	LimitIP      int    `json:"limitIp" gorm:"column:limit_ip;default:0" example:"2"`
	Kind         string `json:"kind" gorm:"not null" example:"new"`
	ClientEmail  string `json:"clientEmail" gorm:"column:client_email;index" example:"alice"`
	Contact      string `json:"contact" gorm:"default:''" example:"@alice"`
	Amount       int64  `json:"amount" gorm:"not null" example:"250000"`
	Currency     string `json:"currency" gorm:"not null" example:"IRT"`
	Gateway      string `json:"gateway" gorm:"default:''" example:"card"`
	Status       string `json:"status" gorm:"not null;index" example:"pending"`
	GatewayRef   string `json:"gatewayRef" gorm:"column:gateway_ref;index;default:''"`
	ReceiptRef   string `json:"receiptRef" gorm:"column:receipt_ref;default:''" example:"482913"`
	ReceiptNote  string `json:"receiptNote" gorm:"column:receipt_note;default:''"`
	RejectReason string `json:"rejectReason" gorm:"column:reject_reason;default:''"`
	CreatedAt    int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
	UpdatedAt    int64  `json:"updatedAt" gorm:"autoUpdateTime:milli"`
	PaidAt       int64  `json:"paidAt" gorm:"column:paid_at;default:0"`
	FulfilledAt  int64  `json:"fulfilledAt" gorm:"column:fulfilled_at;default:0"`
}

func (Order) TableName() string { return "orders" }
