package service

import (
	"fmt"
	"slices"
	"strings"
)

// Apps that import a JSON array of full Xray configs; selected ones get the
// Xray JSON from the plain subscription URL instead of the share-link list.
const (
	SubJsonAppV2rayNG   = "v2rayng"
	SubJsonAppV2rayN    = "v2rayn"
	SubJsonAppStreisand = "streisand"
)

var subJsonApps = []string{SubJsonAppV2rayNG, SubJsonAppV2rayN, SubJsonAppStreisand}

func (s *SettingService) GetSubJsonApps() (string, error) {
	return s.getString("subJsonApps")
}

func normalizeSubJsonApps(list string) (string, error) {
	var out []string
	for id := range strings.SplitSeq(list, ",") {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" || slices.Contains(out, id) {
			continue
		}
		if !slices.Contains(subJsonApps, id) {
			return "", fmt.Errorf("unknown Xray JSON app %q", id)
		}
		out = append(out, id)
	}
	return strings.Join(out, ","), nil
}
