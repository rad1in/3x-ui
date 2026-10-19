package panel

import (
	"encoding/json"
	"errors"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

const (
	maxDashboardWidgets  = 40
	dashboardWidgetIDMax = 40
	dashboardGridColumns = 12
)

// DashboardWidget is one overview widget: its slot in the order, whether it is
// hidden and how many of the 12 grid columns it spans.
type DashboardWidget struct {
	Id     string `json:"id" example:"clientGeo"`
	Hidden bool   `json:"hidden" example:"false"`
	Span   int    `json:"span" example:"6"`
}

// DashboardLayout is an admin's overview arrangement. Widgets absent from the
// list keep their default place, so a new widget never disappears.
type DashboardLayout struct {
	Widgets []DashboardWidget `json:"widgets"`
}

func validateDashboardLayout(in DashboardLayout) error {
	if len(in.Widgets) > maxDashboardWidgets {
		return errors.New("too many dashboard widgets")
	}
	seen := make(map[string]struct{}, len(in.Widgets))
	for _, w := range in.Widgets {
		if w.Id == "" || len(w.Id) > dashboardWidgetIDMax {
			return errors.New("invalid dashboard widget id")
		}
		if w.Span < 1 || w.Span > dashboardGridColumns {
			return errors.New("dashboard widget span must be between 1 and 12")
		}
		if _, dup := seen[w.Id]; dup {
			return errors.New("duplicate dashboard widget id")
		}
		seen[w.Id] = struct{}{}
	}
	return nil
}

func (s *UserService) GetDashboardLayout(userId int) (*DashboardLayout, error) {
	var u model.User
	if err := database.GetDB().Select("id", "dashboard_layout").First(&u, userId).Error; err != nil {
		return nil, ErrAdminNotFound
	}
	out := &DashboardLayout{Widgets: []DashboardWidget{}}
	if u.DashboardLayout == "" {
		return out, nil
	}
	if err := json.Unmarshal([]byte(u.DashboardLayout), out); err != nil || validateDashboardLayout(*out) != nil {
		return &DashboardLayout{Widgets: []DashboardWidget{}}, nil
	}
	if out.Widgets == nil {
		out.Widgets = []DashboardWidget{}
	}
	return out, nil
}

func (s *UserService) SetDashboardLayout(userId int, in DashboardLayout) error {
	if err := validateDashboardLayout(in); err != nil {
		return common.NewError(err.Error())
	}
	if in.Widgets == nil {
		in.Widgets = []DashboardWidget{}
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	res := database.GetDB().Model(&model.User{}).Where("id = ?", userId).Update("dashboard_layout", string(raw))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAdminNotFound
	}
	return nil
}
