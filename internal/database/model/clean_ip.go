package model

// CleanIPResult is the latest probe of one CDN address from the panel host.
// Each scan replaces the whole table, so a row is never older than the last scan.
type CleanIPResult struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	IP        string `json:"ip" gorm:"column:ip;not null;uniqueIndex:idx_clean_ip_target" example:"104.16.124.96"`
	Port      int    `json:"port" gorm:"not null;uniqueIndex:idx_clean_ip_target" example:"443"`
	Ok        bool   `json:"ok" example:"true"`
	LatencyMs int    `json:"latencyMs" gorm:"column:latency_ms" example:"84"`
	// LossPct is the share of failed attempts among the samples taken, 0-100.
	LossPct   int    `json:"lossPct" gorm:"column:loss_pct" example:"0"`
	Reason    string `json:"reason" example:""`
	ScannedAt int64  `json:"scannedAt" gorm:"column:scanned_at" example:"1760000000000"`
}

func (CleanIPResult) TableName() string { return "clean_ip_results" }
