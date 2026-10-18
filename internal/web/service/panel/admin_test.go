package panel

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestListAdminsCarriesResellerCreditAndPrices(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, config.GetDBPath())
	perGB := int64(150)
	reseller := &model.User{Username: "shop", Password: "x", Role: model.AdminRoleOperator, ScopeInboundIds: "[]", ScopeGroups: "[]", PricePerGB: &perGB}
	if err := database.GetDB().Create(reseller).Error; err != nil {
		t.Fatal(err)
	}
	entries := []model.ResellerEntry{
		{UserId: reseller.Id, Amount: 1000, Kind: model.ResellerTopup},
		{UserId: reseller.Id, Amount: -350, Kind: model.ResellerCreate},
	}
	if err := database.GetDB().Create(&entries).Error; err != nil {
		t.Fatal(err)
	}

	admins, err := (&UserService{}).ListAdmins()
	if err != nil {
		t.Fatalf("ListAdmins: %v", err)
	}
	var got *AdminView
	for i := range admins {
		if admins[i].Id == reseller.Id {
			got = &admins[i]
		}
	}
	if got == nil {
		t.Fatal("reseller missing from the list")
	}
	if got.Credit != 650 {
		t.Fatalf("credit = %d, want 650", got.Credit)
	}
	if got.PricePerGB == nil || *got.PricePerGB != 150 || got.PricePerDay != nil {
		t.Fatalf("prices = %v / %v, want 150 and unset", got.PricePerGB, got.PricePerDay)
	}
}

func TestAdminKeepsItsTelegramChatForLoginCodes(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, config.GetDBPath())
	svc := &UserService{}
	created, err := svc.CreateAdmin(AdminInput{Username: "linked", Password: "pw-12345", Role: model.AdminRoleAdmin, TgChatId: 777})
	if err != nil || created.TgChatId != 777 {
		t.Fatalf("CreateAdmin = %+v, %v; want chat 777", created, err)
	}
	updated, err := svc.UpdateAdmin(0, created.Id, AdminInput{Username: "linked", Role: model.AdminRoleAdmin, TgChatId: 0})
	if err != nil || updated.TgChatId != 0 {
		t.Fatalf("UpdateAdmin = %+v, %v; want the chat unlinked", updated, err)
	}
	if _, err := svc.CreateAdmin(AdminInput{Username: "bad", Password: "pw-12345", Role: model.AdminRoleAdmin, TgChatId: -5}); err == nil {
		t.Fatal("a negative chat id was accepted")
	}
}
