package service

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// AdminScope is what an operator may see and change. A nil *AdminScope means
// unrestricted; every method treats it that way.
type AdminScope struct {
	InboundIds []int
	Groups     []string
}

// ScopeForUser returns the operator's scope, or nil for every other role.
func ScopeForUser(u *model.User) *AdminScope {
	if u == nil || u.Role != model.AdminRoleOperator {
		return nil
	}
	return &AdminScope{InboundIds: ParseScopeInboundIds(u.ScopeInboundIds), Groups: ParseScopeGroups(u.ScopeGroups)}
}

func ParseScopeInboundIds(raw string) []int {
	ids := []int{}
	_ = json.Unmarshal([]byte(raw), &ids)
	return ids
}

func ParseScopeGroups(raw string) []string {
	groups := []string{}
	_ = json.Unmarshal([]byte(raw), &groups)
	return groups
}

func (s *AdminScope) HasInbound(id int) bool {
	return s == nil || slices.Contains(s.InboundIds, id)
}

func (s *AdminScope) HasAllInbounds(ids []int) bool {
	for _, id := range ids {
		if !s.HasInbound(id) {
			return false
		}
	}
	return true
}

func (s *AdminScope) HasGroup(name string) bool {
	return s == nil || (name != "" && slices.Contains(s.Groups, strings.TrimSpace(name)))
}

// restrictClients narrows a query over clients aliased as alias.
func (s *AdminScope) restrictClients(tx *gorm.DB, alias string) *gorm.DB {
	if s == nil {
		return tx
	}
	inGroups := alias + ".group_name IN ?"
	inInbounds := alias + ".id IN (SELECT client_id FROM client_inbounds WHERE inbound_id IN ?)"
	switch {
	case len(s.Groups) > 0 && len(s.InboundIds) > 0:
		return tx.Where("("+inGroups+" OR "+inInbounds+")", s.Groups, s.InboundIds)
	case len(s.Groups) > 0:
		return tx.Where(inGroups, s.Groups)
	case len(s.InboundIds) > 0:
		return tx.Where(inInbounds, s.InboundIds)
	default:
		return tx.Where("1 = 0")
	}
}

// restrictInbounds narrows a query over inbounds to the scope; col is the id column.
func (s *AdminScope) restrictInbounds(tx *gorm.DB, col string) *gorm.DB {
	if s == nil {
		return tx
	}
	if len(s.InboundIds) == 0 {
		return tx.Where("1 = 0")
	}
	return tx.Where(col+" IN ?", s.InboundIds)
}

// VisibleEmails keeps the emails of clients inside the scope, in input order.
func (s *AdminScope) VisibleEmails(emails []string) ([]string, error) {
	if s == nil {
		return emails, nil
	}
	if len(emails) == 0 {
		return []string{}, nil
	}
	visible := make(map[string]struct{}, len(emails))
	for _, batch := range chunkStrings(emails, sqlInChunk) {
		var found []string
		tx := database.GetDB().Table("clients AS c").Where("c.email IN ?", batch)
		if err := s.restrictClients(tx, "c").Pluck("c.email", &found).Error; err != nil {
			return nil, err
		}
		for _, e := range found {
			visible[e] = struct{}{}
		}
	}
	out := make([]string, 0, len(visible))
	for _, e := range emails {
		if _, ok := visible[e]; ok {
			out = append(out, e)
		}
	}
	return out, nil
}

// HasAllClients reports whether every email names an existing client in scope.
func (s *AdminScope) HasAllClients(emails []string) (bool, error) {
	if s == nil {
		return true, nil
	}
	visible, err := s.VisibleEmails(emails)
	if err != nil {
		return false, err
	}
	return len(visible) == len(emails), nil
}

// HasClientWhere reports whether a client matching the condition is in scope.
func (s *AdminScope) HasClientWhere(cond string, args ...any) (bool, error) {
	if s == nil {
		return true, nil
	}
	var count int64
	tx := database.GetDB().Table("clients AS c").Where("c."+cond, args...)
	err := s.restrictClients(tx, "c").Count(&count).Error
	return count > 0, err
}

// ClientKeepsScope reports whether the operator may move the client to newGroup:
// it must be theirs now, the target group too, and it must stay visible after.
func (s *AdminScope) ClientKeepsScope(email, newGroup string) (bool, error) {
	if s == nil {
		return true, nil
	}
	var rec model.ClientRecord
	err := s.restrictClients(database.GetDB().Table("clients AS c").Select("c.id, c.group_name").Where("c.email = ?", email), "c").
		Take(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if newGroup == rec.Group || s.HasGroup(newGroup) {
		return true, nil
	}
	if newGroup != "" {
		return false, nil
	}
	var attached int64
	err = database.GetDB().Model(&model.ClientInbound{}).
		Where("client_id = ? AND inbound_id IN ?", rec.Id, append([]int{-1}, s.InboundIds...)).
		Count(&attached).Error
	return attached > 0, err
}

func (s *AdminScope) KeepGroupNames(names []string) []string {
	if s == nil {
		return names
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if s.HasGroup(n) {
			out = append(out, n)
		}
	}
	return out
}

func (s *AdminScope) KeepGroups(groups []GroupSummary) []GroupSummary {
	if s == nil {
		return groups
	}
	out := make([]GroupSummary, 0, len(groups))
	for _, g := range groups {
		if s.HasGroup(g.Name) {
			out = append(out, g)
		}
	}
	return out
}

// KeepLastOnline drops the entries of clients outside the scope.
func (s *AdminScope) KeepLastOnline(byEmail map[string]int64) (map[string]int64, error) {
	if s == nil {
		return byEmail, nil
	}
	emails := make([]string, 0, len(byEmail))
	for e := range byEmail {
		emails = append(emails, e)
	}
	visible, err := s.VisibleEmails(emails)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(visible))
	for _, e := range visible {
		out[e] = byEmail[e]
	}
	return out, nil
}

// renameScopeGroup follows a group rename or delete (newName "") into every
// operator scope, so a rename never silently widens or empties one.
func renameScopeGroup(tx *gorm.DB, oldName, newName string) error {
	var users []model.User
	if err := tx.Where("role = ?", model.AdminRoleOperator).Find(&users).Error; err != nil {
		return err
	}
	for _, u := range users {
		groups := ParseScopeGroups(u.ScopeGroups)
		idx := slices.Index(groups, oldName)
		if idx < 0 {
			continue
		}
		groups = slices.Delete(groups, idx, idx+1)
		if newName != "" && !slices.Contains(groups, newName) {
			groups = append(groups, newName)
		}
		raw, _ := json.Marshal(groups)
		if err := tx.Model(&model.User{}).Where("id = ?", u.Id).Update("scope_groups", string(raw)).Error; err != nil {
			return err
		}
	}
	return nil
}
