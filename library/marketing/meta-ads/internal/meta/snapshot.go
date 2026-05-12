package meta

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	DefaultGraphAPIVersion = "v21.0"
	DefaultGraphBaseURL    = "https://graph.facebook.com"
)

var (
	accessTokenQueryPattern = regexp.MustCompile(`(?i)(access_token=)[^&\s"']+`)
	accessTokenJSONPattern  = regexp.MustCompile(`(?i)("access_token"\s*:\s*")[^"]+(")`)
)

// Config contains Meta Graph credentials and routing. Secrets are loaded from
// environment by the CLI and must never be accepted as command-line flags.
type Config struct {
	AccessToken     string
	AdAccountID     string
	BusinessID      string
	GraphAPIVersion string
	GraphBaseURL    string
}

func (c Config) normalized() Config {
	if c.GraphAPIVersion == "" {
		c.GraphAPIVersion = DefaultGraphAPIVersion
	}
	if c.GraphBaseURL == "" {
		c.GraphBaseURL = DefaultGraphBaseURL
	}
	c.GraphBaseURL = strings.TrimRight(c.GraphBaseURL, "/")
	return c
}

func (c Config) Validate() error {
	var missing []string
	if strings.TrimSpace(c.AccessToken) == "" {
		missing = append(missing, "META_ACCESS_TOKEN")
	}
	if strings.TrimSpace(c.AdAccountID) == "" {
		missing = append(missing, "META_AD_ACCOUNT_ID")
	}
	if strings.TrimSpace(c.BusinessID) == "" {
		missing = append(missing, "META_BUSINESS_ID")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

type GraphClient struct {
	cfg        Config
	httpClient *http.Client
	Sleep      func(time.Duration)
}

func NewGraphClient(cfg Config, httpClient *http.Client, sleep func(time.Duration)) *GraphClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	if sleep == nil {
		sleep = time.Sleep
	}
	return &GraphClient{cfg: cfg.normalized(), httpClient: httpClient, Sleep: sleep}
}

func (c *GraphClient) initialURL(path string, params map[string]string) (string, error) {
	path = strings.TrimLeft(path, "/")
	u, err := url.Parse(c.cfg.GraphBaseURL + "/" + strings.Trim(c.cfg.GraphAPIVersion, "/") + "/" + path)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *GraphClient) Get(ctx context.Context, path string, params map[string]string) (any, error) {
	nextURL, err := c.initialURL(path, params)
	if err != nil {
		return nil, err
	}
	var out []any
	for nextURL != "" {
		payload, err := c.getURL(ctx, nextURL, path)
		if err != nil {
			return nil, err
		}
		data, hasData := payload["data"]
		if !hasData {
			return payload, nil
		}
		items, ok := data.([]any)
		if !ok {
			return nil, fmt.Errorf("%s: Graph response data is %T, want array", path, data)
		}
		out = append(out, items...)
		nextURL = ""
		if paging, ok := payload["paging"].(map[string]any); ok {
			if next, ok := paging["next"].(string); ok {
				nextURL = next
			}
		}
		if nextURL != "" {
			c.Sleep(200 * time.Millisecond)
		}
	}
	return out, nil
}

func (c *GraphClient) getURL(ctx context.Context, requestURL string, label string) (map[string]any, error) {
	requestURL, err := stripAccessTokenFromURL(requestURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Graph URL for %s: %s", label, c.redact(err.Error()))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %s", label, c.redact(err.Error()))
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %s", label, c.redact(err.Error()))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %s", label, c.redact(err.Error()))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyText := c.redact(string(bytes.TrimSpace(body)))
		if len(bodyText) > 500 {
			bodyText = bodyText[:500]
		}
		return nil, fmt.Errorf("HTTP %d on %s: %s", resp.StatusCode, label, bodyText)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode %s: %w", label, err)
	}
	return payload, nil
}

