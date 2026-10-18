package controller

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

var regexpDigits = regexp.MustCompile(`\d{6}`)

type sentCode struct {
	chat int64
	text string
}

// telegramHarness enables the Telegram step for the seeded admin and records
// what the panel would have sent to its chat.
func telegramHarness(t *testing.T, running bool) (*loginHarness, *[]sentCode) {
	t.Helper()
	h := newLoginHarness(t)
	if err := (&service.SecurityConfigService{}).Save(&service.SecurityConfig{TgLoginEnable: true}); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Model(&model.User{}).Where("1 = 1").Update("tg_chat_id", 4242).Error; err != nil {
		t.Fatal(err)
	}
	var sent []sentCode
	h.ctl.tgLogin = telegramLogin{
		running: func() bool { return running },
		send:    func(chat int64, text string) { sent = append(sent, sentCode{chat, text}) },
	}
	return h, &sent
}

type loginReply struct {
	Success bool
	Msg     string
	Obj     map[string]any
}

func (h *loginHarness) loginReply(form url.Values) loginReply {
	h.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-CSRF-Token", h.csrf)
	resp, err := h.client.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out loginReply
	if err := json.Unmarshal(body, &out); err != nil {
		h.t.Fatalf("reply %q: %v", body, err)
	}
	return out
}

func TestPasswordAloneIsNotEnoughWhenTelegramLoginIsOn(t *testing.T) {
	h, sent := telegramHarness(t, true)

	first := h.loginReply(credentials(nil))
	if first.Success || first.Obj["tgCodeRequired"] != true {
		t.Fatalf("first step = %+v, want a code to be asked for", first)
	}
	if len(*sent) != 1 || (*sent)[0].chat != 4242 {
		t.Fatalf("sent = %+v, want one message to chat 4242", *sent)
	}
	digits := regexpDigits.FindString((*sent)[0].text)
	if digits == "" {
		t.Fatalf("message %q carries no code", (*sent)[0].text)
	}
	wrong := "000000"
	if digits == wrong {
		wrong = "000001"
	}
	if h.login(credentials(url.Values{"tgCode": {wrong}})) {
		t.Fatal("a wrong code logged in")
	}
	if !h.login(credentials(url.Values{"tgCode": {digits}})) {
		t.Fatal("one wrong guess must not burn the code")
	}
}

func TestTheRightCodeLogsIn(t *testing.T) {
	h, sent := telegramHarness(t, true)
	h.loginReply(credentials(nil))
	digits := regexpDigits.FindString((*sent)[0].text)
	if !h.login(credentials(url.Values{"tgCode": {digits}})) {
		t.Fatal("the right code was refused")
	}
}

func TestAWrongPasswordNeverSendsACode(t *testing.T) {
	h, sent := telegramHarness(t, true)
	if h.login(credentials(url.Values{"password": {"nope"}})) {
		t.Fatal("a wrong password logged in")
	}
	if len(*sent) != 0 {
		t.Fatalf("%d codes sent for a wrong password, anyone could spam the admin", len(*sent))
	}
}

func TestWithoutARunningBotTheLoginIsNotBlocked(t *testing.T) {
	h, sent := telegramHarness(t, false)
	if !h.login(credentials(nil)) {
		t.Fatal("login was blocked although no code can be delivered, locking the admin out")
	}
	if len(*sent) != 0 {
		t.Fatal("a code was sent through a bot that is not running")
	}
}

func TestAdminsWithoutALinkedChatLogInNormally(t *testing.T) {
	h, _ := telegramHarness(t, true)
	if err := database.GetDB().Model(&model.User{}).Where("1 = 1").Update("tg_chat_id", 0).Error; err != nil {
		t.Fatal(err)
	}
	if !h.login(credentials(nil)) {
		t.Fatal("an admin with no linked chat was asked for a code it can never receive")
	}
}
