package cloudflare

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danysz/versionradar/backend/core"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVerifyAndParse(t *testing.T) {
	p := Plugin{}
	r := httptest.NewRequest("POST", "/", nil)
	if p.VerifyWebhook(r, nil, "s3cret") == nil {
		t.Fatal("missing header accepted")
	}
	r.Header.Set("cf-webhook-auth", "s3cret")
	if err := p.VerifyWebhook(r, nil, "s3cret"); err != nil {
		t.Fatal(err)
	}
	ev, err := p.ParseWebhook([]byte(`{"text":"hi","data":{"event":"EVENT_DEPLOYMENT_FAILED","deployment_id":"d1","commit_hash":"abcdef123456","project_name":"web","environment":"ENVIRONMENT_PRODUCTION"}}`), nil)
	if err != nil || ev.Status != "failed" || ev.Version != "abcdef1" || ev.CommitSHA != "abcdef123456" || ev.ExternalID != "d1" || ev.Message != "web production failed" {
		t.Fatalf("%+v %v", ev, err)
	}
}

func TestCheckStatus(t *testing.T) {
	orig := core.DefaultClient
	defer func() { core.DefaultClient = orig }()
	core.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Authorization") != "Bearer tok" || r.URL.Path != "/client/v4/accounts/a/pages/projects/p/deployments" {
				return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("no"))}, nil
			}
			body := `{"result":[{"id":"d1","created_on":"2026-01-02T03:04:05Z","environment":"production","latest_stage":{"name":"deploy","status":"success"},"deployment_trigger":{"metadata":{"commit_hash":"abcdef123456","branch":"main","commit_message":"fix\nmore"}}},{"id":"d2","latest_stage":{"name":"build","status":"success"},"deployment_trigger":{"metadata":{}}},{"id":"d3","latest_stage":{"name":"queued","status":"skipped"}}]}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		}),
	}

	evs, err := Plugin{}.CheckStatus(context.Background(), map[string]any{"api_token": "tok",
		"targets": []any{map[string]any{"product": "pr", "account_id": "a", "project": "p"}}})
	// d2: build finished but not deployed yet -> still in progress; d3 skipped -> dropped.
	if err != nil || len(evs) != 2 || evs[0].Status != "success" || evs[0].Version != "abcdef1" || evs[0].CommitSHA != "abcdef123456" || evs[0].ProductID != "pr" ||
		evs[0].Message != "production main: fix" || evs[0].Progress != 100 || evs[1].Status != "in_progress" || evs[1].Progress != 75 {
		t.Fatalf("%+v %v", evs, err)
	}
}
