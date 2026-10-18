package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"time"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service/panel"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

type rbacFixture struct {
	server              *httptest.Server
	userIds             map[string]int
	inScope, outOfScope int
}

// newRBACFixture serves the real /panel/api router behind sessions. The
// operator owns inbound "in" and group "team": alice sits on "in", bob on
// "out" and carol only in "team".
func newRBACFixture(t *testing.T) *rbacFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	dbtest.InitDB(t, filepath.Join(dbDir, "x-ui.db"))
	db := database.GetDB()

	f := &rbacFixture{userIds: map[string]int{}}
	ids := map[string]int{}
	for i, tag := range []string{"in", "out"} {
		ib := &model.Inbound{
			UserId: 1, Enable: true, Port: 45100 + i, Tag: tag, Remark: tag,
			Protocol: model.VLESS, Settings: `{"clients": []}`,
			StreamSettings: `{"network":"tcp","security":"none"}`,
		}
		if err := db.Create(ib).Error; err != nil {
			t.Fatalf("create inbound %s: %v", tag, err)
		}
		ids[tag] = ib.Id
	}
	f.inScope, f.outOfScope = ids["in"], ids["out"]
	clients := []struct {
		email, group string
		inbounds     []int
	}{
		{"alice", "", []int{f.inScope}},
		{"bob", "", []int{f.outOfScope}},
		{"carol", "team", []int{f.outOfScope}},
	}
	for i, cl := range clients {
		if _, err := (&service.ClientService{}).Create(&service.InboundService{}, &service.ClientCreatePayload{
			Client:     model.Client{Email: cl.email, ID: "11111111-2222-3333-4444-55555555555" + strconv.Itoa(i), SubID: "sub-" + cl.email, Enable: true, Group: cl.group},
			InboundIds: cl.inbounds,
		}); err != nil {
			t.Fatalf("seed client %s: %v", cl.email, err)
		}
	}

	users := &panel.UserService{}
	owner, err := users.GetPrimaryOwner()
	if err != nil {
		t.Fatalf("owner: %v", err)
	}
	f.userIds[model.AdminRoleOwner] = owner.Id
	for _, role := range []string{model.AdminRoleAdmin, model.AdminRoleOperator, model.AdminRoleViewer} {
		in := panel.AdminInput{Username: role + "-user", Password: "pw", Role: role}
		if role == model.AdminRoleOperator {
			in.InboundIds, in.Groups = []int{f.inScope}, []string{"team"}
		}
		view, err := users.CreateAdmin(in)
		if err != nil {
			t.Fatalf("create %s: %v", role, err)
		}
		f.userIds[role] = view.Id
	}

	previous := global.GetWebServer()
	s := NewServer()
	s.cron = cron.New(cron.WithLocation(time.Local), cron.WithSeconds())
	global.SetWebServer(s)
	t.Cleanup(func() {
		s.cancel()
		global.SetWebServer(previous)
	})
	engine, err := s.initRouter()
	if err != nil {
		t.Fatalf("init router: %v", err)
	}
	engine.GET("/test-login/:id", func(c *gin.Context) {
		id, _ := strconv.Atoi(c.Param("id"))
		var u model.User
		if err := database.GetDB().First(&u, id).Error; err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		if err := session.SetLoginUser(c, &u); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		token, _ := session.EnsureCSRFToken(c)
		c.String(http.StatusOK, token)
	})
	f.server = httptest.NewServer(engine)
	t.Cleanup(f.server.Close)
	return f
}

type rbacClient struct {
	t    *testing.T
	http *http.Client
	base string
	csrf string
}

func (f *rbacFixture) login(t *testing.T, role string) *rbacClient {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	cl := &rbacClient{t: t, http: &http.Client{Jar: jar}, base: f.server.URL}
	resp, err := cl.http.Get(f.server.URL + "/test-login/" + strconv.Itoa(f.userIds[role]))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("login %s: %v %v", role, err, resp)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	cl.csrf = string(raw)
	return cl
}

