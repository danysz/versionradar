package render

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

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVerifyParseAndPoll(t *testing.T) {
	p := Plugin{}
	body := []byte(`{"type":"deploy_ended","timestamp":"2026-02-01T00:00:00Z","data":{"id":"dep-1","status":"live","commit":{"id":"abcdef123456","message":"Ship render"}}}`)
	mac := hmac.New(sha256.New, []byte("sec"))
	mac.Write([]byte("msg_1.1700000000." + string(body)))
	sig := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("webhook-id", "msg_1")
	r.Header.Set("webhook-timestamp", "1700000000")
	r.Header.Set("webhook-signature", sig)
	if err := p.VerifyWebhook(r, body, "sec"); err != nil {
		t.Fatal(err)
	}
	ev, err := p.ParseWebhook(body, nil)
	if err != nil || ev.Status != core.StatusSuccess || ev.Version != "abcdef1" || ev.Stage != "live" {
		t.Fatalf("unexpected webhook: %+v %v", ev, err)
	}

	orig := core.DefaultClient
	defer func() { core.DefaultClient = orig }()
	core.DefaultClient = &http.Client{
		Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`[{"deploy":{"id":"dep-2","status":"build_in_progress","commit":{"id":"1234567890ab","message":"WIP"}}}]`)),
			}, nil
		}),
	}
	evs, err := p.CheckStatus(context.Background(), map[string]any{
		"api_token": "rnd_key",
		"targets":   []any{map[string]any{"product": "p1", "service_id": "srv-1"}},
	})
	if err != nil || len(evs) != 1 || evs[0].Status != core.StatusInProgress || evs[0].Progress != 45 {
		t.Fatalf("unexpected poll: %+v %v", evs, err)
	}
}
