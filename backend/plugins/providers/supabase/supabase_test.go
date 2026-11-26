package supabase

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

func TestCheckStatusMigrationsAndFunctions(t *testing.T) {
	_ = httptest.NewRequest
	orig := core.DefaultClient
	defer func() { core.DefaultClient = orig }()

	core.DefaultClient = &http.Client{
		Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Authorization") != "Bearer sbp_tok" {
				return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
			switch r.URL.Path {
			case "/v1/projects/abcdefghijklmnopqrst/database/migrations":
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`[{"version":"20261101120000","name":"init"},{"version":"20261125153000","name":"add_radars_index"}]`)),
				}, nil
			case "/v1/projects/abcdefghijklmnopqrst/functions":
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(strings.NewReader(`[{"id":"f1","slug":"send-email","status":"ACTIVE","version":7,"updated_at":1700000000000}]`)),
				}, nil
			default:
				return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
		}),
	}

	p := Plugin{}
	evs, err := p.CheckStatus(context.Background(), map[string]any{
		"access_token": "sbp_tok",
		"targets": []any{
			map[string]any{"product": "r1", "project_ref": "abcdefghijklmnopqrst", "resource": "migrations"},
			map[string]any{"product": "r2", "project_ref": "abcdefghijklmnopqrst", "resource": "functions", "function_slug": "send-email"},
		},
	})
	if err != nil || len(evs) != 3 {
		t.Fatalf("unexpected events: %+v err=%v", evs, err)
	}
	if evs[0].Version != "20261125153000" || evs[0].Message != "add_radars_index" {
		t.Fatalf("newest migration should sort first: %+v", evs[0])
	}
	if evs[2].Version != "v7" || evs[2].Status != core.StatusSuccess {
		t.Fatalf("unexpected function event: %+v", evs[2])
	}
}