// call returns the status and decoded JSON body of one API request.
func (cl *rbacClient) call(method, path, body string) (int, map[string]any) {
	cl.t.Helper()
	req, _ := http.NewRequest(method, cl.base+"/panel/api"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set(session.CSRFHeaderName, cl.csrf)
	resp, err := cl.http.Do(req)
	if err != nil {
		cl.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (cl *rbacClient) forbidden(method, path, body string) bool {
	cl.t.Helper()
	status, out := cl.call(method, path, body)
	return status == http.StatusForbidden && out["success"] == false && out["msg"] != ""
}

func TestAdminRolesAreEnforcedOnTheAPI(t *testing.T) {
	f := newRBACFixture(t)
	cases := []struct {
		role, method, path, body string
		denied                   bool
	}{
		{model.AdminRoleOwner, http.MethodGet, "/admins/list", "", false},
		{model.AdminRoleAdmin, http.MethodGet, "/admins/list", "", true},
		{model.AdminRoleAdmin, http.MethodPost, "/admins/create", `{"username":"x","password":"y","role":"owner"}`, true},
		{model.AdminRoleAdmin, http.MethodGet, "/setting/apiTokens", "", true},
		{model.AdminRoleAdmin, http.MethodGet, "/server/getDb", "", true},
		{model.AdminRoleAdmin, http.MethodPost, "/clients/del/bob", "", false},
		{model.AdminRoleViewer, http.MethodGet, "/clients/list", "", false},
		{model.AdminRoleViewer, http.MethodPost, "/clients/del/alice", "", true},
		{model.AdminRoleViewer, http.MethodPost, "/clients/onlines", "", false},
		{model.AdminRoleViewer, http.MethodGet, "/admins/me", "", false},
		{model.AdminRoleOperator, http.MethodGet, "/server/getConfigJson", "", true},
		{model.AdminRoleOperator, http.MethodPost, "/inbounds/add", `{}`, true},
		{model.AdminRoleOperator, http.MethodGet, "/inbounds/get/" + strconv.Itoa(f.outOfScope), "", true},
		{model.AdminRoleOperator, http.MethodGet, "/inbounds/get/" + strconv.Itoa(f.inScope), "", false},
		{model.AdminRoleOperator, http.MethodPost, "/clients/resetTraffic/bob", "", true},
		{model.AdminRoleOperator, http.MethodPost, "/clients/resetTraffic/alice", "", false},
		{model.AdminRoleOperator, http.MethodPost, "/clients/resetTraffic/carol", "", false},
		{model.AdminRoleOperator, http.MethodPost, "/clients/bulkDel", `{"emails":["alice","bob"]}`, true},
		{model.AdminRoleOperator, http.MethodPost, "/clients/alice/attach", `{"inboundIds":[` + strconv.Itoa(f.outOfScope) + `]}`, true},
		{model.AdminRoleOperator, http.MethodPost, "/clients/add", `{"client":{"email":"dave"},"inboundIds":[` + strconv.Itoa(f.outOfScope) + `]}`, true},
		{model.AdminRoleOperator, http.MethodPost, "/clients/add", `{"client":{"email":"dave","group":"other"},"inboundIds":[` + strconv.Itoa(f.inScope) + `]}`, true},
		{model.AdminRoleOperator, http.MethodPost, "/clients/add", `{"client":{"email":"dave","enable":true},"inboundIds":[` + strconv.Itoa(f.inScope) + `]}`, false},
		{model.AdminRoleOperator, http.MethodPost, "/clients/update/carol", `{"email":"carol","group":""}`, true},
		{model.AdminRoleOperator, http.MethodPost, "/clients/update/carol", `{"email":"carol","group":"team"}`, false},
		{model.AdminRoleOperator, http.MethodPost, "/clients/groups/rename", `{"oldName":"team","newName":"x"}`, true},
		{model.AdminRoleOperator, http.MethodPost, "/setting/updateUser", `{}`, false},
	}
	sessions := map[string]*rbacClient{}
	for _, tc := range cases {
		cl := sessions[tc.role]
		if cl == nil {
			cl = f.login(t, tc.role)
			sessions[tc.role] = cl
		}
		if got := cl.forbidden(tc.method, tc.path, tc.body); got != tc.denied {
			t.Errorf("%s %s %s: denied=%v, want %v", tc.role, tc.method, tc.path, got, tc.denied)
		}
	}
}

func emailsOf(t *testing.T, items any) []string {
	t.Helper()
	list, _ := items.([]any)
	out := []string{}
	for _, it := range list {
		m, _ := it.(map[string]any)
		if e, ok := m["email"].(string); ok {
			out = append(out, e)
		}
	}
	return out
}

// An operator's lists must hold only their own clients and inbounds, counts included.
func TestOperatorListsHoldOnlyTheirScope(t *testing.T) {
	f := newRBACFixture(t)
	op := f.login(t, model.AdminRoleOperator)

	_, out := op.call(http.MethodGet, "/clients/list", "")
	if got := strings.Join(emailsOf(t, out["obj"]), ","); got != "alice,carol" {
		t.Errorf("clients/list = %q, want alice,carol", got)
	}

	_, out = op.call(http.MethodGet, "/clients/list/paged?pageSize=50", "")
	page, _ := out["obj"].(map[string]any)
	if got := strings.Join(emailsOf(t, page["items"]), ","); got != "alice,carol" {
		t.Errorf("clients/list/paged items = %q, want alice,carol", got)
	}
	if page["total"] != float64(2) {
		t.Errorf("clients/list/paged total = %v, want 2", page["total"])
	}

	_, out = op.call(http.MethodGet, "/inbounds/list", "")
	list, _ := out["obj"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["tag"] != "in" {
		t.Errorf("inbounds/list = %v, want only the in-scope inbound", out["obj"])
	}

	_, out = op.call(http.MethodGet, "/clients/groups", "")
	groups, _ := out["obj"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["name"] != "team" {
		t.Errorf("clients/groups = %v, want only team", out["obj"])
	}

	owner := f.login(t, model.AdminRoleOwner)
	_, out = owner.call(http.MethodGet, "/inbounds/list", "")
	if list, _ := out["obj"].([]any); len(list) != 2 {
		t.Errorf("owner inbounds/list = %d rows, want both inbounds", len(list))
	}
}

// An admin may save ordinary settings but not the ones that decide who can log in.
func TestAdminCannotChangeOwnerOnlySettings(t *testing.T) {
	f := newRBACFixture(t)
	stored, err := (&service.SettingService{}).GetAllSetting()
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	body := func(mutate func(map[string]any)) string {
		raw, _ := json.Marshal(stored)
		m := map[string]any{}
		_ = json.Unmarshal(raw, &m)
		mutate(m)
		out, _ := json.Marshal(m)
		return string(out)
	}
	admin := f.login(t, model.AdminRoleAdmin)
	status, out := admin.call(http.MethodPost, "/setting/update", body(func(m map[string]any) { m["ldapHost"] = "evil.example" }))
	if status != http.StatusForbidden || out["success"] != false || out["msg"] == "" {
		t.Errorf("admin LDAP change = %d %v, want 403 securityOwnerOnly", status, out)
	}
	status, out = admin.call(http.MethodPost, "/setting/update", body(func(m map[string]any) { m["pageSize"] = 77 }))
	if status != http.StatusOK || out["success"] != true {
		t.Errorf("admin ordinary change = %d %v, want success", status, out)
	}
}
