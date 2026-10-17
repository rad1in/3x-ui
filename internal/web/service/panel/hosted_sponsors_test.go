package panel

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/hosted"
)

// A hosted panel shows the owner's sponsors, never ones its own admin
// stored, and keeps ending a placement on time between publishes.
func TestHostedPanelShowsOnlyTheOwnersSponsors(t *testing.T) {
	svc := setupLocalSponsors(t)
	if err := svc.SetSponsorConfig(SponsorConfig{Contact: "https://t.me/owner"}); err != nil {
		t.Fatal(err)
	}
	owner := activeRecord("Owner")
	owner.Logo = pngDataURL
	owner.UntilMs = sponsorTestNow.Add(time.Hour).UnixMilli()
	mustSave(t, svc, owner)

	panel := &model.HostedPanel{Name: "p", Username: "u", WebPort: 20530, SubPort: 20531, WebBasePath: "/p/", PortMin: 30000, PortMax: 30099}
	if err := database.GetDB().Create(panel).Error; err != nil {
		t.Fatal(err)
	}
	dir := hostedPanelDir(panel.Id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	PublishHostedSponsors()

	// From here on this process plays the hosted panel, whose own records must not show.
	t.Setenv(hosted.EnvDir, dir)
	if err := database.GetDB().Where("1 = 1").Delete(&model.SponsorRecord{}).Error; err != nil {
		t.Fatal(err)
	}
	mustSave(t, svc, activeRecord("Tenant"))

	got, err := svc.GetSponsors()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sponsors) != 1 || got.Sponsors[0].Name != "Owner" || got.Contact != "https://t.me/owner" {
		t.Fatalf("hosted sponsors = %+v, want only the owner's", got)
	}
	logo := got.Sponsors[0].Logo
	data, ct, err := svc.GetSponsorLogo(logo[len(sponsorLogoPath):])
	if err != nil || ct != "image/png" || string(data) != string(pngMagic) {
		t.Fatalf("owner's logo in the hosted panel = %q %q %v", data, ct, err)
	}

	sponsorNow = func() time.Time { return sponsorTestNow.Add(2 * time.Hour) }
	if got, _ := svc.GetSponsors(); len(got.Sponsors) != 0 {
		t.Fatalf("an ended placement still shows until the next publish: %+v", got.Sponsors)
	}
	if _, _, err := svc.GetSponsorLogo(logo[len(sponsorLogoPath):]); !errors.Is(err, ErrSponsorLogoUnknown) {
		t.Fatalf("logo of an ended placement: err = %v", err)
	}
}

func TestOwnerSponsorEditsReachHostedPanelsAtOnce(t *testing.T) {
	svc := setupLocalSponsors(t)
	panel := &model.HostedPanel{Name: "p", Username: "u", WebPort: 20530, SubPort: 20531, WebBasePath: "/p/", PortMin: 30000, PortMax: 30099}
	if err := database.GetDB().Create(panel).Error; err != nil {
		t.Fatal(err)
	}
	dir := hostedPanelDir(panel.Id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	saved := mustSave(t, svc, activeRecord("Fresh"))
	if snap := readHostedSponsors(dir); len(snap.List.Sponsors) != 1 || snap.List.Sponsors[0].Name != "Fresh" {
		t.Fatalf("after save the hosted panel has %+v", snap.List.Sponsors)
	}
	if err := svc.DeleteSponsorRecord(saved.Id); err != nil {
		t.Fatal(err)
	}
	if snap := readHostedSponsors(dir); len(snap.List.Sponsors) != 0 {
		t.Fatalf("a deleted sponsor still shows in the hosted panel: %+v", snap.List.Sponsors)
	}
}
