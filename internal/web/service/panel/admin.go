package panel

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

var (
	ErrLastOwner        = errors.New("the panel must keep at least one owner")
	ErrAdminSelfChange  = errors.New("you cannot change your own role or delete yourself")
	ErrAdminNameTaken   = errors.New("an admin with that username already exists")
	ErrAdminNotFound    = errors.New("admin not found")
	ErrAdminUnknownRole = errors.New("role must be owner, admin, operator or viewer")
)

const adminUsernameMaxLen = 64

// AdminView is an admin account as the owner sees it; it never carries the hash.
type AdminView struct {
	Id         int      `json:"id" example:"2"`
	Username   string   `json:"username" example:"support"`
	Role       string   `json:"role" example:"operator"`
	InboundIds []int    `json:"inboundIds"`
	Groups     []string `json:"groups"`
	CreatedAt  int64    `json:"createdAt" example:"1760684400000"`
	TgChatId   int64    `json:"tgChatId" example:"123456789"`
	// A reseller's credit and own price list; a nil price follows the panel's list.
	Credit      int64  `json:"credit" example:"8500"`
	PricePerGB  *int64 `json:"pricePerGb" example:"100"`
	PricePerDay *int64 `json:"pricePerDay" example:"10"`
}

// AdminInput creates or edits an admin. An empty Password on edit keeps it.
type AdminInput struct {
	Username   string   `json:"username" example:"support"`
	Password   string   `json:"password" example:"a-long-password"`
	Role       string   `json:"role" example:"operator"`
	InboundIds []int    `json:"inboundIds"`
	Groups     []string `json:"groups"`
	TgChatId   int64    `json:"tgChatId" example:"123456789"`
}

func ToAdminView(u *model.User) AdminView {
	return AdminView{
		Id:         u.Id,
		Username:   u.Username,
		Role:       u.Role,
		InboundIds: service.ParseScopeInboundIds(u.ScopeInboundIds),
		Groups:     service.ParseScopeGroups(u.ScopeGroups),
		CreatedAt:  u.CreatedAt,
		TgChatId:   u.TgChatId,

		PricePerGB:  u.PricePerGB,
		PricePerDay: u.PricePerDay,
	}
}

func (s *UserService) ListAdmins() ([]AdminView, error) {
	var users []model.User
	if err := database.GetDB().Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	var credits []struct {
		UserId int
		Sum    int64
	}
	err := database.GetDB().Model(&model.ResellerEntry{}).Group("user_id").
		Select("user_id, COALESCE(SUM(amount), 0) AS sum").Scan(&credits).Error
	if err != nil {
		return nil, err
	}
	creditOf := make(map[int]int64, len(credits))
	for _, c := range credits {
		creditOf[c.UserId] = c.Sum
	}
	out := make([]AdminView, 0, len(users))
	for i := range users {
		view := ToAdminView(&users[i])
		view.Credit = creditOf[users[i].Id]
		out = append(out, view)
	}
	return out, nil
}

func (s *UserService) CreateAdmin(in AdminInput) (*AdminView, error) {
	if in.Password == "" {
		return nil, common.NewError("password can not be empty")
	}
	row := &model.User{CreatedAt: time.Now().UnixMilli()}
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := applyAdminInput(tx, row, in); err != nil {
			return err
		}
		return tx.Create(row).Error
	})
	if err != nil {
		return nil, err
	}
	view := ToAdminView(row)
	return &view, nil
}

