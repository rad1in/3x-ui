package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/util/random"
)

const (
	nodeJoinSetting   = "nodeJoinTokens"
	nodeJoinMaxTokens = 20
	nodeJoinProbe     = 6 * time.Second
	// JoinScriptURL is the one-command node installer the join command downloads.
	JoinScriptURL = "https://raw.githubusercontent.com/rad1in/3x-ui/main/join-node.sh"
)

// nodeJoinMu serializes token claims so one token can only ever add one node.
var nodeJoinMu sync.Mutex

type joinToken struct {
	Hash    string `json:"hash"`
	Expires int64  `json:"expires"`
}

// NodeJoinRequest is what a freshly installed node reports about itself. Only
// these fields are accepted from an unauthenticated caller; everything else about
// the node is fixed by the master.
type NodeJoinRequest struct {
	Name             string `json:"name"`
	Scheme           string `json:"scheme"`
	Address          string `json:"address"`
	Port             int    `json:"port"`
	BasePath         string `json:"basePath"`
	ApiToken         string `json:"apiToken"`
	TlsVerifyMode    string `json:"tlsVerifyMode"`
	PinnedCertSha256 string `json:"pinnedCertSha256"`
}

// NodeJoinService hands out single-use tokens that let a node register itself.
type NodeJoinService struct {
	nodes    NodeService
	settings SettingService

	// now and reachable are test seams; zero values use the clock and a real probe.
	now       func() time.Time
	reachable func(ctx context.Context, req *NodeMutationRequest) error
}

func (s *NodeJoinService) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func hashJoinToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *NodeJoinService) load() []joinToken {
	raw, _ := s.settings.getString(nodeJoinSetting)
	var tokens []joinToken
	_ = json.Unmarshal([]byte(raw), &tokens)
	return tokens
}

func (s *NodeJoinService) store(tokens []joinToken) error {
	blob, err := json.Marshal(tokens)
	if err != nil {
		return err
	}
	return s.settings.setString(nodeJoinSetting, string(blob))
}

func (s *NodeJoinService) live(tokens []joinToken) []joinToken {
	nowMs := s.clock().UnixMilli()
	out := tokens[:0:0]
	for _, t := range tokens {
		if t.Expires > nowMs {
			out = append(out, t)
		}
	}
	return out
}

// Issue creates a token valid for ttl and returns it with its expiry (unix ms).
// Only a hash is stored, so the token cannot be read back later.
func (s *NodeJoinService) Issue(ttl time.Duration) (string, int64, error) {
	nodeJoinMu.Lock()
	defer nodeJoinMu.Unlock()
	token := random.Seq(40)
	expires := s.clock().Add(ttl).UnixMilli()
	tokens := append(s.live(s.load()), joinToken{Hash: hashJoinToken(token), Expires: expires})
	if len(tokens) > nodeJoinMaxTokens {
		tokens = tokens[len(tokens)-nodeJoinMaxTokens:]
	}
	if err := s.store(tokens); err != nil {
		return "", 0, err
	}
	return token, expires, nil
}

// claim removes and returns the live token matching token; nil when none does.
func (s *NodeJoinService) claim(token string) (*joinToken, error) {
	nodeJoinMu.Lock()
	defer nodeJoinMu.Unlock()
	want := hashJoinToken(token)
	tokens := s.live(s.load())
	for i, t := range tokens {
		if subtle.ConstantTimeCompare([]byte(t.Hash), []byte(want)) == 1 {
			rest := append(append([]joinToken{}, tokens[:i]...), tokens[i+1:]...)
			if err := s.store(rest); err != nil {
				return nil, err
			}
			return &t, nil
		}
	}
	return nil, nil
}

func (s *NodeJoinService) restore(t *joinToken) {
	nodeJoinMu.Lock()
	defer nodeJoinMu.Unlock()
	_ = s.store(append(s.live(s.load()), *t))
}

func (s *NodeJoinService) probe(ctx context.Context, req *NodeMutationRequest) error {
	if s.reachable != nil {
		return s.reachable(ctx, req)
	}
	node, err := s.nodes.RuntimeNodeFromRequest(0, req)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, nodeJoinProbe)
	defer cancel()
	if _, err := s.nodes.Probe(ctx, node); err != nil {
		return common.NewError(FriendlyProbeError(err.Error()))
	}
	return nil
}

// Join adds the node described by in when token is valid. A node the master
// cannot reach, or any other refusal, leaves the token usable for a retry.
func (s *NodeJoinService) Join(token string, in NodeJoinRequest) (*NodeView, error) {
	if in.TlsVerifyMode == "" {
		in.TlsVerifyMode = "verify"
	}
	if in.TlsVerifyMode != "verify" && in.TlsVerifyMode != "skip" && in.TlsVerifyMode != "pin" {
		return nil, common.NewError("a joining node may use verify, skip or pin TLS only")
	}
	if strings.TrimSpace(in.ApiToken) == "" {
		return nil, common.NewError("the node's API token is required")
	}
	claimed, err := s.claim(token)
	if err != nil {
		return nil, err
	}
	if claimed == nil {
		return nil, common.NewError("invalid or expired join token")
	}
	apiToken := in.ApiToken
	req := &NodeMutationRequest{
		Name: strings.TrimSpace(in.Name), Scheme: in.Scheme, Address: strings.TrimSpace(in.Address), Port: in.Port,
		BasePath: in.BasePath, ApiToken: &apiToken, Enable: true, TlsVerifyMode: in.TlsVerifyMode,
		PinnedCertSha256: in.PinnedCertSha256, InboundSyncMode: "all",
	}
	if err := s.probe(context.Background(), req); err != nil {
		s.restore(claimed)
		return nil, err
	}
	view, err := s.nodes.CreateFromRequest(req)
	if err != nil {
		s.restore(claimed)
		return nil, err
	}
	return view, nil
}

// NodeJoinCommand is the join command with the token it carries and when it expires.
type NodeJoinCommand struct {
	Command   string `json:"command" example:"XUI_NONINTERACTIVE=1 XUI_JOIN_URL='https://panel.example.com:2053/abc/join/node' XUI_JOIN_TOKEN='...' bash <(curl -Ls ...)"`
	JoinURL   string `json:"joinUrl" example:"https://panel.example.com:2053/abc/join/node"`
	ExpiresAt int64  `json:"expiresAt" example:"1760003600000"`
}

// IssueCommand creates a one-hour token and the command that uses it against masterURL.
func (s *NodeJoinService) IssueCommand(masterURL, name string) (*NodeJoinCommand, error) {
	token, expires, err := s.Issue(time.Hour)
	if err != nil {
		return nil, err
	}
	return &NodeJoinCommand{
		Command:   JoinCommand(masterURL, token, name),
		JoinURL:   strings.TrimRight(masterURL, "/") + "/join/node",
		ExpiresAt: expires,
	}, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// JoinCommand is the shell command an admin runs on a new server: it installs
// the panel and registers the server on the master at masterURL (with base path).
func JoinCommand(masterURL, token, name string) string {
	joinURL := strings.TrimRight(masterURL, "/") + "/join/node"
	parts := []string{
		"XUI_NONINTERACTIVE=1",
		"XUI_JOIN_URL=" + shellQuote(joinURL),
		"XUI_JOIN_TOKEN=" + shellQuote(token),
	}
	if name != "" {
		parts = append(parts, "XUI_NODE_NAME="+shellQuote(name))
	}
	return strings.Join(parts, " ") + " bash <(curl -Ls " + JoinScriptURL + ")"
}
