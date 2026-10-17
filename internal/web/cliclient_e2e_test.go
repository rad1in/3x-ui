package web

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/cliclient"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
)

// `x-ui client` drives the production router with a bearer token end to end.
func TestCLIClientCommandsAgainstThePanel(t *testing.T) {
	n := startContractNode(t)
	if err := database.GetDB().Create(&model.ApiToken{
		Name: "cli", Token: crypto.HashTokenSHA256("cli-token"), Enabled: true, Scope: "admin",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(masterInbound("cli-in", true)).Error; err != nil {
		t.Fatal(err)
	}
	var inbound model.Inbound
	if err := database.GetDB().Where("remark = ?", "cli-in").First(&inbound).Error; err != nil {
		t.Fatal(err)
	}
	api := &cliclient.API{Base: n.srv.URL, Token: "cli-token", Client: n.srv.Client()}
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local)
	run := func(args ...string) (string, error) {
		t.Helper()
		var out bytes.Buffer
		err := cliclient.Run(api, args, &out, now)
		return out.String(), err
	}
	id := strconv.Itoa(inbound.Id)

	if out, err := run("add", "carol", "-inbound", id, "-gb", "2", "-days", "10", "-comment", "vip"); err != nil || out != "added carol\n" {
		t.Fatalf("add: %q, %v", out, err)
	}
	out, err := run("show", "carol")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"used      0.00B of 2.00GB", "expires   2026-10-11 12:00", "inbounds  " + id, "comment   vip"} {
		if !strings.Contains(out, want) {
			t.Fatalf("show lacks %q:\n%s", want, out)
		}
	}
	if out, err := run("disable", "carol"); err != nil || out != "disabled 1 client(s)\n" {
		t.Fatalf("disable: %q, %v", out, err)
	}
	out, err = run("list", "-inbound", id)
	if err != nil || !strings.Contains(out, "carol  false") {
		t.Fatalf("list after disable: %q, %v", out, err)
	}
	if out, _ := run("list", "-inbound", strconv.Itoa(inbound.Id+1)); strings.Contains(out, "carol") {
		t.Fatalf("-inbound filter listed a client of another inbound:\n%s", out)
	}
	if out, err := run("add", "carol", "-inbound", id); err == nil {
		t.Fatalf("adding a duplicate email succeeded: %q", out)
	}
	out, err = run("del", "carol", "ghost")
	if err == nil || !strings.Contains(out, "deleted carol\n") || !strings.Contains(out, "ghost: ") {
		t.Fatalf("del: %q, %v; want carol deleted and ghost reported", out, err)
	}
	if out, _ := run("list"); strings.Contains(out, "carol") {
		t.Fatalf("carol still listed after delete:\n%s", out)
	}
	if _, err := (&cliclient.API{Base: n.srv.URL, Token: "wrong", Client: n.srv.Client()}).List(); err == nil {
		t.Fatal("a wrong token was accepted")
	}
}
