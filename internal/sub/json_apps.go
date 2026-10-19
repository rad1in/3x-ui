package sub

import (
	"regexp"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

// xrayJsonAppUserAgents matches the clients whose subscription parser imports a
// JSON array of full Xray configs (v2rayNG CustomFmt, v2rayN V2rayFmt, Streisand).
var xrayJsonAppUserAgents = map[string]*regexp.Regexp{
	service.SubJsonAppV2rayNG:   regexp.MustCompile(`(?i)^v2rayng/`),
	service.SubJsonAppV2rayN:    regexp.MustCompile(`(?i)^v2rayn/`),
	service.SubJsonAppStreisand: regexp.MustCompile(`(?i)\bstreisand\b`),
}

type xrayJsonApps []*regexp.Regexp

func parseXrayJsonApps(list string) xrayJsonApps {
	var apps xrayJsonApps
	for id := range strings.SplitSeq(list, ",") {
		if re := xrayJsonAppUserAgents[strings.ToLower(strings.TrimSpace(id))]; re != nil {
			apps = append(apps, re)
		}
	}
	return apps
}

func (apps xrayJsonApps) match(userAgent string) bool {
	for _, re := range apps {
		if re.MatchString(userAgent) {
			return true
		}
	}
	return false
}
