// Package catalog reads the public Bitcoin++ API. Spaces credentials are never
// sent to this service.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Conference struct {
	ID       string `json:"id"`
	Tag      string `json:"tag"`
	Name     string `json:"description"`
	Venue    string `json:"venue"`
	Location string `json:"location"`
	StartsAt string `json:"starts_at"`
	EndsAt   string `json:"ends_at"`
}

type Day struct {
	Number int      `json:"day_number"`
	Rooms  []string `json:"venues"`
}

type UploadContext struct {
	Days           []Day  `json:"days"`
	HasHackathon   bool   `json:"hasHackathon"`
	HackathonError string `json:"hackathonError,omitempty"`
}

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(base string) *Client {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = "https://btcpp.dev"
	}
	if !strings.HasSuffix(base, "/api/v1") {
		base += "/api/v1"
	}
	return &Client{BaseURL: base, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) get(ctx context.Context, path string, target any) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("Bitcoin++ API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Bitcoin++ API %s: HTTP %d", req.URL.Path, resp.StatusCode)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
		Meta struct {
			NextCursor string `json:"next_cursor"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&envelope); err != nil {
		return "", fmt.Errorf("decode Bitcoin++ API: %w", err)
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return "", fmt.Errorf("decode Bitcoin++ API data: %w", err)
	}
	return envelope.Meta.NextCursor, nil
}

func (c *Client) Conferences(ctx context.Context) ([]Conference, error) {
	all := []Conference{}
	cursor := ""
	seen := map[string]bool{}
	for {
		var page []Conference
		next, err := c.get(ctx, "/conferences?limit=100&cursor="+url.QueryEscape(cursor), &page)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if next == "" {
			break
		}
		if seen[next] {
			return nil, fmt.Errorf("Bitcoin++ API returned a repeated page cursor")
		}
		seen[next] = true
		cursor = next
	}
	// Keep recent events easy to find while retaining past recordings' events.
	sort.SliceStable(all, func(i, j int) bool { return all[i].StartsAt > all[j].StartsAt })
	return all, nil
}

func (c *Client) UploadContext(ctx context.Context, tag string) (UploadContext, error) {
	out := UploadContext{Days: []Day{}}
	if strings.TrimSpace(tag) == "" {
		return out, fmt.Errorf("choose an event first")
	}
	path := "/conferences/" + url.PathEscape(tag)
	if _, err := c.get(ctx, path+"/days", &out.Days); err != nil {
		return out, err
	}
	if out.Days == nil {
		out.Days = []Day{}
	}
	sort.SliceStable(out.Days, func(i, j int) bool { return out.Days[i].Number < out.Days[j].Number })
	for i := range out.Days {
		rooms := []string{}
		seen := map[string]bool{}
		for _, room := range out.Days[i].Rooms {
			room = strings.TrimSpace(room)
			if room != "" && !seen[room] {
				rooms = append(rooms, room)
				seen[room] = true
			}
		}
		out.Days[i].Rooms = rooms
	}
	var hackathons []json.RawMessage
	if _, err := c.get(ctx, path+"/hackathons?limit=1", &hackathons); err != nil {
		out.HackathonError = "Could not check hackathon availability. " + err.Error()
	} else {
		out.HasHackathon = len(hackathons) > 0
	}
	return out, nil
}
