package service

import (
	"crypto/subtle"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"

	"gorm.io/gorm"
)

const (
	maxTicketSubject           = 120
	maxTicketBody              = 2000
	maxOpenTickets             = 5
	maxCustomerMessagesPerHour = 20
	maxTicketPage              = 200
	ticketExcerptLen           = 200
)

// ticketMu makes each limit check and the write it guards one step.
var ticketMu sync.Mutex

// TicketAuth is how a customer proves which account they speak for: the
// subscription id of the client (as for the wallet), or the token of one of
// its orders.
type TicketAuth struct {
	Email      string
	Proof      string
	OrderToken string
}

// TicketDetail is a ticket with its whole conversation, oldest message first.
type TicketDetail struct {
	Ticket   model.Ticket          `json:"ticket"`
	Messages []model.TicketMessage `json:"messages"`
}

// TicketPage is one page of tickets, most recently active first, with the
// unpaged total.
type TicketPage struct {
	Items []model.Ticket `json:"items"`
	Total int64          `json:"total"`
}

// TicketService runs support tickets between customers and admins.
type TicketService struct {
	orders  OrderService
	clients ClientService
}

// account resolves auth to the client username it speaks for.
func (s *TicketService) account(auth TicketAuth) (string, error) {
	if auth.OrderToken != "" {
		order, err := s.orders.GetOrderByToken(auth.OrderToken)
		if err != nil {
			return "", err
		}
		return order.ClientEmail, nil
	}
	rec, err := s.clients.GetRecordByEmail(nil, strings.TrimSpace(auth.Email))
	if err != nil || auth.Proof == "" || subtle.ConstantTimeCompare([]byte(rec.SubID), []byte(auth.Proof)) != 1 {
		return "", salesErr(ErrProofInvalid)
	}
	return rec.Email, nil
}

// cleanText keeps a message to plain text: CRLF folds to LF, control
// characters and the bidi override/isolate marks that spoof text direction
// are dropped. Markup is left as the literal characters the customer typed;
// every renderer escapes it.
func cleanText(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case unicode.IsControl(r), r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func validText(s string, max int) bool {
	return s != "" && len([]rune(s)) <= max
}

func publishTicket(typ eventbus.EventType, t *model.Ticket, body string) {
	if eventBus == nil {
		return
	}
	eventBus.Publish(eventbus.Event{
		Type:   typ,
		Source: "ticket-" + strconv.Itoa(t.Id),
		Data: &eventbus.TicketEventData{
			TicketId: t.Id, Email: t.ClientEmail, Subject: t.Subject, Excerpt: truncate(body, ticketExcerptLen),
		},
		Timestamp: time.Now(),
	})
}

func (s *TicketService) detail(db *gorm.DB, id int) (*TicketDetail, error) {
	d := &TicketDetail{Messages: []model.TicketMessage{}}
	if err := db.First(&d.Ticket, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, salesErr(ErrTicketNotFound)
		}
		return nil, err
	}
	err := db.Where("ticket_id = ?", id).Order("id asc").Find(&d.Messages).Error
	return d, err
}

// checkCustomerRoom refuses a customer message beyond the hourly allowance.
func checkCustomerRoom(db *gorm.DB, email string) error {
	var n int64
	err := db.Model(&model.TicketMessage{}).
		Joins("JOIN tickets ON tickets.id = ticket_messages.ticket_id").
		Where("tickets.client_email = ? AND ticket_messages.author = ? AND ticket_messages.created_at > ?",
			email, model.TicketByCustomer, time.Now().Add(-time.Hour).UnixMilli()).
		Count(&n).Error
	if err != nil {
		return err
	}
	if n >= maxCustomerMessagesPerHour {
		return salesErr(ErrRateLimited)
	}
	return nil
}

// Open starts a ticket for the account auth speaks for.
func (s *TicketService) Open(auth TicketAuth, subject, body string) (*TicketDetail, error) {
	email, err := s.account(auth)
	if err != nil {
		return nil, err
	}
	subject, body = cleanText(subject), cleanText(body)
	if !validText(subject, maxTicketSubject) || !validText(body, maxTicketBody) {
		return nil, salesErr(ErrTicketInvalid)
	}
	ticketMu.Lock()
	defer ticketMu.Unlock()
	db := database.GetDB()
	var open int64
	if err := db.Model(&model.Ticket{}).Where("client_email = ? AND status <> ?", email, model.TicketClosed).Count(&open).Error; err != nil {
		return nil, err
	}
	if open >= maxOpenTickets {
		return nil, salesErr(ErrTicketLimit)
	}
	if err := checkCustomerRoom(db, email); err != nil {
		return nil, err
	}
	ticket := &model.Ticket{ClientEmail: email, Subject: subject, Status: model.TicketOpen}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(ticket).Error; err != nil {
			return err
		}
		return tx.Create(&model.TicketMessage{TicketId: ticket.Id, Author: model.TicketByCustomer, Body: body}).Error
	})
	if err != nil {
		return nil, err
	}
	publishTicket(eventbus.EventTicketOpened, ticket, body)
	return s.detail(db, ticket.Id)
}

