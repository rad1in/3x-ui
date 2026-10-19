package controller

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

func setMaintenance(t *testing.T, on bool) {
	t.Helper()
	if err := (&service.SettingService{}).SetMaintenance(on, "back at noon"); err != nil {
		t.Fatal(err)
	}
}

func setAdminRole(t *testing.T, role string) {
	t.Helper()
	if err := database.GetDB().Model(&model.User{}).Where("1 = 1").Update("role", role).Error; err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceRefusesEveryRoleButTheOwnerAtLogin(t *testing.T) {
	h := newLoginHarness(t)
	setMaintenance(t, true)

	if !h.login(credentials(nil)) {
		t.Fatal("the owner was locked out during maintenance")
	}
	setAdminRole(t, model.AdminRoleAdmin)
	if h.login(credentials(nil)) {
		t.Fatal("a non-owner admin logged in during maintenance")
	}
	setMaintenance(t, false)
	if !h.login(credentials(nil)) {
		t.Fatal("the non-owner admin stayed locked out after maintenance ended")
	}
}
