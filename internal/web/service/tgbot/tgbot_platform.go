package tgbot

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
)

// baleHost is Bale's Bot API server; its /bot<token>/<method> URLs mirror Telegram's,
// so the bot only needs the API-server setting pointed at it.
const baleHost = "tapi.bale.ai"

// baleMode is set by NewBot from the configured API server.
var baleMode atomic.Bool

func isBaleServer(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && strings.EqualFold(u.Hostname(), baleHost)
}

// platformText returns the text and parse mode for the connected messenger.
// Bale has no HTML parse mode, so tags would show up literally.
func platformText(msg string) (text, parseMode string) {
	if !baleMode.Load() {
		return msg, "HTML"
	}
	return htmlToBale(msg), ""
}

var (
	preCodeRe = regexp.MustCompile(`(?is)<pre[^>]*>\s*<code[^>]*>(.*?)</code>\s*</pre>`)
	preRe     = regexp.MustCompile(`(?is)<pre[^>]*>(.*?)</pre>`)
	boldRe    = regexp.MustCompile(`(?i)</?(?:b|strong)>`)
	codeRe    = regexp.MustCompile(`(?i)</?code[^>]*>`)
	linkRe    = regexp.MustCompile(`(?is)<a\s+[^>]*href="([^"]*)"[^>]*>(.*?)</a>`)
	otherRe   = regexp.MustCompile(`(?i)</?(?:i|em|u|ins|s|strike|del|tg-spoiler|span|blockquote)[^>]*>`)
)

// htmlToBale rewrites the Telegram HTML subset the bot emits into Bale's
// markdown (bold, inline code, code blocks) and drops the other tags.
func htmlToBale(msg string) string {
	msg = preCodeRe.ReplaceAllString(msg, "<pre>$1</pre>")
	msg = preRe.ReplaceAllString(msg, "\n```\n$1\n```\n")
	msg = boldRe.ReplaceAllString(msg, "*")
	msg = codeRe.ReplaceAllString(msg, "`")
	msg = linkRe.ReplaceAllStringFunc(msg, func(m string) string {
		sub := linkRe.FindStringSubmatch(m)
		if strings.TrimSpace(sub[2]) == sub[1] {
			return sub[1]
		}
		return sub[2] + " (" + sub[1] + ")"
	})
	msg = otherRe.ReplaceAllString(msg, "")
	return html.UnescapeString(msg)
}
