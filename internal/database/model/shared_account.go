package model

// ClientIPSighting is one address a client was seen from inside the shared-account
// window. Country is empty when no geoip database could place the address.
type ClientIPSighting struct {
	Id       int    `gorm:"primaryKey;autoIncrement"`
	Email    string `gorm:"not null;uniqueIndex:idx_client_ip_sighting,priority:1"`
	IP       string `gorm:"column:ip;not null;uniqueIndex:idx_client_ip_sighting,priority:2"`
	Country  string `gorm:"not null;default:''"`
	LastSeen int64  `gorm:"column:last_seen;not null;index"`
}

func (ClientIPSighting) TableName() string { return "client_ip_sightings" }

// SharedAccountFlag marks a client seen from more addresses or countries than the
// thresholds allow. It is dropped once the client falls back under them.
type SharedAccountFlag struct {
	Email        string `json:"email" gorm:"primaryKey" example:"alice"`
	IPCount      int    `json:"ipCount" gorm:"column:ip_count;not null" example:"7"`
	CountryCount int    `json:"countryCount" gorm:"column:country_count;not null" example:"3"`
	Countries    string `json:"countries" gorm:"not null;default:''" example:"DE,US,BR"`
	FirstFlagged int64  `json:"firstFlagged" gorm:"column:first_flagged;not null" example:"1760000000000"`
	LastSeen     int64  `json:"lastSeen" gorm:"column:last_seen;not null" example:"1760003600000"`
}

func (SharedAccountFlag) TableName() string { return "shared_account_flags" }
