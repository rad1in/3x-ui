package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/entity"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

const resellerGiB = int64(1) << 30

func resellerCtx(t *testing.T, actor *model.User, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(adminRoleKey, actor.Role)
	session.SetAPIAuthUser(c, actor)
	return c, w
}

func seedResellerDB(t *testing.T, role string, credit int64) (*model.User, int) {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	dbtest.InitDB(t, filepath.Join(dbDir, "eternal.db"))
	db := database.GetDB()
	perGB, perDay := int64(100), int64(10)
	user := &model.User{Username: "shop", Password: "x", Role: role, ScopeInboundIds: "[]", ScopeGroups: "[]", PricePerGB: &perGB, PricePerDay: &perDay}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	if credit > 0 {
		if err := db.Create(&model.ResellerEntry{UserId: user.Id, Amount: credit, Kind: model.ResellerTopup}).Error; err != nil {
			t.Fatal(err)
		}
	}
	ib := &model.Inbound{UserId: 1, Enable: true, Port: 43410, Tag: "reseller-in", Protocol: model.VLESS, Settings: `{"clients": []}`, StreamSettings: `{"network":"tcp","security":"none"}`}
	if err := db.Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	return user, ib.Id
}

func createBody(email string, inbound int, totalGB, expiry int64) map[string]any {
	return map[string]any{
		"client":     map[string]any{"email": email, "id": "11111111-2222-3333-4444-555555555555", "subId": "sub-" + email, "enable": true, "totalGB": totalGB, "expiryTime": expiry},
		"inboundIds": []int{inbound},
	}
}

func clientCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	database.GetDB().Model(&model.ClientRecord{}).Count(&n)
	return n
}

func TestResellerCreateThroughTheHandlerIsDebited(t *testing.T) {
	user, inbound := seedResellerDB(t, model.AdminRoleOperator, 5000)
	a := &ClientController{}
	expiry := time.Now().Add(30*24*time.Hour + time.Hour).UnixMilli()

	c, w := resellerCtx(t, user, createBody("sold", inbound, 2*resellerGiB, expiry))
	a.create(c)

	var msg entity.Msg
	if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil || !msg.Success {
		t.Fatalf("create answered %q (%v), want success", w.Body.String(), err)
	}
	balance, err := (&service.ResellerService{}).Balance(user.Id)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(5000 - (2*100 + 31*10)); balance != want {
		t.Fatalf("balance = %d, want %d", balance, want)
	}
}

func TestResellerWithoutCreditCannotCreateAClient(t *testing.T) {
	user, inbound := seedResellerDB(t, model.AdminRoleOperator, 50)
	a := &ClientController{}
	expiry := time.Now().Add(30 * 24 * time.Hour).UnixMilli()

	c, w := resellerCtx(t, user, createBody("sold", inbound, 2*resellerGiB, expiry))
	a.create(c)

	var msg entity.Msg
	if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil || msg.Success {
		t.Fatalf("create answered %q (%v), want a refusal", w.Body.String(), err)
	}
	if n := clientCount(t); n != 0 {
		t.Fatalf("%d clients created without credit", n)
	}
}

func TestResellerFailedCreateIsRefunded(t *testing.T) {
	user, _ := seedResellerDB(t, model.AdminRoleOperator, 5000)
	a := &ClientController{}
	expiry := time.Now().Add(30 * 24 * time.Hour).UnixMilli()

	c, w := resellerCtx(t, user, createBody("sold", 99999, resellerGiB, expiry))
	a.create(c)

	var msg entity.Msg
	if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil || msg.Success {
		t.Fatalf("create on a missing inbound answered %q (%v), want failure", w.Body.String(), err)
	}
	balance, _ := (&service.ResellerService{}).Balance(user.Id)
	if balance != 5000 {
		t.Fatalf("balance = %d, want the 5000 back after the failed create", balance)
	}
}

func TestResellerReportShowsAResellerOnlyItself(t *testing.T) {
	user, _ := seedResellerDB(t, model.AdminRoleOperator, 100)
	rival := &model.User{Username: "rival", Password: "x", Role: model.AdminRoleOperator, ScopeInboundIds: "[]", ScopeGroups: "[]"}
	if err := database.GetDB().Create(rival).Error; err != nil {
		t.Fatal(err)
	}
	owner := &model.User{Username: "boss", Password: "x", Role: model.AdminRoleOwner, ScopeInboundIds: "[]", ScopeGroups: "[]"}
	a := &ResellerController{}

	rows := func(actor *model.User) []service.ResellerReportRow {
		c, w := resellerCtx(t, actor, struct{}{})
		a.report(c)
		var msg struct {
			Success bool                   `json:"success"`
			Obj     service.ResellerReport `json:"obj"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil || !msg.Success {
			t.Fatalf("report answered %q (%v)", w.Body.String(), err)
		}
		return msg.Obj.Rows
	}
	if got := rows(user); len(got) != 1 || got[0].Username != "shop" {
		t.Fatalf("reseller report = %+v, want only its own row", got)
	}
	if got := rows(owner); len(got) != 2 {
		t.Fatalf("owner report has %d rows, want both resellers", len(got))
	}
}

func TestOwnersAreNeverBilled(t *testing.T) {
	user, inbound := seedResellerDB(t, model.AdminRoleOwner, 0)
	a := &ClientController{}

	c, w := resellerCtx(t, user, createBody("free", inbound, 0, 0))
	a.create(c)

	var msg entity.Msg
	if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil || !msg.Success {
		t.Fatalf("owner create answered %q (%v), want success without credit", w.Body.String(), err)
	}
}
