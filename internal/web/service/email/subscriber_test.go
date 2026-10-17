package email

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/web/locale"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

type fixedLang struct{}

func (fixedLang) GetTgLang() (string, error) { return "en-US", nil }

func TestMain(m *testing.M) {
	if err := locale.InitLocalizer(os.DirFS("../.."), fixedLang{}); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

// Client emails are admin-typed text going into an HTML body.
func TestClientWarningMailEscapesTheEmail(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	sub := NewSubscriber(service.SettingService{}, nil)
	subject, body := sub.formatMessage(eventbus.Event{
		Type:   eventbus.EventClientQuotaWarning,
		Source: "<b>eve</b>",
		Data:   &eventbus.ClientEventData{Email: "<b>eve</b>", Up: 900 << 20, Total: 1 << 30, Percent: 87},
	})
	if !strings.Contains(subject, "Client <b>eve</b> has used 87% (900.00MB / 1.00GB) of their traffic") {
		t.Fatalf("subject = %q", subject)
	}
	if strings.Contains(body, "<b>eve</b>") || !strings.Contains(body, "&lt;b&gt;eve&lt;/b&gt;") {
		t.Fatalf("body does not escape the client email:\n%s", body)
	}
	if !strings.Contains(body, "87% (900.00MB / 1.00GB)") {
		t.Fatalf("body lacks the usage line:\n%s", body)
	}
}
