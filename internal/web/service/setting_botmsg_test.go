package service

import "testing"

// A bad bot text must be refused at save time, not discovered when a client messages the bot.
func TestBotMessageOverridesValidatedOnSave(t *testing.T) {
	setupSettingTestDB(t)
	s := &SettingService{}
	view, err := s.GetAllSettingView()
	if err != nil {
		t.Fatal(err)
	}
	view.AllSetting.TgBotMessageOverrides = `{"en-US":{"tgbot.commands.start":"{{ .Firstname "}}`
	if err := s.UpdateAllSetting(&view.AllSetting, SecretClears{}); err == nil {
		t.Fatal("broken template accepted")
	}
	view.AllSetting.TgBotMessageOverrides = `{"en-US":{"tgbot.commands.start":"Hi {{ .Firstname }}"}}`
	if err := s.UpdateAllSetting(&view.AllSetting, SecretClears{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetTgBotMessageOverrides(); got != view.AllSetting.TgBotMessageOverrides {
		t.Fatalf("stored %q", got)
	}
}
