package service

import (
	"errors"
	"slices"
	"strings"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

var ErrTransferAdmins = errors.New("a transfer needs two different operators")

// Why a client was not transferred.
const (
	TransferNotSource       = "not_source_client"
	TransferSharedInbound   = "shared_inbound"
	TransferNoTargetInbound = "no_target_inbound"
	TransferNeedsGroup      = "needs_group"
	TransferFailed          = "failed"
)

// ClientTransferRequest hands clients from one operator to another. Emails
// empty means every client the source can see; Group is the target's group the
// source's group clients move into (optional when the target has only one).
type ClientTransferRequest struct {
	FromAdminId int      `json:"fromAdminId" example:"2"`
	ToAdminId   int      `json:"toAdminId" example:"3"`
	Emails      []string `json:"emails"`
	Group       string   `json:"group" example:"team-b"`
}

// ClientTransferSkip names a client left with the source and why.
type ClientTransferSkip struct {
	Email  string `json:"email" example:"alice"`
	Reason string `json:"reason" example:"shared_inbound"`
}

// ClientTransferResult lists what moved and what stayed.
type ClientTransferResult struct {
	Moved   []string             `json:"moved"`
	Skipped []ClientTransferSkip `json:"skipped"`
}

// ClientTransferService moves clients between the scopes of two operators: a
// client in the source's group joins the target's group, and one on the
// source's inbounds is attached to the target's and detached from the source's.
type ClientTransferService struct {
	clients ClientService
}

type transferPlan struct {
	email      string
	toGroup    string
	srcInbound []int
}

func loadOperator(id int) (*model.User, error) {
	var u model.User
	err := database.GetDB().Where("id = ? AND role = ?", id, model.AdminRoleOperator).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTransferAdmins
	}
	return &u, err
}

func (s *ClientTransferService) candidates(from *AdminScope, requested []string, res *ClientTransferResult) ([]string, error) {
	if len(requested) == 0 {
		var all []string
		tx := from.restrictClients(database.GetDB().Table("clients AS c"), "c")
		return all, tx.Order("c.email").Pluck("c.email", &all).Error
	}
	wanted := trimmedUniqueEmails(requested)
	visible, err := from.VisibleEmails(wanted)
	if err != nil {
		return nil, err
	}
	for _, e := range wanted {
		if !slices.Contains(visible, e) {
			res.Skipped = append(res.Skipped, ClientTransferSkip{Email: e, Reason: TransferNotSource})
		}
	}
	return visible, nil
}

func (s *ClientTransferService) targetGroup(to *AdminScope, requested string) string {
	requested = strings.TrimSpace(requested)
	if requested == "" && len(to.Groups) == 1 {
		return to.Groups[0]
	}
	return requested
}

// Transfer moves the clients and reports each one it left behind. The bool is
// true when Xray needs a restart.
func (s *ClientTransferService) Transfer(inboundSvc *InboundService, req ClientTransferRequest) (*ClientTransferResult, bool, error) {
	res := &ClientTransferResult{Moved: []string{}, Skipped: []ClientTransferSkip{}}
	if req.FromAdminId == req.ToAdminId {
		return nil, false, ErrTransferAdmins
	}
	fromUser, err := loadOperator(req.FromAdminId)
	if err != nil {
		return nil, false, err
	}
	toUser, err := loadOperator(req.ToAdminId)
	if err != nil {
		return nil, false, err
	}
	from, to := ScopeForUser(fromUser), ScopeForUser(toUser)
	emails, err := s.candidates(from, req.Emails, res)
	if err != nil {
		return nil, false, err
	}
	plans, err := s.plan(from, to, s.targetGroup(to, req.Group), emails, res)
	if err != nil {
		return nil, false, err
	}
	needRestart, err := s.apply(inboundSvc, to, plans)
	if err != nil {
		return nil, needRestart, err
	}
	return res, needRestart, s.verify(from, to, plans, res)
}

func (s *ClientTransferService) plan(from, to *AdminScope, group string, emails []string, res *ClientTransferResult) ([]transferPlan, error) {
	skip := func(email, reason string) {
		res.Skipped = append(res.Skipped, ClientTransferSkip{Email: email, Reason: reason})
	}
	db := database.GetDB()
	var plans []transferPlan
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var recs []model.ClientRecord
		if err := db.Where("email IN ?", batch).Find(&recs).Error; err != nil {
			return nil, err
		}
		for _, rec := range recs {
			var attached []int
			err := db.Model(&model.ClientInbound{}).Where("client_id = ?", rec.Id).Pluck("inbound_id", &attached).Error
			if err != nil {
				return nil, err
			}
			p := transferPlan{email: rec.Email}
			for _, id := range attached {
				if from.HasInbound(id) {
					p.srcInbound = append(p.srcInbound, id)
				}
			}
			if rec.Group != "" && from.HasGroup(rec.Group) {
				if group == "" || !to.HasGroup(group) || from.HasGroup(group) {
					skip(rec.Email, TransferNeedsGroup)
					continue
				}
				p.toGroup = group
			}
			if len(p.srcInbound) > 0 {
				if len(to.InboundIds) == 0 {
					skip(rec.Email, TransferNoTargetInbound)
					continue
				}
				if slices.ContainsFunc(p.srcInbound, to.HasInbound) {
					skip(rec.Email, TransferSharedInbound)
					continue
				}
			}
			plans = append(plans, p)
		}
	}
	return plans, nil
}

func (s *ClientTransferService) apply(inboundSvc *InboundService, to *AdminScope, plans []transferPlan) (bool, error) {
	var groupMoves, inboundMoves []string
	var group string
	for _, p := range plans {
		if p.toGroup != "" {
			groupMoves, group = append(groupMoves, p.email), p.toGroup
		}
		if len(p.srcInbound) > 0 {
			inboundMoves = append(inboundMoves, p.email)
		}
	}
	if _, err := s.clients.AddToGroup(groupMoves, group); err != nil {
		return false, err
	}
	if len(inboundMoves) == 0 {
		return false, nil
	}
	_, restartAttach, err := s.clients.BulkAttach(inboundSvc, inboundMoves, to.InboundIds)
	if err != nil {
		return restartAttach, err
	}
	var sources []int
	for _, p := range plans {
		for _, id := range p.srcInbound {
			if !slices.Contains(sources, id) {
				sources = append(sources, id)
			}
		}
	}
	_, restartDetach, err := s.clients.BulkDetach(inboundSvc, inboundMoves, sources)
	return restartAttach || restartDetach, err
}

// verify moves a client to Moved only once the source no longer sees it and the
// target does, whatever the operations reported.
func (s *ClientTransferService) verify(from, to *AdminScope, plans []transferPlan, res *ClientTransferResult) error {
	planned := make([]string, 0, len(plans))
	for _, p := range plans {
		planned = append(planned, p.email)
	}
	stillSource, err := from.VisibleEmails(planned)
	if err != nil {
		return err
	}
	nowTarget, err := to.VisibleEmails(planned)
	if err != nil {
		return err
	}
	for _, e := range planned {
		if !slices.Contains(stillSource, e) && slices.Contains(nowTarget, e) {
			res.Moved = append(res.Moved, e)
		} else {
			res.Skipped = append(res.Skipped, ClientTransferSkip{Email: e, Reason: TransferFailed})
		}
	}
	return nil
}