func stripAccessTokenFromURL(requestURL string) (string, error) {
	u, err := url.Parse(requestURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if _, ok := q["access_token"]; ok {
		q.Del("access_token")
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func (c *GraphClient) redact(text string) string {
	if c.cfg.AccessToken != "" {
		text = strings.ReplaceAll(text, c.cfg.AccessToken, "[REDACTED]")
	}
	text = accessTokenQueryPattern.ReplaceAllString(text, `$1[REDACTED]`)
	text = accessTokenJSONPattern.ReplaceAllString(text, `$1[REDACTED]$2`)
	return text
}

type SaveResult struct {
	Name      string
	Path      string
	Wrote     bool
	Preserved bool
	Count     int
}

func SaveJSONPreservingExisting(outDir, name string, data any, failed bool, w io.Writer) (SaveResult, error) {
	if w == nil {
		w = io.Discard
	}
	path := filepath.Join(outDir, name+".json")
	result := SaveResult{Name: name, Path: path}
	if failed || data == nil {
		if hasExistingGoodJSON(path) {
			info, _ := os.Stat(path)
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			fmt.Fprintf(w, "  ⚠ %s.json pull failed; leaving existing %s-byte file in place\n", name, comma(size))
			result.Preserved = true
			return result, nil
		}
		fmt.Fprintf(w, "  ⚠ %s.json pull failed; writing empty list\n", name)
		data = []any{}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return result, err
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return result, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o644); err != nil {
		return result, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return result, err
	}
	result.Wrote = true
	result.Count = countJSONItems(data)
	info, _ := os.Stat(path)
	size := int64(len(encoded))
	if info != nil {
		size = info.Size()
	}
	fmt.Fprintf(w, "  → %s.json (%d items, %s bytes)\n", name, result.Count, comma(size))
	return result, nil
}

func hasExistingGoodJSON(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" {
		return false
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return false
	}
	switch v := decoded.(type) {
	case nil:
		return false
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return false
	}
}

func countJSONItems(data any) int {
	switch v := data.(type) {
	case []any:
		return len(v)
	case []map[string]any:
		return len(v)
	default:
		return 1
	}
}

func comma(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	prefix := len(s) % 3
	if prefix == 0 {
		prefix = 3
	}
	b.WriteString(s[:prefix])
	for i := prefix; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

type Puller struct {
	client *GraphClient
	cfg    Config
	out    io.Writer
	Sleep  func(time.Duration)
}

func NewPuller(cfg Config, httpClient *http.Client, stdout io.Writer) *Puller {
	if stdout == nil {
		stdout = io.Discard
	}
	p := &Puller{cfg: cfg.normalized(), out: stdout}
	p.Sleep = time.Sleep
	p.client = NewGraphClient(p.cfg, httpClient, func(d time.Duration) { p.Sleep(d) })
	return p
}

type snapshotResource struct {
	Number int
	Label  string
	Name   string
	Path   string
	Params map[string]string
}

var snapshotResources = []snapshotResource{
	{1, "business", "business", "{business_id}", map[string]string{"fields": "id,name,verification_status,created_time,primary_page,timezone_id,vertical"}},
	{2, "ad account", "ad_account", "{ad_account_id}", map[string]string{"fields": "id,name,account_status,currency,timezone_name,amount_spent,balance,spend_cap,disable_reason,business_country_code,age,funding_source_details"}},
	{3, "campaigns", "campaigns", "{ad_account_id}/campaigns", map[string]string{"fields": "id,name,objective,status,effective_status,buying_type,bid_strategy,daily_budget,lifetime_budget,start_time,stop_time,created_time,updated_time", "limit": "100"}},
	{4, "adsets", "adsets", "{ad_account_id}/adsets", map[string]string{"fields": "id,name,campaign_id,status,effective_status,optimization_goal,billing_event,bid_amount,daily_budget,lifetime_budget,start_time,end_time,targeting,promoted_object", "limit": "100"}},
	{5, "ads", "ads", "{ad_account_id}/ads", map[string]string{"fields": "id,name,adset_id,campaign_id,status,effective_status,creative,created_time,updated_time", "limit": "100"}},
	{6, "ad creatives", "creatives", "{ad_account_id}/adcreatives", map[string]string{"fields": "id,name,object_story_spec,thumbnail_url,image_url,body,title,call_to_action_type,effective_object_story_id", "limit": "100"}},
	{7, "insights — campaign-level last 90d", "insights_campaigns_90d", "{ad_account_id}/insights", map[string]string{"level": "campaign", "date_preset": "last_90d", "fields": "campaign_id,campaign_name,spend,impressions,reach,clicks,ctr,cpc,cpm,actions,action_values,purchase_roas,website_purchase_roas,frequency", "limit": "200"}},
	{8, "insights — adset-level last 90d", "insights_adsets_90d", "{ad_account_id}/insights", map[string]string{"level": "adset", "date_preset": "last_90d", "fields": "adset_id,adset_name,campaign_id,spend,impressions,reach,clicks,ctr,cpc,cpm,actions,action_values,purchase_roas", "limit": "300"}},
	{9, "insights — daily account 90d", "insights_daily_90d", "{ad_account_id}/insights", map[string]string{"level": "account", "date_preset": "last_90d", "time_increment": "1", "fields": "date_start,spend,impressions,reach,clicks,ctr,cpc,cpm,actions,action_values,purchase_roas", "limit": "200"}},
	{10, "custom audiences", "custom_audiences", "{ad_account_id}/customaudiences", map[string]string{"fields": "id,name,description,subtype,approximate_count_lower_bound,approximate_count_upper_bound,delivery_status,operation_status,time_created,time_updated,retention_days", "limit": "100"}},
	{11, "pages", "pages", "me/accounts", map[string]string{"fields": "id,name,category,fan_count,followers_count,instagram_business_account,tasks", "limit": "50"}},
}

func (p *Puller) PullSnapshot(ctx context.Context, outDir string) error {
	if err := p.cfg.Validate(); err != nil {
		return err
	}
	for _, resource := range snapshotResources {
		fmt.Fprintf(p.out, "[%d] %s\n", resource.Number, resource.Label)
		data, err := p.client.Get(ctx, p.expandPath(resource.Path), cloneParams(resource.Params))
		failed := err != nil
		if failed {
			fmt.Fprintf(p.out, "  %v\n", err)
		}
		if resource.Name == "pages" && !failed {
			data = removeAccessTokenFields(data)
		}
		if _, err := SaveJSONPreservingExisting(outDir, resource.Name, data, failed, p.out); err != nil {
			return err
		}
	}
	if err := p.pullInstagramAccounts(ctx, outDir); err != nil {
		return err
	}
	fmt.Fprintln(p.out, "[13] catalogs / product feeds")
	catalogs, err := p.client.Get(ctx, p.expandPath("{business_id}/owned_product_catalogs"), map[string]string{"fields": "id,name,product_count,vertical,business", "limit": "50"})
	failed := err != nil
	if failed {
		fmt.Fprintf(p.out, "  %v\n", err)
	}
	if _, err := SaveJSONPreservingExisting(outDir, "catalogs", catalogs, failed, p.out); err != nil {
		return err
	}
	fmt.Fprintln(p.out, "\n✅ DONE")
	return nil
}

func (p *Puller) pullInstagramAccounts(ctx context.Context, outDir string) error {
	pagesForIG, err := p.client.Get(ctx, "me/accounts", map[string]string{"fields": "id,name,instagram_business_account"})
	if err != nil {
		fmt.Fprintln(p.out, "[12] instagram business accounts (page discovery failed)")
		fmt.Fprintf(p.out, "  %v\n", err)
		_, saveErr := SaveJSONPreservingExisting(outDir, "instagram_accounts", nil, true, p.out)
		return saveErr
	}
	igIDs := instagramBusinessAccountIDs(pagesForIG)
	fmt.Fprintf(p.out, "[12] instagram business accounts (%d)\n", len(igIDs))
	igData := make([]any, 0, len(igIDs))
	igFailed := false
	for _, id := range igIDs {
		data, err := p.client.Get(ctx, id, map[string]string{"fields": "id,username,name,biography,followers_count,follows_count,media_count,profile_picture_url,website"})
		if err != nil {
			fmt.Fprintf(p.out, "  %v\n", err)
			igFailed = true
			break
		}
		igData = append(igData, data)
	}
	var data any = igData
	if igFailed {
		data = nil
	}
	_, saveErr := SaveJSONPreservingExisting(outDir, "instagram_accounts", data, igFailed, p.out)
	return saveErr
}

func (p *Puller) expandPath(path string) string {
	path = strings.ReplaceAll(path, "{business_id}", p.cfg.BusinessID)
	path = strings.ReplaceAll(path, "{ad_account_id}", p.cfg.AdAccountID)
	return path
}

func cloneParams(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func instagramBusinessAccountIDs(data any) []string {
	items, ok := data.([]any)
	if !ok {
		return nil
	}
	ids := make([]string, 0)
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ig, ok := m["instagram_business_account"].(map[string]any)
		if !ok {
			continue
		}
		id, ok := ig["id"].(string)
		if ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func removeAccessTokenFields(value any) any {
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = removeAccessTokenFields(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			if strings.EqualFold(k, "access_token") {
				continue
			}
			out[k] = removeAccessTokenFields(item)
		}
		return out
	default:
		return value
	}
}

func RawFilenames() []string {
	return []string{
		"business.json",
		"ad_account.json",
		"campaigns.json",
		"adsets.json",
		"ads.json",
		"creatives.json",
		"insights_campaigns_90d.json",
		"insights_adsets_90d.json",
		"insights_daily_90d.json",
		"custom_audiences.json",
		"pages.json",
		"instagram_accounts.json",
		"catalogs.json",
	}
}

var ErrEndpointFailed = errors.New("meta graph endpoint failed")
