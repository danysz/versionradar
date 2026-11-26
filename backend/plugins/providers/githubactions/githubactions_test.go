package githubactions

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	body := []byte(`{"action":"completed","workflow_run":{"id":101,"name":"Deploy","head_branch":"main","head_sha":"abcdef1234567890","status":"completed","conclusion":"success","display_title":"Release v3"}}`)
	mac := hmac.New(sha256.New, []byte("s3c"))
	mac.Write(body)
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	if err := p.VerifyWebhook(r, body, "s3c"); err != nil {
		t.Fatal(err)
	}
	ev, err := p.ParseWebhook(body, nil)
	if err != nil || ev.Status != core.StatusSuccess || ev.Version != "abcdef1" || ev.ExternalID != "101" {
		t.Fatalf("unexpected webhook: %+v %v", ev, err)
	}

	orig := core.DefaultClient
	defer func() { core.DefaultClient = orig }()
	core.DefaultClient = &http.Client{
		Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"workflow_runs":[{"id":202,"name":"CI","head_sha":"112233445566","status":"in_progress"}]}`)),
			}, nil
		}),
	}
	evs, err := p.CheckStatus(context.Background(), map[string]any{
		"token":   "tok",
		"targets": []any{map[string]any{"product": "p1", "repo": "org/app", "workflow": "ci.yml"}},
	})
	if err != nil || len(evs) != 1 || evs[0].Status != core.StatusInProgress || evs[0].Version != "1122334" {
		t.Fatalf("unexpected poll: %+v %v", evs, err)
	}
}
