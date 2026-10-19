package model

// Ticket statuses: a customer message leaves a ticket open, an admin reply
// marks it answered, and a closed ticket takes no replies until reopened.
const (
	TicketOpen     = "open"
	TicketAnswered = "answered"
	TicketClosed   = "closed"
)

// Authors of a ticket message.
const (
	TicketByCustomer = "customer"
	TicketByAdmin    = "admin"
)

// Ticket is a support thread opened by a customer, who is the client with
// ClientEmail. Subject and every message body are plain text.
type Ticket struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	ClientEmail string `json:"clientEmail" gorm:"column:client_email;index;not null" example:"alice"`
	Subject     string `json:"subject" gorm:"not null" example:"Connection drops every hour"`
	Status      string `json:"status" gorm:"not null;index" example:"open"`
	CreatedAt   int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
	UpdatedAt   int64  `json:"updatedAt" gorm:"autoUpdateTime:milli"`
}

func (Ticket) TableName() string { return "tickets" }

// TicketMessage is one line of a ticket's conversation.
type TicketMessage struct {
	Id        int    `json:"id" gorm:"primaryKey;autoIncrement" example:"1"`
	TicketId  int    `json:"ticketId" gorm:"column:ticket_id;index;not null" example:"1"`
	Author    string `json:"author" gorm:"not null" example:"customer"`
	Body      string `json:"body" gorm:"not null" example:"It disconnects about once an hour."`
	CreatedAt int64  `json:"createdAt" gorm:"autoCreateTime:milli"`
}

func (TicketMessage) TableName() string { return "ticket_messages" }
