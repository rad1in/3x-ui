package service

import "testing"

// A blank submitted push credential means "unchanged" (the browser never sees it); only the clear flag empties it.
func TestPushSecretsPreservedUntilExplicitlyCleared(t *testing.T) {
	setupSettingTestDB(t)
	s := &SettingService{}
	for k, v := range map[string]string{"ntfyToken": "tk_n", "gotifyToken": "AppG", "slackWebhookUrl": "https://hooks.slack.com/services/T/B/x"} {
		if err := s.saveSetting(k, v); err != nil {
			t.Fatal(err)
		}
	}
	view, err := s.GetAllSettingView()
	if err != nil {
		t.Fatal(err)
	}
	if !view.HasNtfyToken || !view.HasGotifyToken || !view.HasSlackWebhookUrl || view.SlackWebhookUrl != "" {
		t.Fatalf("view must flag but not expose the secrets: %#v", view)
	}
	if err := s.UpdateAllSetting(&view.AllSetting, SecretClears{}); err != nil {
		t.Fatal(err)
	}
	if c := s.PushConfig(); c.NtfyToken != "tk_n" || c.GotifyToken != "AppG" || c.SlackWebhookUrl != "https://hooks.slack.com/services/T/B/x" {
		t.Fatalf("secrets not preserved: %#v", c)
	}

	view, _ = s.GetAllSettingView()
	if err := s.UpdateAllSetting(&view.AllSetting, SecretClears{GotifyToken: true}); err != nil {
		t.Fatal(err)
	}
	if c := s.PushConfig(); c.GotifyToken != "" || c.NtfyToken != "tk_n" {
		t.Fatalf("clear flag must empty only its own secret: %#v", c)
	}
}

func TestPushURLsMustBeHTTP(t *testing.T) {
	setupSettingTestDB(t)
	s := &SettingService{}
	view, err := s.GetAllSettingView()
	if err != nil {
		t.Fatal(err)
	}
	view.AllSetting.NtfyServer = "file:///etc/passwd"
	if err := s.UpdateAllSetting(&view.AllSetting, SecretClears{}); err == nil {
		t.Fatal("non-http ntfy server accepted")
	}
}