// UpdateAdmin edits another admin, or the actor's own name and password. A new
// password bumps the login epoch so that account's open sessions end.
func (s *UserService) UpdateAdmin(actorId, id int, in AdminInput) (*AdminView, error) {
	var row model.User
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAdminNotFound
			}
			return err
		}
		oldRole, oldInbounds, oldGroups := row.Role, row.ScopeInboundIds, row.ScopeGroups
		if err := applyAdminInput(tx, &row, in); err != nil {
			return err
		}
		if actorId == id && (row.Role != oldRole || row.ScopeInboundIds != oldInbounds || row.ScopeGroups != oldGroups) {
			return ErrAdminSelfChange
		}
		if oldRole == model.AdminRoleOwner && row.Role != model.AdminRoleOwner {
			if err := requireAnotherOwner(tx, id); err != nil {
				return err
			}
		}
		if in.Password != "" {
			row.LoginEpoch++
			if err := tx.Where("user_id = ?", id).Delete(&model.AdminSession{}).Error; err != nil {
				return err
			}
		}
		return tx.Save(&row).Error
	})
	if err != nil {
		return nil, err
	}
	view := ToAdminView(&row)
	return &view, nil
}

func (s *UserService) DeleteAdmin(actorId, id int) error {
	if actorId == id {
		return ErrAdminSelfChange
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		var row model.User
		if err := tx.Where("id = ?", id).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAdminNotFound
			}
			return err
		}
		if row.Role == model.AdminRoleOwner {
			if err := requireAnotherOwner(tx, id); err != nil {
				return err
			}
		}
		if err := tx.Where("user_id = ?", id).Delete(&model.AdminSession{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.User{}, id).Error
	})
}

func requireAnotherOwner(tx *gorm.DB, exceptId int) error {
	var owners int64
	if err := tx.Model(&model.User{}).Where("role = ? AND id <> ?", model.AdminRoleOwner, exceptId).Count(&owners).Error; err != nil {
		return err
	}
	if owners == 0 {
		return ErrLastOwner
	}
	return nil
}

func applyAdminInput(tx *gorm.DB, row *model.User, in AdminInput) error {
	username := strings.TrimSpace(in.Username)
	if username == "" {
		return common.NewError("username can not be empty")
	}
	if len(username) > adminUsernameMaxLen {
		return common.NewError("username must be 64 characters or fewer")
	}
	var taken int64
	if err := tx.Model(&model.User{}).Where("LOWER(username) = ? AND id <> ?", strings.ToLower(username), row.Id).Count(&taken).Error; err != nil {
		return err
	}
	if taken > 0 {
		return ErrAdminNameTaken
	}
	role := strings.ToLower(strings.TrimSpace(in.Role))
	if !model.IsKnownAdminRole(role) {
		return ErrAdminUnknownRole
	}
	inboundIds, groups := []int{}, []string{}
	if role == model.AdminRoleOperator {
		var err error
		if inboundIds, err = existingInboundIds(tx, in.InboundIds); err != nil {
			return err
		}
		groups = normalizeGroups(in.Groups)
	}
	if in.Password != "" {
		hash, err := crypto.HashPasswordAsBcrypt(in.Password)
		if err != nil {
			return err
		}
		row.Password = hash
	}
	rawInbounds, _ := json.Marshal(inboundIds)
	rawGroups, _ := json.Marshal(groups)
	if in.TgChatId < 0 {
		return common.NewError("telegram chat id cannot be negative")
	}
	row.TgChatId = in.TgChatId
	row.Username = username
	row.Role = role
	row.ScopeInboundIds = string(rawInbounds)
	row.ScopeGroups = string(rawGroups)
	return nil
}

func existingInboundIds(tx *gorm.DB, ids []int) ([]int, error) {
	want := slices.Compact(slices.Sorted(slices.Values(ids)))
	if len(want) == 0 {
		return []int{}, nil
	}
	var found []int
	if err := tx.Model(&model.Inbound{}).Where("id IN ?", want).Order("id ASC").Pluck("id", &found).Error; err != nil {
		return nil, err
	}
	if len(found) != len(want) {
		return nil, common.NewError("one of the selected inbounds no longer exists")
	}
	return found, nil
}

func normalizeGroups(groups []string) []string {
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		g = strings.TrimSpace(g)
		if g != "" && !slices.Contains(out, g) {
			out = append(out, g)
		}
	}
	return out
}
