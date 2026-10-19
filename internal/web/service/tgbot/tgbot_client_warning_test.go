package tgbot

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"

	"github.com/mymmrac/telego"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

func warningLocalizer(t *testing.T) {
	t.Helper()
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	_ = bundle.AddMessages(language.MustParse("en-US"),
		&i18n.Message{ID: "tgbot.messages.usagePercent", Other: "{{ .Percent }}% ({{ .Used }} / {{ .Total }})"},
		&i18n.Message{ID: "tgbot.messages.expiresOn", Other: "{{ .Date }} ({{ .Days }} days left)"},
		&i18n.Message{ID: "tgbot.messages.eventQuotaWarning", Other: "admin: {{ .Email }} used {{ .Usage }}"},
		&i18n.Message{ID: "tgbot.messages.ownQuotaWarning", Other: "you used {{ .Usage }}"},
	)
	orig := locale.LocalizerBot
	t.Cleanup(func() { locale.LocalizerBot = orig })
	locale.LocalizerBot = i18n.NewLocalizer(bundle, "en-US")
}

// A quota warning reaches the admins and the client's own linked chat.
func TestClientWarningReachesAdminsAndTheClient(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "eternal.db"))
	db := database.GetDB()
	if err := db.Create(&model.Setting{Key: "tgEnabledEvents", Value: "client.quota_warning"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, c := range []model.ClientRecord{{Email: "<i>owned", SubID: "s1", TgID: 555}, {Email: "unlinked", SubID: "s2"}} {
		if err := db.Create(&c).Error; err != nil {
			t.Fatal(err)
		}
	}
	warningLocalizer(t)
	url, _, bodies := newBroadcastMock(t)
	origBot := bot
	t.Cleanup(func() { bot = origBot })
	var err error
	if bot, err = telego.NewBot(testBotToken, telego.WithAPIServer(url)); err != nil {
		t.Fatal(err)
	}
	setBroadcastAdmins(t, []int64{1})
	setBroadcastRunning(t, true)

	tb := &Tgbot{}
	for _, email := range []string{"<i>owned", "unlinked"} {
		tb.HandleEvent(eventbus.Event{Type: eventbus.EventClientQuotaWarning, Source: email,
			Data: &eventbus.ClientEventData{Email: email, Up: 850, Total: 1000, Percent: 85}})
	}

	got := map[string][]string{}
	for _, b := range bodies("sendMessage") {
		chat, _ := b["chat_id"].(float64)
		text, _ := b["text"].(string)
		key := "admin"
		if chat == 555 {
			key = "client"
		} else if chat != 1 {
			t.Fatalf("message went to unexpected chat %v: %q", chat, text)
		}
		got[key] = append(got[key], text)
	}
	if len(got["admin"]) != 2 || !strings.Contains(got["admin"][0], "admin: &lt;i&gt;owned used 85% (850.00B / 1000.00B)") {
		t.Fatalf("admin messages = %q", got["admin"])
	}
	if len(got["client"]) != 1 || got["client"][0] != "you used 85% (850.00B / 1000.00B)" {
		t.Fatalf("client messages = %q; only the linked client hears about its own quota", got["client"])
	}
}
