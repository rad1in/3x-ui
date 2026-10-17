package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func hostedPanelAt(web, sub, lo, hi int) *model.HostedPanel {
	return &model.HostedPanel{Name: "p", Username: "u", WebPort: web, SubPort: sub, WebBasePath: "/p/", PortMin: lo, PortMax: hi}
}

func TestCheckHostedPanelPortsRefusesEveryCollision(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	if err := (&SettingService{}).SetPort(2053); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (&InboundService{}).AddInbound(hostedVless(t, 443)); err != nil {
		t.Fatal(err)
	}
	sibling := hostedPanelAt(21000, 21001, 31000, 31099)
	if err := db.Create(sibling).Error; err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		panel *model.HostedPanel
		want  error
	}{
		{"free address and range", hostedPanelAt(22000, 22001, 32000, 32099), nil},
		{"no inbound range", hostedPanelAt(22000, 22001, 0, 0), ErrHostedPanelPortRange},
		{"inverted range", hostedPanelAt(22000, 22001, 32099, 32000), ErrHostedPanelPortRange},
		{"web and sub on one port", hostedPanelAt(22000, 22000, 32000, 32099), ErrHostedPanelPortRange},
		{"own web port inside own range", hostedPanelAt(32010, 22001, 32000, 32099), ErrHostedPanelPortRange},
		{"master web port", hostedPanelAt(2053, 22001, 32000, 32099), ErrHostedPanelPortTaken},
		{"range covers the master's sub port", hostedPanelAt(22000, 22001, 2090, 2100), ErrHostedPanelPortTaken},
		{"range covers the Xray API port", hostedPanelAt(22000, 22001, 62000, 63000), ErrHostedPanelPortTaken},
		{"range covers a master inbound", hostedPanelAt(22000, 22001, 400, 500), ErrHostedPanelPortTaken},
		{"sibling web port", hostedPanelAt(21000, 22001, 32000, 32099), ErrHostedPanelPortTaken},
		{"overlaps sibling range", hostedPanelAt(22000, 22001, 31050, 31150), ErrHostedPanelPortTaken},
		{"own address inside sibling range", hostedPanelAt(31010, 22001, 32000, 32099), ErrHostedPanelPortTaken},
		{"range covers sibling sub port", hostedPanelAt(22000, 22002, 21000, 21010), ErrHostedPanelPortTaken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckHostedPanelPorts(db, tt.panel)
			if !errors.Is(err, tt.want) || (tt.want == nil) != (err == nil) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("a panel does not collide with itself on edit", func(t *testing.T) {
		if err := CheckHostedPanelPorts(db, sibling); err != nil {
			t.Fatalf("re-checking the saved sibling: %v", err)
		}
	})
}

func TestMasterInboundCannotTakeAHostedPanelsPorts(t *testing.T) {
	setupConflictDB(t)
	if err := database.GetDB().Create(hostedPanelAt(21000, 21001, 31000, 31099)).Error; err != nil {
		t.Fatal(err)
	}
	svc := &InboundService{}
	for _, port := range []int{31050, 21001} {
		_, _, err := svc.AddInbound(hostedVless(t, port))
		if err == nil || !strings.Contains(err.Error(), "hosted panel 'p'") {
			t.Fatalf("master inbound on port %d: err = %v, want a hosted-panel conflict", port, err)
		}
	}
}

func TestMasterSettingsCannotMoveOntoAHostedPanelsPort(t *testing.T) {
	setupConflictDB(t)
	if err := database.GetDB().Create(hostedPanelAt(21000, 21001, 31000, 31099)).Error; err != nil {
		t.Fatal(err)
	}
	s := &SettingService{}
	all, err := s.GetAllSetting()
	if err != nil {
		t.Fatal(err)
	}
	all.SubPort = 31000
	if err := s.UpdateAllSetting(all, SecretClears{}); !errors.Is(err, ErrHostedPanelPortTaken) {
		t.Fatalf("moving the master sub port into a hosted range: err = %v", err)
	}
}
