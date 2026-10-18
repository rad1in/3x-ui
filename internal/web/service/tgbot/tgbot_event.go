package tgbot

import (
	"fmt"
	"html"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/eventbus"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

var cachedHostname string

func getHostname() string {
	if cachedHostname != "" {
		return cachedHostname
	}
	h, err := os.Hostname()
	if err != nil {
		cachedHostname = "unknown"
	} else {
		cachedHostname = h
	}
	return cachedHostname
}

var tgEventLimiter = eventbus.NewRateLimiter(1 * time.Minute)

// HandleEvent is the eventbus subscriber callback. It formats incoming events
// as Telegram messages and sends them to all admin chats.
func (t *Tgbot) HandleEvent(e eventbus.Event) {
	if !t.isEventEnabled(e.Type) {
		return
	}
	if e.Type != eventbus.EventLoginAttempt {
		if !tgEventLimiter.Allow(e.Type, e.Source) {
			return
		}
	}
	msg := t.formatEventMessage(e)
	if msg != "" {
		t.SendMsgToTgbotAdmins(msg)
	}
	t.notifyClientOwner(e)
}

// notifyClientOwner also tells the client, in its linked chat, that its plan runs low.
func (t *Tgbot) notifyClientOwner(e eventbus.Event) {
	if e.Type != eventbus.EventClientQuotaWarning && e.Type != eventbus.EventClientExpiryWarning {
		return
	}
	data, ok := e.Data.(*eventbus.ClientEventData)
	if !ok {
		return
	}
	rec, err := t.clientService.GetRecordByEmail(nil, data.Email)
	if err != nil || rec.TgID == 0 {
		return
	}
	t.SendMsgToTgbot(rec.TgID, t.clientWarningText(e.Type, data, true))
}

func (t *Tgbot) clientWarningText(typ eventbus.EventType, d *eventbus.ClientEventData, own bool) string {
	email := "Email==" + html.EscapeString(d.Email)
	if typ == eventbus.EventClientQuotaWarning {
		usage := "Usage==" + t.I18nBot("tgbot.messages.usagePercent", "Percent=="+strconv.Itoa(d.Percent),
			"Used=="+common.FormatTraffic(d.Used()), "Total=="+common.FormatTraffic(d.Total))
		if own {
			return t.I18nBot("tgbot.messages.ownQuotaWarning", usage)
		}
		return t.I18nBot("tgbot.messages.eventQuotaWarning", email, usage)
	}
	expires := "Expires==" + t.I18nBot("tgbot.messages.expiresOn", "Date=="+d.ExpiryDate(), "Days=="+strconv.Itoa(d.DaysLeft))
	if own {
		return t.I18nBot("tgbot.messages.ownExpiryWarning", expires)
	}
	return t.I18nBot("tgbot.messages.eventExpiryWarning", email, expires)
}

func (t *Tgbot) isEventEnabled(eventType eventbus.EventType) bool {
	events, err := t.settingService.GetTgEnabledEvents()
	if err != nil || events == "" {
		return false
	}
	for e := range strings.SplitSeq(events, ",") {
		if strings.TrimSpace(e) == string(eventType) {
			return true
		}
	}
	return false
}

func (t *Tgbot) formatEventMessage(e eventbus.Event) string {
	host := getHostname()
	header := fmt.Sprintf("<b>📡 %s</b>\n", host)

	switch e.Type {
	case eventbus.EventOutboundDown:
		msg := header + t.I18nBot("tgbot.messages.eventOutboundDown",
			"Tag=="+e.Source)
		if data, ok := e.Data.(*eventbus.OutboundHealthData); ok {
			if data.Error != "" {
				msg += "\n" + t.I18nBot("tgbot.messages.eventErrorDetail",
					"Error=="+data.Error)
			}
			if data.Delay > 0 {
				msg += "\n" + t.I18nBot("tgbot.messages.eventDelayDetail",
					"Delay=="+fmt.Sprintf("%d", data.Delay))
			}
		}
		return msg

	case eventbus.EventOutboundUp:
		msg := header + t.I18nBot("tgbot.messages.eventOutboundUp",
			"Tag=="+e.Source)
		if data, ok := e.Data.(*eventbus.OutboundHealthData); ok && data.Delay > 0 {
			msg += "\n" + t.I18nBot("tgbot.messages.eventDelayDetail",
				"Delay=="+fmt.Sprintf("%d", data.Delay))
		}
		return msg

	case eventbus.EventXrayCrash:
		errStr := ""
		if e.Data != nil {
			errStr = fmt.Sprint(e.Data)
		}
		msg := header + "🔥 " + t.I18nBot("tgbot.messages.eventXrayCrash")
		if errStr != "" {
			msg += "\n" + t.I18nBot("tgbot.messages.eventXrayCrashError", "Error=="+errStr)
		}
		return msg

	case eventbus.EventNodeDown:
		msg := header + "🔴 " + t.I18nBot("tgbot.messages.eventNodeDown", "Name=="+e.Source)
		if data, ok := e.Data.(*eventbus.NodeHealthData); ok && data.XrayError != "" {
			msg += "\n" + t.I18nBot("tgbot.messages.eventErrorDetail", "Error=="+data.XrayError)
		}
		return msg

	case eventbus.EventNodeUp:
		msg := header + "🟢 " + t.I18nBot("tgbot.messages.eventNodeUp", "Name=="+e.Source)
		if data, ok := e.Data.(*eventbus.NodeHealthData); ok && data.LatencyMs > 0 {
			msg += "\n" + t.I18nBot("tgbot.messages.eventDelayDetail", "Delay=="+fmt.Sprintf("%d", data.LatencyMs))
		}
		return msg

	case eventbus.EventNodeCapReached:
		msg := header + "📊 " + t.I18nBot("tgbot.messages.eventNodeCapReached", "Name=="+e.Source)
		if data, ok := e.Data.(*eventbus.NodeCapData); ok {
			msg += "\n" + common.FormatTraffic(data.UsedBytes) + " / " + common.FormatTraffic(data.CapBytes)
			if data.Stopped {
				msg += "\n" + t.I18nBot("tgbot.messages.eventNodeCapStopped")
			}
		}
		return msg

	case eventbus.EventCPUHigh:
		if data, ok := e.Data.(*eventbus.SystemMetricData); ok {
			tgCpu, err := t.settingService.GetTgCpu()
			if err != nil || tgCpu <= 0 || data.Percent <= float64(tgCpu) {
				return ""
			}
			return header + "🔴 " + t.I18nBot("tgbot.messages.cpuThreshold",
				"Percent=="+strconv.FormatFloat(data.Percent, 'f', 2, 64),
				"Threshold=="+strconv.Itoa(tgCpu))
		}
		return ""

	case eventbus.EventMemoryHigh:
		if data, ok := e.Data.(*eventbus.SystemMetricData); ok {
			tgMemory, err := t.settingService.GetTgMemory()
			if err != nil || tgMemory <= 0 || data.Percent <= float64(tgMemory) {
				return ""
			}
			return header + "🔴 " + t.I18nBot("tgbot.messages.memoryThreshold",
				"Percent=="+strconv.FormatFloat(data.Percent, 'f', 2, 64),
				"Threshold=="+strconv.Itoa(tgMemory))
		}
		return ""

	case eventbus.EventClientQuotaWarning, eventbus.EventClientExpiryWarning:
		if data, ok := e.Data.(*eventbus.ClientEventData); ok {
			return header + t.clientWarningText(e.Type, data, false)
		}
		return ""

	case eventbus.EventLoginAttempt:
		if data, ok := e.Data.(*eventbus.LoginEventData); ok {
			if data.Status == "success" {
				msg := t.I18nBot("tgbot.messages.loginSuccess")
				msg += t.I18nBot("tgbot.messages.hostname", "Hostname=="+host)
				msg += t.I18nBot("tgbot.messages.username", "Username=="+data.Username)
				msg += t.I18nBot("tgbot.messages.ip", "IP=="+data.IP)
				msg += t.I18nBot("tgbot.messages.time", "Time=="+data.Time)
				return msg
			}
			msg := t.I18nBot("tgbot.messages.loginFailed")
			msg += t.I18nBot("tgbot.messages.hostname", "Hostname=="+host)
			if data.Reason != "" {
				msg += t.I18nBot("tgbot.messages.reason", "Reason=="+data.Reason)
			}
			msg += t.I18nBot("tgbot.messages.username", "Username=="+data.Username)
			msg += t.I18nBot("tgbot.messages.ip", "IP=="+data.IP)
			msg += t.I18nBot("tgbot.messages.time", "Time=="+data.Time)
			return msg
		}
		return header + t.I18nBot("tgbot.messages.eventLoginFallback", "Source=="+e.Source)
	}

	return ""
}
