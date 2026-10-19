package panel

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

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
