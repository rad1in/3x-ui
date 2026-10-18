package locale

import (
	"os"
	"strings"
	"testing"
)

type tgLang string

func (l tgLang) GetTgLang() (string, error) { return string(l), nil }

func initFor(t *testing.T, lang string) {
	t.Helper()
	if err := InitLocalizer(os.DirFS(".."), tgLang(lang)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = SetBotOverrides("") })
}

const startKey = "tgbot.commands.start"

func TestBotOverrideReplacesDefaultAndRendersParams(t *testing.T) {
	initFor(t, "en-US")
	if err := SetBotOverrides(`{"en-US":{"tgbot.commands.start":"Hi {{ .Firstname }}, welcome to Acme VPN"}}`); err != nil {
		t.Fatal(err)
	}
	if got := I18n(Bot, startKey, "Firstname==Sara"); got != "Hi Sara, welcome to Acme VPN" {
		t.Fatalf("got %q", got)
	}
}

// The bot speaks one language; an override written for another must not leak into it.
func TestBotOverrideIgnoresOtherLanguagesAndKeys(t *testing.T) {
	initFor(t, "en-US")
	if err := SetBotOverrides(`{"ru-RU":{"tgbot.commands.start":"Привет"}}`); err != nil {
		t.Fatal(err)
	}
	if got := I18n(Bot, startKey, "Firstname==Sara"); !strings.Contains(got, "Hello") {
		t.Fatalf("default lost: %q", got)
	}
	initFor(t, "ru-RU")
	if err := SetBotOverrides(`{"ru-RU":{"tgbot.commands.start":"Привет"}}`); err != nil {
		t.Fatal(err)
	}
	if got := I18n(Bot, startKey, "Firstname==Sara"); got != "Привет" {
		t.Fatalf("ru override not used: %q", got)
	}
	if got := I18n(Bot, "tgbot.commands.status"); got == "" || got == "Привет" {
		t.Fatalf("unrelated key affected: %q", got)
	}
}

func TestSetBotOverridesRejectsBadInput(t *testing.T) {
	initFor(t, "en-US")
	for name, raw := range map[string]string{
		"key outside the allowlist": `{"en-US":{"tgbot.messages.cpuThreshold":"x"}}`,
		"broken template":           `{"en-US":{"tgbot.commands.start":"{{ .Firstname "}}`,
		"not json":                  `nope`,
		"too long":                  `{"en-US":{"tgbot.commands.start":"` + strings.Repeat("a", 4001) + `"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := SetBotOverrides(raw); err == nil {
				t.Fatalf("%s accepted", name)
			}
		})
	}
}

// The editor shows the shipped text with its {{ .Var }} placeholders unrendered.
func TestBotMessageDefaultsKeepPlaceholders(t *testing.T) {
	initFor(t, "en-US")
	d := BotMessageDefaults("en-US")
	if len(d) != len(BotMessageKeys) {
		t.Fatalf("got %d defaults for %d keys", len(d), len(BotMessageKeys))
	}
	if !strings.Contains(d[startKey], "{{ .Firstname }}") {
		t.Fatalf("start default = %q", d[startKey])
	}
}
