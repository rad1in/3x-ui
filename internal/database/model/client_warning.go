package model

// ClientWarning records which advance warnings a client was already sent, so
// each threshold crossing notifies once; the row goes once the client is back under.
type ClientWarning struct {
	Email  string `gorm:"primaryKey"`
	Quota  bool   `gorm:"not null;default:false"`
	Expiry bool   `gorm:"not null;default:false"`
}

// ClientDepletion is when a client was first seen out of quota or past its
// expiry; autoDeleteDepletedDays counts from it.
type ClientDepletion struct {
	Email string `gorm:"primaryKey"`
	Since int64  `gorm:"not null"`
}
