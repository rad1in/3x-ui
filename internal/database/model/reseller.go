package model

// Reseller ledger entry kinds. Amounts are in credit units, the panel's own
// accounting unit for resellers (admins with the operator role).
const (
	ResellerTopup  = "topup"
	ResellerCreate = "create"
	ResellerRenew  = "renew"
)

// ResellerEntry is one line of a reseller's credit ledger; the balance is the
// sum of the Amount column. Create and renew lines name the client and the
// traffic and days that were billed.
type ResellerEntry struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	UserId      int    `json:"userId" gorm:"column:user_id;not null;index" example:"2"`
	Amount      int64  `json:"amount" gorm:"not null" example:"-1500"`
	Kind        string `json:"kind" gorm:"not null;index" example:"create"`
	ClientEmail string `json:"clientEmail" gorm:"column:client_email;default:''" example:"alice"`
	Bytes       int64  `json:"bytes" gorm:"default:0" example:"53687091200"`
	Days        int64  `json:"days" gorm:"default:0" example:"30"`
	Note        string `json:"note" gorm:"default:''"`
	CreatedAt   int64  `json:"createdAt" gorm:"autoCreateTime:milli" example:"1760684400000"`
}

func (ResellerEntry) TableName() string { return "reseller_entries" }
