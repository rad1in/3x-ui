package panel

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
)

func pointFeedAt(t *testing.T, status int, body string) *atomic.Int32 {
	t.Helper()
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, config.GetDBPath())
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	prevNow, prevFeed := sponsorNow, sponsorFeed
	sponsorNow = func() time.Time { return sponsorTestNow }
	sponsorFeed = srv.URL + "/sponsors.json"
	resetSponsorCache()
	t.Cleanup(func() {
		sponsorNow, sponsorFeed = prevNow, prevFeed
		resetSponsorCache()
	})
	return &hits
}

func readExample(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/sponsors.example.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The example the owner is told to paste into the sponsors repo must be accepted as written.
func TestDocumentedExampleFeedParsesAndIsLive(t *testing.T) {
	pointFeedAt(t, http.StatusOK, readExample(t))
	got, err := (&PanelService{}).GetSponsors()
	if err != nil {
		t.Fatal(err)
	}
	if got.Contact != "https://t.me/example" || len(got.Sponsors) != 1 {
		t.Fatalf("got %+v, want the contact and one live sponsor", got)
	}
	sp := got.Sponsors[0]
	if sp.ID != "acme-2026-10" || sp.Logo != "/sponsors/logo/acme.png" || sp.Title["en"] == "" ||
		sp.Text["fa"] == "" || strings.Join(sp.Slots, ",") != "dashboard,page,login" {
		t.Fatalf("sponsor = %+v", sp)
	}
}

// The docs and README show the example verbatim, so they can never drift from what parses.
func TestExampleFeedIsPublishedVerbatimInDocs(t *testing.T) {
	example := strings.TrimSpace(readExample(t))
	for _, path := range []string{"../../../../docs/content/docs/en/reference/sponsors.mdx", "../../../../README.md"} {
		doc, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(doc), example) {
			t.Errorf("%s does not contain testdata/sponsors.example.json verbatim", path)
		}
	}
}

func TestMissingFeedMeansNoSponsors(t *testing.T) {
	hits := pointFeedAt(t, http.StatusNotFound, "404: Not Found")
	for range 2 {
		got, err := (&PanelService{}).GetSponsors()
		if err != nil || got == nil || len(got.Sponsors) != 0 {
			t.Fatalf("GetSponsors = %+v, %v; want an empty list and no error", got, err)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("feed hits = %d, want 1 (the empty answer is cached)", n)
	}
}

func TestServerErrorFromFeedIsNotMistakenForEmpty(t *testing.T) {
	pointFeedAt(t, http.StatusBadGateway, "")
	if _, err := (&PanelService{}).GetSponsors(); err == nil {
		t.Fatal("a 502 must surface as an error so the last good list is kept")
	}
}

// A reseller's hosted panel reads the same remote feed as every other panel.
func TestHostedPanelReadsTheSameRemoteFeed(t *testing.T) {
	t.Setenv("XUI_HOSTED_DIR", t.TempDir())
	pointFeedAt(t, http.StatusOK, readExample(t))
	got, err := (&PanelService{}).GetSponsors()
	if err != nil || len(got.Sponsors) != 1 {
		t.Fatalf("hosted GetSponsors = %+v, %v; want the remote feed's sponsor", got, err)
	}
}
