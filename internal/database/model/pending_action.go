package model

// PendingAction is a sensitive admin action parked until a second admin
// approves it. Payload is the JSON the executor needs; Status moves once,
// from pending to approved, rejected, cancelled or expired (failed when the
// approved action returned an error).
type PendingAction struct {
	Id              int    `json:"id" gorm:"primaryKey;autoIncrement" example:"7"`
	Action          string `json:"action" gorm:"index;not null" example:"inbound.delete"`
	Target          string `json:"target" gorm:"not null;default:''" example:"inbound #3"`
	Payload         string `json:"-" gorm:"type:text;not null"`
	RequestedBy     int    `json:"requestedBy" gorm:"index;not null" example:"1"`
	RequestedByName string `json:"requestedByName" gorm:"not null;default:''" example:"alice"`
	RequestedAt     int64  `json:"requestedAt" gorm:"not null" example:"1760684400000"`
	ExpiresAt       int64  `json:"expiresAt" gorm:"index;not null" example:"1760770800000"`
	Status          string `json:"status" gorm:"index;not null" example:"pending"`
	DecidedBy       int    `json:"decidedBy" gorm:"not null;default:0" example:"2"`
	DecidedByName   string `json:"decidedByName" gorm:"not null;default:''" example:"bob"`
	DecidedAt       int64  `json:"decidedAt" gorm:"not null;default:0" example:"1760684500000"`
	Result          string `json:"result" gorm:"type:text;not null" example:""`
}

func (PendingAction) TableName() string { return "pending_actions" }
