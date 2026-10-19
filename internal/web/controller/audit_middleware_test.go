package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

func newAuditTestEngine(t *testing.T) *gin.Engine {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/panel/api")
	api.Use(func(c *gin.Context) {
		c.Set("I18n", func(_ locale.I18nType, key string, _ ...string) string { return key })
		if name := c.GetHeader("X-Test-Token"); name != "" {
			c.Set(apiTokenNameKey, name)
		}
		session.SetAPIAuthUser(c, &model.User{Id: 1, Username: "root"})
		c.Next()
	})
	api.Use(auditMiddleware(&service.AuditService{}))
	api.POST("/clients/update/:email", func(c *gin.Context) { jsonMsg(c, "", nil) })
	api.POST("/clients/del/:email", func(c *gin.Context) { jsonMsg(c, "", errors.New("not found")) })
	api.POST("/clients/onlines", func(c *gin.Context) { jsonObj(c, []string{}, nil) })
	api.GET("/clients/list", func(c *gin.Context) { jsonObj(c, []string{}, nil) })
	return engine
}

func call(engine *gin.Engine, method, path string, header ...string) {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "203.0.113.5:4000"
	if len(header) == 2 {
		req.Header.Set(header[0], header[1])
	}
	engine.ServeHTTP(httptest.NewRecorder(), req)
}

func auditRows(t *testing.T) []model.AuditLog {
	t.Helper()
	page, err := (&service.AuditService{}).List(service.AuditListParams{})
	if err != nil {
		t.Fatal(err)
	}
	return page.Items
}

func TestAuditMiddlewareRecordsMutationsWithOutcome(t *testing.T) {
	engine := newAuditTestEngine(t)

	call(engine, http.MethodPost, "/panel/api/clients/update/alice@x")
	call(engine, http.MethodPost, "/panel/api/clients/del/bob@x", "X-Test-Token", "billing-bot")

	rows := auditRows(t)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want 2", rows)
	}
	del, upd := rows[0], rows[1]
	if upd.Actor != "root" || upd.Ip != "203.0.113.5" || upd.Action != "POST /clients/update/:email" ||
		upd.Target != "email=alice@x" || !upd.Success {
		t.Errorf("update row = %+v", upd)
	}
	if del.Actor != "token:billing-bot" || del.Action != "POST /clients/del/:email" || del.Success {
		t.Errorf("failed delete must be recorded as a failure by its token, got %+v", del)
	}
}

func TestAuditMiddlewareSkipsReads(t *testing.T) {
	engine := newAuditTestEngine(t)

	call(engine, http.MethodGet, "/panel/api/clients/list")
	call(engine, http.MethodPost, "/panel/api/clients/onlines")

	if rows := auditRows(t); len(rows) != 0 {
		t.Fatalf("reads must not be audited, got %+v", rows)
	}
}
