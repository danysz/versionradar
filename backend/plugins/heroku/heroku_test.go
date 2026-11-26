package heroku

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danysz/versionradar/backend/core"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVerifyAndParseWebhook(t *testing.T) {
	p := Plugin{}
	secret := "heroku-secret"

	buildBody := []byte(`{"action":"create","resource":"build","data":{"id":"b1","status":"pending","source_blob":{"version":"abcdef123456"}}}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(buildBody)
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	if p.VerifyWebhook(r, buildBody, secret) == nil {
		t.Fatal("unsigned request accepted")
	}
	r.Header.Set("Heroku-Webhook-Hmac-SHA256", sig)
	if err := p.VerifyWebhook(r, buildBody, secret); err != nil {
		t.Fatal(err)
	}
	ev, err := p.ParseWebhook(buildBody, nil)
	if err != nil || ev.Status != core.StatusInProgress || ev.Stage != "building" || ev.Version != "abcdef1" || ev.CommitSHA != "abcdef123456" {
		t.Fatalf("unexpected build event: %+v %v", ev, err)
	}

	relBody := []byte(`{"action":"update","resource":"release","data":{"id":"r42","version":42,"status":"succeeded","description":"Deploy abcdef12"}}`)
	ev, err = p.ParseWebhook(relBody, nil)
	if err != nil || ev.Status != core.StatusSuccess || ev.Version != "v42" || ev.CommitSHA != "abcdef12" || ev.Progress != 100 {
		t.Fatalf("unexpected release event: %+v %v", ev, err)
	}
}

func TestCheckStatusAndConnectWebhook(t *testing.T) {
	orig := core.DefaultClient
	defer func() { core.DefaultClient = orig }()

	createdHook := false
	core.DefaultClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/apps/my-app/builds":
				body := `[{"id":"b1","status":"succeeded","source_blob":{"version":"deadbeef1234"},"release":{"id":"r10"}},{"id":"b2","status":"pending","source_blob":{"version":"cafebabe1234"}}]`
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			case r.Method == http.MethodGet && r.URL.Path == "/apps/my-app/releases":
				body := `[{"id":"r10","version":10,"status":"succeeded","description":"Deploy deadbeef","created_at":"2026-01-02T03:04:05Z"}]`
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
			case r.Method == http.MethodGet && r.URL.Path == "/apps/my-app/webhooks":
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[]`))}, nil
			case r.Method == http.MethodPost && r.URL.Path == "/apps/my-app/webhooks":
				createdHook = true
				return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"id":"wh-123"}`))}, nil
			default:
				return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
		}),
	}

	p := Plugin{}
	evs, err := p.CheckStatus(context.Background(), map[string]any{
		"api_token": "tok",
		"targets":   []any{map[string]any{"product": "prod1", "app_name": "my-app"}},
	})
	if err != nil || len(evs) != 2 {
		t.Fatalf("evs=%+v err=%v", evs, err)
	}
	if evs[0].Status != core.StatusInProgress || evs[0].CommitSHA != "cafebabe1234" {
		t.Fatalf("unexpected pending build: %+v", evs[0])
	}
	if evs[1].Status != core.StatusSuccess || evs[1].Version != "v10" || evs[1].CommitSHA != "deadbeef1234" {
		t.Fatalf("unexpected release: %+v", evs[1])
	}

	id, err := p.ConnectWebhook(context.Background(), map[string]any{"api_token": "tok"}, map[string]any{"app_name": "my-app"}, "https://api.example.com/hook", "sec")
	if err != nil || !createdHook || id != "wh-123" {
		t.Fatalf("connect webhook failed: id=%q created=%v err=%v", id, createdHook, err)
	}
}
