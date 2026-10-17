package panel

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

var pngDataURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngMagic)

func setupLocalSponsors(t *testing.T) *PanelService {
	t.Helper()
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, config.GetDBPath())
	prevNow := sponsorNow
	sponsorNow = func() time.Time { return sponsorTestNow }
	resetSponsorCache()
	t.Cleanup(func() {
		sponsorNow = prevNow
		resetSponsorCache()
	})
	return &PanelService{}
}

func mustSave(t *testing.T, svc *PanelService, r model.SponsorRecord) model.SponsorRecord {
	t.Helper()
	saved, err := svc.SaveSponsorRecord(r)
	if err != nil {
		t.Fatalf("save %q: %v", r.Name, err)
	}
	return saved
}

func activeRecord(name string) model.SponsorRecord {
	return model.SponsorRecord{
		Name:   name,
		Enable: true,
		Slots:  []string{"dashboard", "login"},
		Title:  name + " title",
		Text:   name + " text",
		Link:   "https://" + strings.ToLower(name) + ".example/",
	}
}

func TestLocalSponsorsServeOnlyLiveRecords(t *testing.T) {
	svc := setupLocalSponsors(t)
	live := activeRecord("Live")
	live.Logo = pngDataURL
	live = mustSave(t, svc, live)

	disabled := activeRecord("Off")
	disabled.Enable = false
	mustSave(t, svc, disabled)

	expired := activeRecord("Old")
	expired.UntilMs = sponsorTestNow.Add(-time.Hour).UnixMilli()
	mustSave(t, svc, expired)

	future := activeRecord("Soon")
	future.FromMs = sponsorTestNow.Add(time.Hour).UnixMilli()
	mustSave(t, svc, future)

	got, err := svc.GetSponsors()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sponsors) != 1 {
		t.Fatalf("sponsors = %+v, want only the live one", got.Sponsors)
	}
	sp := got.Sponsors[0]
	if sp.ID != "local-1" || sp.Name != "Live" || sp.Link != "https://live.example/" ||
		sp.Title[anyLanguage] != "Live title" || sp.Text[anyLanguage] != "Live text" ||
		sp.Logo != sponsorLogoPath+"local-1.png" || strings.Join(sp.Slots, ",") != "dashboard,login" {
		t.Errorf("sponsor = %+v", sp)
	}
	if live.Id != 1 {
		t.Fatalf("expected first record id 1, got %d", live.Id)
	}

	data, ct, err := svc.GetSponsorLogo("local-1.png")
	if err != nil || ct != "image/png" || string(data) != string(pngMagic) {
		t.Errorf("logo = %q %q %v", data, ct, err)
	}
	if _, _, err := svc.GetSponsorLogo("local-2.png"); !errors.Is(err, ErrSponsorLogoUnknown) {
		t.Errorf("logo of a disabled sponsor must not be served, got %v", err)
	}
}

func TestSaveSponsorRecordRejectsUnsafeInput(t *testing.T) {
	svc := setupLocalSponsors(t)
	cases := map[string]func(r *model.SponsorRecord){
		"empty name":        func(r *model.SponsorRecord) { r.Name = " " },
		"plain http link":   func(r *model.SponsorRecord) { r.Link = "http://acme.example/" },
		"javascript link":   func(r *model.SponsorRecord) { r.Link = "javascript:alert(1)" },
		"no slots":          func(r *model.SponsorRecord) { r.Slots = nil },
		"unknown slot":      func(r *model.SponsorRecord) { r.Slots = []string{"footer"} },
		"svg logo":          func(r *model.SponsorRecord) { r.Logo = "data:image/svg+xml;base64,PHN2Zy8+" },
		"until before from": func(r *model.SponsorRecord) { r.FromMs, r.UntilMs = 2000, 1000 },
		"text too long":     func(r *model.SponsorRecord) { r.Text = strings.Repeat("x", 301) },
	}
	for name, mutate := range cases {
		r := activeRecord("Acme")
		mutate(&r)
		if _, err := svc.SaveSponsorRecord(r); err == nil {
			t.Errorf("%s: want error, got nil", name)
		}
	}
}

func TestSponsorConfigOnlyAllowsHTTPSOrLoopbackFeeds(t *testing.T) {
	svc := setupLocalSponsors(t)
	ok := []string{"", "https://master.example/p/sponsors/feed/sponsors.json", "http://127.0.0.1:2053/sponsors/feed/sponsors.json", "http://localhost:9/x/sponsors/feed/sponsors.json", "http://[::1]:9/sponsors.json"}
	for _, u := range ok {
		if err := svc.SetSponsorConfig(SponsorConfig{FeedURL: u}); err != nil {
			t.Errorf("feed %q: unexpected error %v", u, err)
		}
	}
	bad := []string{"http://master.example/sponsors/feed/sponsors.json", "ftp://127.0.0.1/sponsors.json", "https://", "http://127.0.0.1.evil.example/sponsors.json"}
	for _, u := range bad {
		if err := svc.SetSponsorConfig(SponsorConfig{FeedURL: u}); err == nil {
			t.Errorf("feed %q: want error", u)
		}
	}
	if err := svc.SetSponsorConfig(SponsorConfig{Contact: "javascript:alert(1)"}); err == nil {
		t.Error("contact must be https")
	}
}

// A child panel pointed at its master's feed shows the master's sponsors,
// logos included, without the master's admin API.
func TestSponsorFeedRoundTripsToAnotherPanel(t *testing.T) {
	master := setupLocalSponsors(t)
	rec := activeRecord("Acme")
	rec.Logo = pngDataURL
	mustSave(t, master, rec)
	feed, err := master.SponsorFeed()
	if err != nil {
		t.Fatal(err)
	}
	feedJSON, err := json.Marshal(feed)
	if err != nil {
		t.Fatal(err)
	}
	logo, _, err := master.GetSponsorFeedLogo("local-1.png")
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/base/sponsors/feed/sponsors.json":
			_, _ = w.Write(feedJSON)
		case "/base/sponsors/feed/logos/local-1.png":
			_, _ = w.Write(logo)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	child := setupLocalSponsors(t)
	mustSave(t, child, activeRecord("ChildOwn"))
	if err := child.SetSponsorConfig(SponsorConfig{FeedURL: srv.URL + "/base/sponsors/feed/sponsors.json"}); err != nil {
		t.Fatal(err)
	}
	got, err := child.GetSponsors()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sponsors) != 1 || got.Sponsors[0].Name != "Acme" || got.Sponsors[0].Title[anyLanguage] != "Acme title" {
		t.Fatalf("child sponsors = %+v, want the master's Acme only", got.Sponsors)
	}
	data, ct, err := child.GetSponsorLogo("local-1.png")
	if err != nil || ct != "image/png" || string(data) != string(pngMagic) {
		t.Fatalf("child logo = %q %q %v", data, ct, err)
	}
}
