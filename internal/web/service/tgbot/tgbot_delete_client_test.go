package tgbot

import (
	"errors"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/global"

	"github.com/mymmrac/telego"
	"gorm.io/gorm"
)

// Deleting from the bot takes two taps: the first only asks to confirm.
func TestDeleteClientNeedsConfirmation(t *testing.T) {
	mock, calls := staleButtonServer(t, map[string]any{
		"answerCallbackQuery":    map[string]any{"ok": true, "result": true},
		"editMessageReplyMarkup": map[string]any{"ok": true, "result": true},
		"editMessageText":        map[string]any{"ok": true, "result": true},
	})
	defer mock.Close()
	swapTestBot(t, mock.URL)
	tb := newStaleButtonTgbot(t)
	origStorage := hashStorage
	hashStorage = global.NewHashStorage(time.Minute)
	t.Cleanup(func() { hashStorage = origStorage })
	if err := database.GetDB().Create(&model.ClientRecord{Email: "doomed", SubID: "s-doomed", Enable: true}).Error; err != nil {
		t.Fatal(err)
	}
	tap := func(data string) {
		t.Helper()
		tb.answerCallback(&telego.CallbackQuery{
			ID: "q", From: telego.User{ID: 1}, Data: tb.encodeQuery(data),
			Message: &telego.Message{Chat: telego.Chat{ID: 1}, MessageID: 5},
		}, true)
	}
	exists := func() bool {
		t.Helper()
		err := database.GetDB().Where("email = ?", "doomed").First(&model.ClientRecord{}).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatal(err)
		}
		return err == nil
	}

	tap("delete_client doomed")
	if !exists() || calls("editMessageReplyMarkup") != 1 {
		t.Fatalf("first tap: exists = %v, confirm keyboards = %d; want the client kept and a confirm prompt",
			exists(), calls("editMessageReplyMarkup"))
	}
	tap("delete_client_c doomed")
	if exists() {
		t.Fatal("confirmed delete left the client in place")
	}
	if calls("answerCallbackQuery") != 1 || calls("editMessageText") != 1 {
		t.Fatalf("confirm: answers = %d, message edits = %d; want the tap answered and the card replaced",
			calls("answerCallbackQuery"), calls("editMessageText"))
	}
}