// CustomerList lists the account's tickets, most recently active first.
func (s *TicketService) CustomerList(auth TicketAuth) ([]model.Ticket, error) {
	email, err := s.account(auth)
	if err != nil {
		return nil, err
	}
	tickets := []model.Ticket{}
	err = database.GetDB().Where("client_email = ?", email).Order("updated_at desc, id desc").Limit(maxTicketPage).Find(&tickets).Error
	return tickets, err
}

// ownTicket loads a ticket of the account; another account's is not found.
func (s *TicketService) ownTicket(auth TicketAuth, id int) (string, *TicketDetail, error) {
	email, err := s.account(auth)
	if err != nil {
		return "", nil, err
	}
	d, err := s.detail(database.GetDB(), id)
	if err != nil {
		return "", nil, err
	}
	if d.Ticket.ClientEmail != email {
		return "", nil, salesErr(ErrTicketNotFound)
	}
	return email, d, nil
}

// CustomerGet returns one of the account's tickets with its messages.
func (s *TicketService) CustomerGet(auth TicketAuth, id int) (*TicketDetail, error) {
	_, d, err := s.ownTicket(auth, id)
	return d, err
}

// CustomerReply adds the customer's message and leaves the ticket open for the
// admins.
func (s *TicketService) CustomerReply(auth TicketAuth, id int, body string) (*TicketDetail, error) {
	email, d, err := s.ownTicket(auth, id)
	if err != nil {
		return nil, err
	}
	body = cleanText(body)
	if !validText(body, maxTicketBody) {
		return nil, salesErr(ErrTicketInvalid)
	}
	ticketMu.Lock()
	defer ticketMu.Unlock()
	db := database.GetDB()
	if err := checkCustomerRoom(db, email); err != nil {
		return nil, err
	}
	if err := s.addMessage(db, d.Ticket.Id, model.TicketByCustomer, body, model.TicketOpen); err != nil {
		return nil, err
	}
	publishTicket(eventbus.EventTicketReplied, &d.Ticket, body)
	return s.detail(db, id)
}

// addMessage appends a message and sets the status in one step; a closed
// ticket takes none.
func (s *TicketService) addMessage(db *gorm.DB, id int, author, body, status string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.Ticket{}).Where("id = ? AND status <> ?", id, model.TicketClosed).Update("status", status)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			if _, err := s.detail(tx, id); err != nil {
				return err
			}
			return salesErr(ErrTicketClosed)
		}
		return tx.Create(&model.TicketMessage{TicketId: id, Author: author, Body: body}).Error
	})
}

// List pages tickets for the admin, most recently active first; an empty
// status lists every status.
func (s *TicketService) List(status string, limit, offset int) (*TicketPage, error) {
	if limit <= 0 || limit > maxTicketPage {
		limit = maxTicketPage
	}
	if offset < 0 {
		offset = 0
	}
	q := database.GetDB().Model(&model.Ticket{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	page := &TicketPage{Items: []model.Ticket{}}
	if err := q.Count(&page.Total).Error; err != nil {
		return nil, err
	}
	err := q.Order("updated_at desc, id desc").Limit(limit).Offset(offset).Find(&page.Items).Error
	return page, err
}

// Get returns a ticket with its messages.
func (s *TicketService) Get(id int) (*TicketDetail, error) {
	return s.detail(database.GetDB(), id)
}

// AdminReply adds an admin's message and marks the ticket answered.
func (s *TicketService) AdminReply(id int, body string) (*TicketDetail, error) {
	body = cleanText(body)
	if !validText(body, maxTicketBody) {
		return nil, salesErr(ErrTicketInvalid)
	}
	db := database.GetDB()
	if err := s.addMessage(db, id, model.TicketByAdmin, body, model.TicketAnswered); err != nil {
		return nil, err
	}
	return s.detail(db, id)
}

func (s *TicketService) setStatus(id int, from []string, to string) (*model.Ticket, error) {
	db := database.GetDB()
	d, err := s.detail(db, id)
	if err != nil {
		return nil, err
	}
	if err := db.Model(&model.Ticket{}).Where("id = ? AND status IN ?", id, from).Update("status", to).Error; err != nil {
		return nil, err
	}
	d, err = s.detail(db, id)
	if err != nil {
		return nil, err
	}
	return &d.Ticket, nil
}

// Close ends a ticket; the customer can no longer reply.
func (s *TicketService) Close(id int) (*model.Ticket, error) {
	return s.setStatus(id, []string{model.TicketOpen, model.TicketAnswered}, model.TicketClosed)
}

// Reopen takes a closed ticket back to open.
func (s *TicketService) Reopen(id int) (*model.Ticket, error) {
	return s.setStatus(id, []string{model.TicketClosed}, model.TicketOpen)
}
