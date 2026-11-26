package gitlabci

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

func TestWebhookAndPoll(t *testing.T) {
	p := Plugin{}
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-Gitlab-Token", "tok")
	if err := p.VerifyWebhook(r, nil, "tok"); err != nil {
		t.Fatal(err)
	}
	ev, err := p.ParseWebhook([]byte(`{"object_attributes":{"id":55,"ref":"main","sha":"abcdef123456","status":"running"},"builds":[{"status":"success"},{"status":"running"}],"commit":{"message":"Deploy API"}}`), nil)
	if err != nil || ev.Status != core.StatusInProgress || ev.Progress != 55 || ev.Stage != "jobs 1/2" || ev.Version != "abcdef1" {
		t.Fatalf("unexpected webhook: %+v %v", ev, err)
	}

	orig := core.DefaultClient
	defer func() { core.DefaultClient = orig }()
	core.DefaultClient = &http.Client{
		Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`[{"id":56,"ref":"main","sha":"deadbeef1234","status":"success","updated_at":"2026-01-02T03:04:05Z"}]`)),
			}, nil
		}),
	}
	evs, err := p.CheckStatus(context.Background(), map[string]any{
		"token":   "glpat",
		"targets": []any{map[string]any{"product": "p1", "project": "grp/app"}},
	})
	if err != nil || len(evs) != 1 || evs[0].Status != core.StatusSuccess || evs[0].Version != "deadbee" {
		t.Fatalf("unexpected poll: %+v %v", evs, err)
	}
}
