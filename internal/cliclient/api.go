// Package cliclient is `eternal client`: it goes through the running panel's API so
// every change reaches Xray and the nodes exactly as an edit in the web UI does.
package cliclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// API is a panel reachable at Base (scheme, host and base path) with an admin token.
type API struct {
	Base   string
	Token  string
	Client *http.Client
}

type envelope struct {
	Success bool            `json:"success"`
	Msg     string          `json:"msg"`
	Obj     json.RawMessage `json:"obj"`
}

// Client is the part of a panel client the CLI shows and sets.
type Client struct {
	Email      string `json:"email"`
	SubID      string `json:"subId,omitempty"`
	TotalGB    int64  `json:"totalGB"`
	ExpiryTime int64  `json:"expiryTime"`
	LimitIP    int    `json:"limitIp"`
	Enable     bool   `json:"enable"`
	Comment    string `json:"comment,omitempty"`
}

type Traffic struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// Row is one entry of the client list.
type Row struct {
	Client
	InboundIds []int    `json:"inboundIds"`
	Traffic    *Traffic `json:"traffic,omitempty"`
}

func (a *API) call(method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(a.Base, "/")+"/panel/api/"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.Client.Do(req)
	if err != nil {
		return fmt.Errorf("panel not reachable (is eternal running?): %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("panel answered HTTP %d", resp.StatusCode)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("unexpected panel answer: %w", err)
	}
	if !env.Success {
		if env.Msg == "" {
			env.Msg = "request failed"
		}
		return errors.New(env.Msg)
	}
	if out != nil && len(env.Obj) > 0 && string(env.Obj) != "null" {
		return json.Unmarshal(env.Obj, out)
	}
	return nil
}

func (a *API) List() ([]Row, error) {
	var rows []Row
	return rows, a.call(http.MethodGet, "clients/list", nil, &rows)
}

func (a *API) Get(email string) (Row, error) {
	var got struct {
		Client     Client `json:"client"`
		InboundIds []int  `json:"inboundIds"`
	}
	if err := a.call(http.MethodGet, "clients/get/"+url.PathEscape(email), nil, &got); err != nil {
		return Row{}, err
	}
	var t Traffic
	if err := a.call(http.MethodGet, "clients/traffic/"+url.PathEscape(email), nil, &t); err != nil {
		return Row{}, err
	}
	return Row{Client: got.Client, InboundIds: got.InboundIds, Traffic: &t}, nil
}

func (a *API) Add(c Client, inboundIds []int) error {
	return a.call(http.MethodPost, "clients/add", map[string]any{"client": c, "inboundIds": inboundIds}, nil)
}

func (a *API) Delete(email string) error {
	return a.call(http.MethodPost, "clients/del/"+url.PathEscape(email), nil, nil)
}

// SetEnable reports the emails the panel skipped, with why.
func (a *API) SetEnable(emails []string, enable bool) ([]string, error) {
	path := "clients/bulkDisable"
	if enable {
		path = "clients/bulkEnable"
	}
	var res struct {
		Skipped []struct {
			Email  string `json:"email"`
			Reason string `json:"reason"`
		} `json:"skipped"`
	}
	if err := a.call(http.MethodPost, path, map[string]any{"emails": emails}, &res); err != nil {
		return nil, err
	}
	var skipped []string
	for _, s := range res.Skipped {
		skipped = append(skipped, s.Email+": "+s.Reason)
	}
	return skipped, nil
}

func (a *API) ResetTraffic(email string) error {
	return a.call(http.MethodPost, "clients/resetTraffic/"+url.PathEscape(email), nil, nil)
}
