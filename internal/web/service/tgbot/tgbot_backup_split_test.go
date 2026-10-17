package tgbot

import (
	"crypto/rand"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"

	"github.com/mymmrac/telego"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// Telegram refuses uploads over 50 MB; a bigger backup used to vanish with a
// log line. It now arrives as gzip parts under the cap, with the rejoin command.
func TestBackupOverTheUploadCapArrivesInParts(t *testing.T) {
	bundle := i18n.NewBundle(language.MustParse("en-US"))
	_ = bundle.AddMessages(language.MustParse("en-US"),
		&i18n.Message{ID: "tgbot.messages.backupSplit", Other: "rejoin: cat {{ .Name }}.gz.part* | gunzip > {{ .Name }}"})
	origLoc := locale.LocalizerBot
	t.Cleanup(func() { locale.LocalizerBot = origLoc })
	locale.LocalizerBot = i18n.NewLocalizer(bundle, "en-US")
	origLimit := telegramUploadLimit
	telegramUploadLimit = 2048
	t.Cleanup(func() { telegramUploadLimit = origLimit })

	url, calls, bodies := newBroadcastMock(t)
	origBot := bot
	t.Cleanup(func() { bot = origBot })
	var err error
	if bot, err = telego.NewBot(testBotToken, telego.WithAPIServer(url)); err != nil {
		t.Fatal(err)
	}
	setBroadcastRunning(t, true)

	db := make([]byte, 5000)
	_, _ = rand.Read(db)
	(&Tgbot{}).sendBackupData(1, db, "x.db")

	if n := calls("sendDocument"); n != 3 {
		t.Fatalf("sendDocument calls = %d, want the 3 parts", n)
	}
	var noted bool
	for _, b := range bodies("sendMessage") {
		if text, _ := b["text"].(string); strings.Contains(text, "cat x.db.gz.part* | gunzip > x.db") {
			noted = true
		}
	}
	if !noted {
		t.Fatalf("no message told how to rejoin the parts: %v", bodies("sendMessage"))
	}
}
