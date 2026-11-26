package vercel

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
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

func TestVerifyParseAndCheckStatus(t *testing.T) {
	p := Plugin{}
	body := []byte(`{"type":"deployment.succeeded","createdAt":1700000000000,"payload":{"target":"production","deployment":{"id":"dpl_1","meta":{"githubCommitSha":"abcdef1234567890","githubCommitMessage":"Ship v2"}}}}`)
	mac := hmac.New(sha1.New, []byte("sec"))
	mac.Write(body)
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("x-vercel-signature", hex.EncodeToString(mac.Sum(nil)))
	if err := p.VerifyWebhook(r, body, "sec"); err != nil {
		t.Fatal(err)
	}
	ev, err := p.ParseWebhook(body, nil)
	if err != nil || ev.Status != core.StatusSuccess || ev.Version != "abcdef1" || ev.CommitSHA != "abcdef1234567890" {
		t.Fatalf("unexpected webhook event: %+v %v", ev, err)
	}

	orig := core.DefaultClient
	defer func() { core.DefaultClient = orig }()
	core.DefaultClient = &http.Client{
		Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"deployments":[{"uid":"dpl_2","readyState":"BUILDING","target":"production","created":1700000001000,"meta":{"githubCommitSha":"1234567890ab"}}],"pagination":{}}`)),
			}, nil
		}),
	}
	evs, err := p.CheckStatus(context.Background(), map[string]any{
		"api_token": "tok",
		"targets":   []any{map[string]any{"product": "p1", "project_id": "prj_1"}},
	})
	if err != nil || len(evs) != 1 || evs[0].Status != core.StatusInProgress || evs[0].Version != "1234567" {
		t.Fatalf("unexpected poll events: %+v %v", evs, err)
	}
}
