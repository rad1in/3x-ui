package eventbus

import "time"

// EventType identifies the kind of event flowing through the bus.
type EventType string

const (
	// Outbound health (observatory-driven)
	EventOutboundDown EventType = "outbound.down"
	EventOutboundUp   EventType = "outbound.up"

	// Xray core (local)
	EventXrayCrash EventType = "xray.crash"

	// Node health (heartbeat-driven)
	EventNodeDown EventType = "node.down"
	EventNodeUp   EventType = "node.up"

	// System health
	EventCPUHigh    EventType = "cpu.high"
	EventMemoryHigh EventType = "memory.high"

	// Security
	EventLoginAttempt EventType = "login.attempt"

	// Client lifecycle; Source is the client's email, Data a *ClientEventData.
	EventClientCreated       EventType = "client.created"
	EventClientDeleted       EventType = "client.deleted"
	EventClientEnabled       EventType = "client.enabled"
	EventClientDisabled      EventType = "client.disabled"
	EventClientDepleted      EventType = "client.depleted"
	EventClientExpired       EventType = "client.expired"
	EventClientRenewed       EventType = "client.renewed"
	EventClientTrafficReset  EventType = "client.traffic_reset"
	EventClientQuotaWarning  EventType = "client.quota_warning"
	EventClientExpiryWarning EventType = "client.expiry_warning"
)

// ClientEventData is the client an event is about, as it stood at the event.
type ClientEventData struct {
	Email      string `json:"email"`
	SubID      string `json:"subId,omitempty"`
	Up         int64  `json:"up"`
	Down       int64  `json:"down"`
	Total      int64  `json:"total"`      // quota in bytes, 0 = unlimited
	ExpiryTime int64  `json:"expiryTime"` // unix ms, 0 = never
	Percent    int    `json:"percent,omitempty"`
	DaysLeft   int    `json:"daysLeft,omitempty"`
}

func (d ClientEventData) Used() int64 { return d.Up + d.Down }

// ExpiryDate is the expiry in the server's local time, as notifications print it.
func (d ClientEventData) ExpiryDate() string {
	return time.UnixMilli(d.ExpiryTime).Format("2006-01-02 15:04")
}

// Event is the unit of information flowing through the bus.
type Event struct {
	Type      EventType
	Source    string    // outbound tag, node name, client email, IP, etc.
	Data      any       // event-specific payload, may be nil
	Timestamp time.Time // when the event was detected
}

// OutboundHealthData carries observatory details for outbound events.
type OutboundHealthData struct {
	Delay int64  // last measured delay in ms, 0 if unknown
	Error string // last error if probe failed, empty if up
}

// NodeHealthData carries heartbeat details for node events.
type NodeHealthData struct {
	NodeId    int
	LatencyMs int
	CpuPct    float64
	MemPct    float64
	XrayState string // "running", "stopped", etc.
	XrayError string
}

// LoginEventData carries login attempt details.
type LoginEventData struct {
	Username string
	IP       string
	Time     string
	Status   string // "success" or "fail"
	Reason   string
}

// SystemMetricData carries raw system metric values for threshold-based events.
type SystemMetricData struct {
	Percent   float64 // current usage percentage
	Threshold int     // configured threshold
}
