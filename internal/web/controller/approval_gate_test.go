package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/wirecodec"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

func gateEngine(t *testing.T, gated bool) (*gin.Engine, *model.Node) {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	gin.SetMode(gin.TestMode)
	var owner model.User
	if err := database.GetDB().First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	second := &model.User{Username: "second", Password: "x", Role: model.AdminRoleAdmin}
	node := &model.Node{Name: "n1", Address: "127.0.0.1", Port: 2096, ApiToken: "tok", Enable: true, Status: "offline"}
	for _, row := range []any{second, node} {
		if err := database.GetDB().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	cfg := service.SecurityConfig{}
	if gated {
		cfg.ApprovalActions = []string{service.ApprovalNodeDelete}
	}
	if err := (&service.SecurityConfigService{}).Save(&cfg); err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	api := engine.Group("/panel/api")
	api.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		session.SetAPIAuthUser(c, &owner)
		c.Next()
	})
	NewNodeController(api.Group("/nodes"))
	return engine, node
}

func deleteNode(engine *gin.Engine, id int, header ...string) {
	req := httptest.NewRequest(http.MethodPost, "/panel/api/nodes/del/"+itoa(id), nil)
	req.RemoteAddr = "203.0.113.5:4000"
	if len(header) == 2 {
		req.Header.Set(header[0], header[1])
	}
	engine.ServeHTTP(httptest.NewRecorder(), req)
}

func itoa(n int) string { return strconv.Itoa(n) }

func count(t *testing.T, m any) int64 {
	t.Helper()
	var n int64
	if err := database.GetDB().Model(m).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestGatedNodeDeleteIsParkedNotExecuted(t *testing.T) {
	engine, node := gateEngine(t, true)
	deleteNode(engine, node.Id)
	if count(t, &model.Node{}) != 1 {
		t.Fatal("the node was deleted although deleting nodes needs a second admin")
	}
	var parked model.PendingAction
	if err := database.GetDB().First(&parked).Error; err != nil || parked.Action != service.ApprovalNodeDelete || parked.Status != service.ApprovalPending {
		t.Fatalf("no pending request recorded: %+v err=%v", parked, err)
	}
}

func TestUngatedNodeDeleteRunsAtOnce(t *testing.T) {
	engine, node := gateEngine(t, false)
	deleteNode(engine, node.Id)
	if count(t, &model.Node{}) != 0 || count(t, &model.PendingAction{}) != 0 {
		t.Fatal("with the gate off the delete must run directly")
	}
}

func TestMasterPushIsNeverHeldForApproval(t *testing.T) {
	engine, node := gateEngine(t, true)
	deleteNode(engine, node.Id, wirecodec.MasterPushHeader, "1")
	if count(t, &model.Node{}) != 0 || count(t, &model.PendingAction{}) != 0 {
		t.Fatal("a master's node-sync push was parked on the node")
	}
}
