package archive

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

// cloudflareAPI and rdapBase are replaced in tests.
var (
	cloudflareAPI = "https://api.cloudflare.com/client/v4"
	rdapBase      = "https://rdap.org/domain/"
	httpClient    = &http.Client{Timeout: 20 * time.Second}
)

// cloudflare is a read-only client for the few endpoints the archive needs.
// The token needs Zone Read and DNS Read; Registrar data is optional.
type cloudflare struct{ token string }

type cfZone struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	NameServers []string `json:"name_servers"`
	Account     struct {
		ID string `json:"id"`
	} `json:"account"`
}

type cfRecord struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

// cfRegistrarDomain tolerates both name and id carrying the domain name;
// the public documentation does not list a name or auto-renew field.
type cfRegistrarDomain struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ExpiresAt string `json:"expires_at"`
	AutoRenew *bool  `json:"auto_renew"`
}

func (d cfRegistrarDomain) domain() string {
	if d.Name != "" {
		return strings.ToLower(d.Name)
	}
	if strings.Contains(d.ID, ".") {
		return strings.ToLower(d.ID)
	}
	return ""
}

type cfEnvelope struct {
	Success bool `json:"success"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	Result     json.RawMessage `json:"result"`
	ResultInfo struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

func (c cloudflare) get(ctx context.Context, path string, query url.Values) (cfEnvelope, error) {
	return c.send(ctx, http.MethodGet, path, query, nil)
}

func (c cloudflare) send(ctx context.Context, method, path string, query url.Values, body any) (cfEnvelope, error) {
	var env cfEnvelope
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return env, err
		}
		reader = bytes.NewReader(b)
	}
	target := cloudflareAPI + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return env, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := httpClient.Do(req)
	if err != nil {
		return env, fmt.Errorf("cloudflare %s: %w", path, err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return env, err
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return env, fmt.Errorf("cloudflare %s: HTTP %d, unexpected response", path, res.StatusCode)
	}
	if !env.Success || res.StatusCode >= 300 {
		var msgs []string
		for _, e := range env.Errors {
			msgs = append(msgs, fmt.Sprintf("%d %s", e.Code, e.Message))
		}
		return env, fmt.Errorf("cloudflare %s: HTTP %d %s", path, res.StatusCode, strings.Join(msgs, "; "))
	}
	return env, nil
}

// pages fetches every page of a paginated list into out (a pointer to a slice).
func pages[T any](ctx context.Context, c cloudflare, path string, perPage int) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		env, err := c.get(ctx, path, url.Values{"page": {fmt.Sprint(page)}, "per_page": {fmt.Sprint(perPage)}})
		if err != nil {
			return nil, err
		}
		var items []T
		if err := json.Unmarshal(env.Result, &items); err != nil {
			return nil, fmt.Errorf("cloudflare %s: %w", path, err)
		}
		all = append(all, items...)
		if env.ResultInfo.TotalPages <= page || len(items) == 0 {
			return all, nil
		}
	}
}

func (c cloudflare) zones(ctx context.Context) ([]cfZone, error) {
	return pages[cfZone](ctx, c, "/zones", 50)
}

func (c cloudflare) records(ctx context.Context, zoneID string) ([]cfRecord, error) {
	return pages[cfRecord](ctx, c, "/zones/"+url.PathEscape(zoneID)+"/dns_records", 100)
}

// registrar lists Cloudflare Registrar domains of an account, keyed by name.
func (c cloudflare) registrar(ctx context.Context, accountID string) (map[string]cfRegistrarDomain, error) {
	env, err := c.get(ctx, "/accounts/"+url.PathEscape(accountID)+"/registrar/domains", url.Values{})
	if err != nil {
		return nil, err
	}
	var items []cfRegistrarDomain
	if err := json.Unmarshal(env.Result, &items); err != nil {
		return nil, err
	}
	out := map[string]cfRegistrarDomain{}
	for _, d := range items {
		if n := d.domain(); n != "" {
			out[n] = d
		}
	}
	return out, nil
}

// rdapExpiry reads the registration expiry date from public RDAP.
func rdapExpiry(ctx context.Context, domain string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rdapBase+url.PathEscape(domain), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/rdap+json")
	res, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("rdap: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("rdap: HTTP %d", res.StatusCode)
	}
	var body struct {
		Events []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&body); err != nil {
		return "", fmt.Errorf("rdap: %w", err)
	}
	for _, e := range body.Events {
		if e.Action == "expiration" {
			if d := dateOf(e.Date); d != "" {
				return d, nil
			}
		}
	}
	return "", errors.New("rdap: no expiration event")
}

// dateOf converts an RFC 3339 timestamp to YYYY-MM-DD in UTC.
func dateOf(ts string) string {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.UTC().Format("2006-01-02")
	}
	if len(ts) >= 10 && validDate(ts[:10]) {
		return ts[:10]
	}
	return ""
}

// WorkerRoute is a Workers route; an empty Script means "no Worker" (origin).
type WorkerRoute struct {
	ID      string `json:"id"`
	Pattern string `json:"pattern"`
	Script  string `json:"script,omitempty"`
}

func (c cloudflare) workerRoutes(ctx context.Context, zoneID string) ([]WorkerRoute, error) {
	env, err := c.get(ctx, "/zones/"+url.PathEscape(zoneID)+"/workers/routes", nil)
	if err != nil {
		return nil, err
	}
	var routes []WorkerRoute
	if err := json.Unmarshal(env.Result, &routes); err != nil {
		return nil, err
	}
	return routes, nil
}

func (c cloudflare) createRoute(ctx context.Context, zoneID, pattern, script string) error {
	body := map[string]any{"pattern": pattern}
	if script != "" {
		body["script"] = script
	}
	_, err := c.send(ctx, http.MethodPost, "/zones/"+url.PathEscape(zoneID)+"/workers/routes", nil, body)
	return err
}

func (c cloudflare) deleteRoute(ctx context.Context, zoneID, routeID string) error {
	_, err := c.send(ctx, http.MethodDelete, "/zones/"+url.PathEscape(zoneID)+"/workers/routes/"+url.PathEscape(routeID), nil, nil)
	return err
}

type CloudflareRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
}

func (c cloudflare) recordsFull(ctx context.Context, zoneID string) ([]CloudflareRecord, error) {
	return pages[CloudflareRecord](ctx, c, "/zones/"+url.PathEscape(zoneID)+"/dns_records", 100)
}

func (c cloudflare) upsertRecord(ctx context.Context, zoneID, recordID string, rec CloudflareRecord) error {
	body := map[string]any{"type": rec.Type, "name": rec.Name, "content": rec.Content, "ttl": rec.TTL, "proxied": rec.Proxied}
	if recordID == "" {
		_, err := c.send(ctx, http.MethodPost, "/zones/"+url.PathEscape(zoneID)+"/dns_records", nil, body)
		return err
	}
	_, err := c.send(ctx, http.MethodPut, "/zones/"+url.PathEscape(zoneID)+"/dns_records/"+url.PathEscape(recordID), nil, body)
	return err
}
