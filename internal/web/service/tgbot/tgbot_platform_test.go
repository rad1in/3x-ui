package tgbot

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/mymmrac/telego"
)

func TestIsBaleServer(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://tapi.bale.ai":        true,
		"https://TAPI.bale.ai/":       true,
		"https://tapi.bale.ai:443/x":  true,
		"https://api.telegram.org":    false,
		"https://tapi.bale.ai.evil.c": false,
		"":                            false,
	} {
		if got := isBaleServer(raw); got != want {
			t.Errorf("isBaleServer(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestHTMLToBale(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"bold and code":    {"<b>Server</b> <code>1.2.3.4</code>", "*Server* `1.2.3.4`"},
		"entities":         {"a &lt;b&gt; &amp; c", "a <b> & c"},
		"pre with code":    {"<pre><code>line1\nline2</code></pre>", "\n```\nline1\nline2\n```\n"},
		"link with label":  {`<a href="https://x.io/s">Open</a>`, "Open (https://x.io/s)"},
		"link is its text": {`<a href="https://x.io/s">https://x.io/s</a>`, "https://x.io/s"},
		"italic dropped":   {"<i>note</i> <u>u</u>", "note u"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := htmlToBale(tc.in); got != tc.want {
				t.Errorf("htmlToBale(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Against a server speaking Bale's shape (/bot<token>/sendMessage, no HTML parse mode),
// a status message must go out as plain Bale markdown, not literal tags.
func TestSendMsgToTgbotOnBaleSendsMarkdownWithoutParseMode(t *testing.T) {
	var mu sync.Mutex
	var gotPath string
	var got map[string]any
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotPath = r.URL.Path
		_ = json.Unmarshal(raw, &got)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"date":1,"chat":{"id":42,"type":"private"}}}`))
	}))
	defer mock.Close()

	origBot, origRunning := bot, isRunning
	t.Cleanup(func() { bot, isRunning = origBot, origRunning; baleMode.Store(false) })
	var err error
	bot, err = telego.NewBot(testBotToken, telego.WithAPIServer(mock.URL))
	if err != nil {
		t.Fatal(err)
	}
	isRunning = true
	baleMode.Store(true)

	(&Tgbot{}).SendMsgToTgbot(42, "<b>CPU</b> 12% &amp; <code>ok</code>")

	mu.Lock()
	defer mu.Unlock()
	if gotPath != "/bot"+testBotToken+"/sendMessage" {
		t.Fatalf("path = %q", gotPath)
	}
	if _, has := got["parse_mode"]; has || got["text"] != "*CPU* 12% & `ok`" {
		t.Fatalf("body = %v", got)
	}
}
