package httpprobe

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

func TestCheckStatusJSONAndWebhook(t *testing.T) {
	orig := core.DefaultClient
	defer func() { core.DefaultClient = orig }()

	core.DefaultClient = &http.Client{
		Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Authorization") != "Bearer secret" {
				return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"build":{"version":"2.4.1","sha":"abcdef123456"}}`)),
			}, nil
		}),
	}

	p := Plugin{}
	evs, err := p.CheckStatus(context.Background(), map[string]any{
		"auth_header": "Bearer secret",
		"targets": []any{map[string]any{
			"product":     "r1",
			"url":         "https://api.example.com/version",
			"json_path":   "build.version",
			"commit_path": "build.sha",
		}},
	})
	if err != nil || len(evs) != 1 || evs[0].Version != "2.4.1" || evs[0].CommitSHA != "abcdef123456" || evs[0].Status != core.StatusSuccess {
		t.Fatalf("unexpected events: %+v err=%v", evs, err)
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("X-VersionRadar-Secret", "s")
	if err := p.VerifyWebhook(req, nil, "s"); err != nil {
		t.Fatal(err)
	}
	ev, err := p.ParseWebhook([]byte(`{"version":"2.5.0","status":"in_progress","stage":"deploying","progress":60}`), nil)
	if err != nil || ev.Status != core.StatusInProgress || ev.Progress != 60 || ev.Version != "2.5.0" {
		t.Fatalf("unexpected webhook event: %+v %v", ev, err)
	}
}
