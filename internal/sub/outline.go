package sub

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

var errNoOutlineKey = errors.New("no Outline-compatible Shadowsocks key")

// outlineMethods are the ciphers the Outline client speaks; Xray's
// chacha20-poly1305 is the IETF variant Outline names chacha20-ietf-poly1305.
var outlineMethods = map[string]string{
	"aes-128-gcm":            "aes-128-gcm",
	"aes-192-gcm":            "aes-192-gcm",
	"aes-256-gcm":            "aes-256-gcm",
	"chacha20-poly1305":      "chacha20-ietf-poly1305",
	"chacha20-ietf-poly1305": "chacha20-ietf-poly1305",
}

// OutlineAccessKey is the JSON body of an Outline dynamic access key.
type OutlineAccessKey struct {
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Password   string `json:"password"`
	Method     string `json:"method"`
}

// OutlineKey is one ssconf:// link the subscription page offers.
type OutlineKey struct {
	InboundID int    `json:"inboundId"`
	Remark    string `json:"remark"`
	URL       string `json:"url"`
}

type outlineEntry struct {
	inboundID int
	remark    string
	key       OutlineAccessKey
}

func outlineKeyFromProxy(p map[string]any) (OutlineAccessKey, bool) {
	if p["type"] != "ss" || p["plugin"] != nil || p["tls"] == true {
		return OutlineAccessKey{}, false
	}
	if network, _ := p["network"].(string); network != "" && network != "tcp" {
		return OutlineAccessKey{}, false
	}
	cipher, _ := p["cipher"].(string)
	method, ok := outlineMethods[strings.ToLower(cipher)]
	server, _ := p["server"].(string)
	password, _ := p["password"].(string)
	port := clashInt(p["port"])
	if !ok || server == "" || password == "" || port <= 0 {
		return OutlineAccessKey{}, false
	}
	return OutlineAccessKey{Server: server, ServerPort: port, Password: password, Method: method}, true
}

// outlineEntries lists, per Shadowsocks inbound of subId, the first endpoint
// Outline can dial: AEAD cipher, TCP, no plugin or TLS.
func (s *SubClashService) outlineEntries(subId, host string) ([]outlineEntry, error) {
	subReq := s.SubService.ForRequest(host)
	subReq.subscriptionBody = true
	inbounds, err := subReq.getInboundsBySubId(subId)
	if err != nil {
		return nil, err
	}
	var entries []outlineEntry
	for _, inbound := range inbounds {
		if inbound.Protocol != model.Shadowsocks || inbound.ExcludeFromSub {
			continue
		}
		if network, _ := subReq.linkSettings(inbound)["network"].(string); network == "udp" {
			continue
		}
		clients := subReq.matchingClients(inbound, subId)
		if len(clients) == 0 {
			continue
		}
		subReq.projectThroughFallbackMaster(inbound)
		if hostEps := subReq.hostEndpoints(inbound, "clash"); len(hostEps) > 0 {
			injectExternalProxy(inbound, hostEps)
		}
		for _, p := range s.getProxies(subReq, inbound, clients[0], host) {
			if key, ok := outlineKeyFromProxy(p); ok {
				remark := inbound.Remark
				if remark == "" {
					remark = inbound.Tag
				}
				entries = append(entries, outlineEntry{inboundID: inbound.Id, remark: remark, key: key})
				break
			}
		}
	}
	return entries, nil
}

// OutlineAccessKey returns the dynamic access key of one of subId's inbounds.
func (s *SubClashService) OutlineAccessKey(subId, host string, inboundId int) (OutlineAccessKey, error) {
	entries, err := s.outlineEntries(subId, host)
	if err != nil {
		return OutlineAccessKey{}, err
	}
	for _, e := range entries {
		if e.inboundID == inboundId {
			return e.key, nil
		}
	}
	return OutlineAccessKey{}, errNoOutlineKey
}

// OutlineKeys builds the ssconf:// links under subURL. Outline swaps ssconf
// for https and never fetches plain HTTP, so an http subURL yields none.
func (s *SubClashService) OutlineKeys(subId, host, subURL string) []OutlineKey {
	rest, ok := strings.CutPrefix(strings.SplitN(subURL, "?", 2)[0], "https://")
	if !ok {
		return nil
	}
	entries, err := s.outlineEntries(subId, host)
	if err != nil {
		return nil
	}
	keys := make([]OutlineKey, 0, len(entries))
	for _, e := range entries {
		link := "ssconf://" + strings.TrimRight(rest, "/") + "/outline/" + strconv.Itoa(e.inboundID) + "#" + url.PathEscape(e.remark)
		keys = append(keys, OutlineKey{InboundID: e.inboundID, Remark: e.remark, URL: link})
	}
	return keys
}
