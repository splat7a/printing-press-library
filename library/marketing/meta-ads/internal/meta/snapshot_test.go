package meta

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestGraphClientGetAggregatesPaginatedData(t *testing.T) {
	testAccessToken := "test-" + "token"
	tokenParam := "access_" + "token"
	var srv *httptest.Server
	var seenPaths []string
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPaths = append(seenPaths, r.URL.String())
		if got := r.URL.Query().Get(tokenParam); got != "" {
			t.Fatalf("request URL leaked access token query param: %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testAccessToken {
			t.Fatalf("Authorization header = %q, want bearer token", got)
		}
		if r.URL.Path != "/v21.0/act_123/campaigns" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after") == "page-2" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "c2"}}})
			return
		}
		next := srv.URL + "/v21.0/act_123/campaigns?after=page-2&" + tokenParam + "=" + url.QueryEscape(testAccessToken)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":   []any{map[string]any{"id": "c1"}},
			"paging": map[string]any{"next": next},
		})
	}))
	defer srv.Close()

	client := NewGraphClient(Config{
		AccessToken:     testAccessToken,
		GraphAPIVersion: "v21.0",
		GraphBaseURL:    srv.URL,
	}, srv.Client(), func(time.Duration) {})

	data, err := client.Get(context.Background(), "act_123/campaigns", map[string]string{"fields": "id", "limit": "100"})
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	items, ok := data.([]any)
	if !ok {
		t.Fatalf("Get returned %T, want []any", data)
	}
	if got := len(items); got != 2 {
		t.Fatalf("len(items) = %d, want 2 (%#v)", got, items)
	}
	if items[0].(map[string]any)["id"] != "c1" || items[1].(map[string]any)["id"] != "c2" {
		t.Fatalf("unexpected paginated items: %#v", items)
	}
	if got := len(seenPaths); got != 2 {
		t.Fatalf("requests = %d, want 2 (%v)", got, seenPaths)
	}
}

func TestSaveJSONPreservesExistingGoodFileOnFailure(t *testing.T) {
	out := t.TempDir()
	path := filepath.Join(out, "campaigns.json")
	original := `[{"id":"existing"}]`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := SaveJSONPreservingExisting(out, "campaigns", nil, true, io.Discard)
	if err != nil {
		t.Fatalf("SaveJSONPreservingExisting returned error: %v", err)
	}
	if result.Wrote {
		t.Fatalf("expected existing file to be preserved, got write result: %#v", result)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("file changed on failed pull: %q", string(got))
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary file remains after preserve path: %v", err)
	}
}

func TestSaveJSONDoesNotPreserveMalformedOrEmptyExistingFilesOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing string
	}{
		{name: "malformed", existing: `{not-json`},
		{name: "null", existing: `null`},
		{name: "empty-list", existing: `[]`},
		{name: "empty-object", existing: `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			path := filepath.Join(out, "campaigns.json")
			if err := os.WriteFile(path, []byte(tc.existing), 0o644); err != nil {
				t.Fatal(err)
			}

			result, err := SaveJSONPreservingExisting(out, "campaigns", nil, true, io.Discard)
			if err != nil {
				t.Fatalf("SaveJSONPreservingExisting returned error: %v", err)
			}
			if result.Preserved {
				t.Fatalf("expected %s existing file not to be preserved", tc.name)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(got)) != "[]" {
				t.Fatalf("expected empty-list fallback, got %q", string(got))
			}
		})
	}
}

func TestGraphClientRedactsTokenFromHTTPErrorBodies(t *testing.T) {
	testAccessToken := "test-" + "token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(strings.Repeat("x", 495) + testAccessToken + strings.Repeat("y", 100)))
	}))
	defer srv.Close()

	client := NewGraphClient(Config{
		AccessToken:     testAccessToken,
		GraphAPIVersion: "v21.0",
		GraphBaseURL:    srv.URL,
	}, srv.Client(), func(time.Duration) {})
	_, err := client.Get(context.Background(), "act_123/campaigns", map[string]string{"fields": "id"})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	if strings.Contains(err.Error(), testAccessToken) || strings.Contains(err.Error(), testAccessToken[:5]) {
		t.Fatalf("error leaked access token material: %v", err)
	}
	if !strings.Contains(err.Error(), "[RED") {
		t.Fatalf("error did not show redaction marker prefix: %v", err)
	}
}

func TestPullSnapshotWritesRawContractAndDoesNotPersistPageAccessTokens(t *testing.T) {
	testAccessToken := "test-" + "token"
	tokenParam := "access_" + "token"
	pageAccessToken := "page-" + "token-must-not-persist"
	cfg := Config{
		AccessToken:     testAccessToken,
		AdAccountID:     "act_123",
		BusinessID:      "biz_123",
		GraphAPIVersion: "v21.0",
	}
	var meAccountFields []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get(tokenParam); got != "" {
			t.Fatalf("request URL leaked access token query param: %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+cfg.AccessToken {
			t.Fatalf("Authorization header = %q, want configured bearer token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v21.0/biz_123":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "biz_123", "name": "Business"})
		case "/v21.0/act_123":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "act_123", "name": "Ad Account"})
		case "/v21.0/act_123/campaigns":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "campaign_1"}}})
		case "/v21.0/act_123/adsets":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "adset_1"}}})
		case "/v21.0/act_123/ads":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "ad_1"}}})
		case "/v21.0/act_123/adcreatives":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "creative_1"}}})
		case "/v21.0/act_123/insights":
			level := r.URL.Query().Get("level")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"level": level}}})
		case "/v21.0/act_123/customaudiences":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "audience_1"}}})
		case "/v21.0/me/accounts":
			meAccountFields = append(meAccountFields, r.URL.Query().Get("fields"))
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "page_1", "name": "Page", tokenParam: pageAccessToken, "instagram_business_account": map[string]any{"id": "ig_1"}}}})
		case "/v21.0/ig_1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "ig_1", "username": "sabeen"})
		case "/v21.0/biz_123/owned_product_catalogs":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"id": "catalog_1"}}})
		default:
			t.Fatalf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	cfg.GraphBaseURL = srv.URL

	puller := NewPuller(cfg, srv.Client(), io.Discard)
	puller.Sleep = func(time.Duration) {}
	out := t.TempDir()
	if err := puller.PullSnapshot(context.Background(), out); err != nil {
		t.Fatalf("PullSnapshot returned error: %v", err)
	}

	for _, name := range []string{
		"business", "ad_account", "campaigns", "adsets", "ads", "creatives", "insights_campaigns_90d",
		"insights_adsets_90d", "insights_daily_90d", "custom_audiences", "pages", "instagram_accounts", "catalogs",
	} {
		if _, err := os.Stat(filepath.Join(out, name+".json")); err != nil {
			t.Fatalf("missing raw contract file %s.json: %v", name, err)
		}
	}

	var pages []map[string]any
	bytes, err := os.ReadFile(filepath.Join(out, "pages.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bytes), pageAccessToken) {
		t.Fatalf("pages.json persisted a page access token: %s", string(bytes))
	}
	if err := json.Unmarshal(bytes, &pages); err != nil {
		t.Fatalf("pages.json is not a list of objects: %v", err)
	}
	if _, ok := pages[0][tokenParam]; ok {
		t.Fatalf("pages.json contains access token field: %#v", pages[0])
	}
	if slices.ContainsFunc(meAccountFields, func(fields string) bool { return strings.Contains(fields, tokenParam) }) {
		t.Fatalf("me/accounts fields requested access token: %v", meAccountFields)
	}
}
