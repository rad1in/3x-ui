package panel

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestDashboardLayoutIsPerAdminAndRoundTrips(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, config.GetDBPath())
	svc := &UserService{}
	a, err := svc.CreateAdmin(AdminInput{Username: "a", Password: "pw-12345", Role: model.AdminRoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.CreateAdmin(AdminInput{Username: "b", Password: "pw-12345", Role: model.AdminRoleAdmin})
	if err != nil {
		t.Fatal(err)
	}

	empty, err := svc.GetDashboardLayout(a.Id)
	if err != nil || len(empty.Widgets) != 0 {
		t.Fatalf("fresh admin layout = %+v, %v; want no widgets", empty, err)
	}

	want := DashboardLayout{Widgets: []DashboardWidget{
		{Id: "geo", Span: 6},
		{Id: "vitals", Hidden: true, Span: 12},
	}}
	if err := svc.SetDashboardLayout(a.Id, want); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetDashboardLayout(a.Id)
	if err != nil || !reflect.DeepEqual(*got, want) {
		t.Fatalf("GetDashboardLayout(a) = %+v, %v; want %+v", got, err, want)
	}
	other, err := svc.GetDashboardLayout(b.Id)
	if err != nil || len(other.Widgets) != 0 {
		t.Fatalf("admin b saw a's layout: %+v, %v", other, err)
	}
}

func TestDashboardLayoutRejectsMalformedInput(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, config.GetDBPath())
	svc := &UserService{}
	a, err := svc.CreateAdmin(AdminInput{Username: "a", Password: "pw-12345", Role: model.AdminRoleAdmin})
	if err != nil {
		t.Fatal(err)
	}
	tooMany := make([]DashboardWidget, maxDashboardWidgets+1)
	for i := range tooMany {
		tooMany[i] = DashboardWidget{Id: "w" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Span: 12}
	}
	cases := map[string]DashboardLayout{
		"empty id":        {Widgets: []DashboardWidget{{Id: "", Span: 12}}},
		"long id":         {Widgets: []DashboardWidget{{Id: strings.Repeat("x", dashboardWidgetIDMax+1), Span: 12}}},
		"span zero":       {Widgets: []DashboardWidget{{Id: "geo", Span: 0}}},
		"span over grid":  {Widgets: []DashboardWidget{{Id: "geo", Span: 13}}},
		"duplicate id":    {Widgets: []DashboardWidget{{Id: "geo", Span: 6}, {Id: "geo", Span: 6}}},
		"too many widget": {Widgets: tooMany},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if err := svc.SetDashboardLayout(a.Id, in); err == nil {
				t.Fatal("malformed layout was stored")
			}
		})
	}
	got, err := svc.GetDashboardLayout(a.Id)
	if err != nil || len(got.Widgets) != 0 {
		t.Fatalf("a rejected save changed the layout: %+v, %v", got, err)
	}
}
