package model

// Kinds of location an admin has signed in from.
const (
	LoginSeenIP      = "ip"
	LoginSeenCountry = "country"
)

// LoginSeen records an address or country an admin has signed in from, so the
// next sign-in from somewhere else can raise an alert.
type LoginSeen struct {
	Id        int    `gorm:"primaryKey;autoIncrement"`
	UserId    int    `gorm:"column:user_id;not null;uniqueIndex:idx_login_seen"`
	Kind      string `gorm:"not null;uniqueIndex:idx_login_seen"`
	Value     string `gorm:"not null;uniqueIndex:idx_login_seen"`
	FirstSeen int64  `gorm:"column:first_seen;not null"`
	LastSeen  int64  `gorm:"column:last_seen;not null"`
}

func (LoginSeen) TableName() string { return "login_seen" }
