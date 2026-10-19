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

	// A node reached its monthly traffic cap; Source is the node name, Data a *NodeCapData.
	EventNodeCapReached EventType = "node.cap_reached"

	// An inbound port stopped answering the external probe, then answered again;
	// Source is the inbound's remark and Data a *PortProbeData.
	EventPortBlocked  EventType = "port.blocked"
	EventPortRestored EventType = "port.restored"

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

	// Weekly summary; Data is a *WeeklySummaryData.
	EventWeeklyReport EventType = "report.weekly"

	// An admin signed in from an address or country not seen before; Source is
	// the username, Data a *NewLoginLocationData.
	EventLoginNewLocation EventType = "login.new_location"

	// A client is being used from more addresses or countries than the shared-account
	// thresholds allow; Source is the client's email, Data a *SharedAccountData.
	EventClientSharedAccount EventType = "client.shared_account"

	// A customer opened a support ticket, or replied to one; Source is
	// "ticket-<id>", Data a *TicketEventData.
	EventTicketOpened  EventType = "ticket.opened"
	EventTicketReplied EventType = "ticket.replied"
)

// TicketEventData is the ticket a support event is about. Excerpt is the start
// of the message, plain text.
type TicketEventData struct {
	TicketId int    `json:"ticketId"`
	Email    string `json:"email"`
	Subject  string `json:"subject"`
	Excerpt  string `json:"excerpt"`
}

// SharedAccountData describes a client that looks shared between several people.
type SharedAccountData struct {
	Email        string `json:"email"`
	IPCount      int    `json:"ipCount"`
	CountryCount int    `json:"countryCount"`
	Countries    string `json:"countries,omitempty"`
	WindowHours  int    `json:"windowHours"`
}

// NewLoginLocationData describes a sign-in from an unfamiliar place. Country is
// empty when no geoip database could place the address.
type NewLoginLocationData struct {
	Username   string `json:"username"`
	IP         string `json:"ip"`
	Country    string `json:"country"`
	NewCountry bool   `json:"newCountry"`
}

// TopUser is one client's traffic over the report window.
type TopUser struct {
	Email string `json:"email"`
	Bytes int64  `json:"bytes"`
}

// WeeklySummaryData is the week the panel reports on. FirstReport means no
// earlier snapshot existed, so the traffic figures are totals since the start.
type WeeklySummaryData struct {
	SinceMs         int64     `json:"sinceMs"`
	UntilMs         int64     `json:"untilMs"`
	Bytes           int64     `json:"bytes"`
	TotalClients    int       `json:"totalClients"`
	NewClients      int       `json:"newClients"`
	ExpiredClients  int       `json:"expiredClients"`
	DepletedClients int       `json:"depletedClients"`
	FirstReport     bool      `json:"firstReport"`
	TopUsers        []TopUser `json:"topUsers"`
}

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

// NodeCapData describes the cap a node just reached.
type NodeCapData struct {
	NodeId    int
	UsedBytes int64
	CapBytes  int64
	Stopped   bool // true when the cap action also disabled the node's inbounds
}

// PortProbeData describes the inbound port a probe event is about. Method is
// "tcp" (dial from the panel host) or "url" (the configured probe URL).
type PortProbeData struct {
	InboundId int
	Tag       string
	Host      string
	Port      int
	Method    string
	Error     string // last probe error, empty on port.restored
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
