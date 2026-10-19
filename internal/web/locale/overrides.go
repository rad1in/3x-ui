package locale

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
)

// BotMessageKeys are the client-facing bot texts an admin may reword.
// Operator-facing alerts and anything with logic-dependent layout stay fixed.
var BotMessageKeys = []string{
	"tgbot.commands.start",
	"tgbot.commands.welcome",
	"tgbot.commands.help",
	"tgbot.commands.helpClientCommands",
	"tgbot.commands.unknown",
	"tgbot.messages.inviteBound",
	"tgbot.messages.inviteInvalid",
	"tgbot.messages.ownQuotaWarning",
	"tgbot.messages.ownExpiryWarning",
	"tgbot.answers.askToAddUserId",
}

const maxOverrideLen = 4000

// override is a pre-parsed replacement text for one bot message.
type override map[string]*template.Template

var (
	botLang      atomic.Value // string: language the bot localizer was built for
	botOverrides atomic.Pointer[map[string]override]

	rawMu           sync.RWMutex
	rawTranslations = map[string][]byte{} // language -> translation JSON as shipped
)

func rememberTranslation(file string, data []byte) {
	lang := strings.TrimSuffix(path.Base(file), ".json")
	rawMu.Lock()
	rawTranslations[lang] = data
	rawMu.Unlock()
}

func setBotLang(lang string) { botLang.Store(lang) }

func isBotMessageKey(key string) bool {
	for _, k := range BotMessageKeys {
		if k == key {
			return true
		}
	}
	return false
}

// SetBotOverrides installs the admin's replacement texts ({language: {key: text}}
// as JSON). Empty clears them; invalid input is rejected and leaves the old set.
func SetBotOverrides(raw string) error {
	out, err := parseOverrides(raw)
	if err != nil {
		return err
	}
	if out == nil {
		botOverrides.Store(nil)
		return nil
	}
	botOverrides.Store(&out)
	return nil
}

// ValidateBotOverrides checks raw the way SetBotOverrides would, without installing it.
func ValidateBotOverrides(raw string) error {
	_, err := parseOverrides(raw)
	return err
}

func parseOverrides(raw string) (map[string]override, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var parsed map[string]map[string]string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("bot messages are not valid JSON: %w", err)
	}
	out := map[string]override{}
	for lang, texts := range parsed {
		o := override{}
		for key, text := range texts {
			if !isBotMessageKey(key) {
				return nil, fmt.Errorf("%q cannot be customised", key)
			}
			if len(text) > maxOverrideLen {
				return nil, fmt.Errorf("%s (%s) is longer than %d characters", key, lang, maxOverrideLen)
			}
			if strings.TrimSpace(text) == "" {
				continue
			}
			tpl, err := template.New(key).Parse(text)
			if err != nil {
				return nil, fmt.Errorf("%s (%s): %w", key, lang, err)
			}
			o[key] = tpl
		}
		out[lang] = o
	}
	return out, nil
}

// botOverrideFor renders the admin's text for key in the bot's language;
// false means use the shipped message.
func botOverrideFor(key string, data map[string]any) (string, bool) {
	m := botOverrides.Load()
	lang, _ := botLang.Load().(string)
	if m == nil || lang == "" {
		return "", false
	}
	tpl := (*m)[lang][key]
	if tpl == nil {
		return "", false
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", false
	}
	return buf.String(), true
}

// BotMessageDefaults returns the shipped text of each customisable key in lang
// (en-US when lang has none), placeholders unrendered.
func BotMessageDefaults(lang string) map[string]string {
	out := make(map[string]string, len(BotMessageKeys))
	for _, key := range BotMessageKeys {
		out[key] = rawMessage(lang, key)
		if out[key] == "" {
			out[key] = rawMessage("en-US", key)
		}
	}
	return out
}

func rawMessage(lang, key string) string {
	rawMu.RLock()
	data := rawTranslations[lang]
	rawMu.RUnlock()
	if data == nil {
		return ""
	}
	var node any
	if err := json.Unmarshal(data, &node); err != nil {
		return ""
	}
	for part := range strings.SplitSeq(key, ".") {
		m, ok := node.(map[string]any)
		if !ok {
			return ""
		}
		node = m[part]
	}
	s, _ := node.(string)
	return s
}

var errNoLang = errors.New("language is required")

// ValidBotLang reports whether lang is a shipped translation.
func ValidBotLang(lang string) error {
	if lang == "" {
		return errNoLang
	}
	rawMu.RLock()
	defer rawMu.RUnlock()
	if rawTranslations[lang] == nil {
		return fmt.Errorf("unknown language %q", lang)
	}
	return nil
}
