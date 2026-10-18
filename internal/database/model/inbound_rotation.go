package model

// InboundRotation is the port/SNI rotation setup of one inbound. Port rotation
// picks a free port in [PortMin, PortMax]; SNI rotation moves a REALITY inbound
// to another entry of SniList. IntervalHours 0 means manual rotation only.
type InboundRotation struct {
	Id            int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	InboundId     int    `json:"inboundId" gorm:"column:inbound_id;uniqueIndex;not null" example:"3"`
	RotatePort    bool   `json:"rotatePort" gorm:"column:rotate_port" example:"true"`
	PortMin       int    `json:"portMin" gorm:"column:port_min" example:"20000"`
	PortMax       int    `json:"portMax" gorm:"column:port_max" example:"40000"`
	RotateSNI     bool   `json:"rotateSni" gorm:"column:rotate_sni" example:"true"`
	SniList       string `json:"sniList" gorm:"column:sni_list;type:text" example:"www.microsoft.com\nwww.apple.com:443"`
	IntervalHours int    `json:"intervalHours" gorm:"column:interval_hours" example:"24"`
	// LastRotatedAt is unix milliseconds; 0 means never rotated.
	LastRotatedAt int64  `json:"lastRotatedAt" gorm:"column:last_rotated_at" example:"1760000000000"`
	LastPort      int    `json:"lastPort" gorm:"column:last_port" example:"23871"`
	LastSni       string `json:"lastSni" gorm:"column:last_sni" example:"www.apple.com"`
	// NextRotationAt is unix milliseconds of the next scheduled rotation; 0 when no schedule runs.
	NextRotationAt int64 `json:"nextRotationAt" gorm:"column:next_rotation_at" example:"1760086400000"`
}

func (InboundRotation) TableName() string { return "inbound_rotations" }
