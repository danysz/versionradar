package neon

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danysz/versionradar/backend/core"
)

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCheckStatusAndWebhook(t *testing.T) {
	orig := core.DefaultClient
	defer func() { core.DefaultClient = orig }()

	core.DefaultClient = &http.Client{
		Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/api/v2/projects/proj-1/branches":
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"branches":[{"id":"br-main","name":"main","current_state":"ready"}]}`)),
				}, nil
			case "/api/v2/projects/proj-1/operations":
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`{"operations":[{"id":"op-1","branch_id":"br-main","action":"apply_config","status":"finished","updated_at":"2026-02-01T12:00:00Z"},{"id":"op-2","branch_id":"br-other","action":"create_branch","status":"running"}]}`)),
				}, nil
			default:
				return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
		}),
	}

	p := Plugin{}
	evs, err := p.CheckStatus(context.Background(), map[string]any{
		"api_key": "neon_key",
		"targets": []any{map[string]any{"product": "r1", "project_id": "proj-1", "branch_name": "main"}},
	})
	if err != nil || len(evs) != 1 || evs[0].Status != core.StatusSuccess || evs[0].Version != "main" {
		t.Fatalf("unexpected events: %+v %v", evs, err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-VersionRadar-Secret", "s")
	if err := p.VerifyWebhook(req, nil, "s"); err != nil {
		t.Fatal(err)
	}
	ev, err := p.ParseWebhook([]byte(`{"version":"20261126_init","branch":"main","status":"success","commit_sha":"abcdef123456"}`), nil)
	if err != nil || ev.Status != core.StatusSuccess || ev.Version != "20261126_init" || ev.CommitSHA != "abcdef123456" {
		t.Fatalf("unexpected webhook: %+v %v", ev, err)
	}
}
