package netlify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

func signJWS(body []byte, secret string) string {
	enc := base64.RawURLEncoding.EncodeToString
	h := enc([]byte(`{"alg":"HS256","typ":"JWT"}`))
	sum := sha256.Sum256(body)
	p := enc([]byte(`{"sha256":"` + hex.EncodeToString(sum[:]) + `"}`))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(h + "." + p))
	return h + "." + p + "." + enc(mac.Sum(nil))
}

func TestVerifyParseAndPoll(t *testing.T) {
	p := Plugin{}
	body := []byte(`{"id":"dep1","state":"ready","context":"production","branch":"main","commit_ref":"abcdef1234567","title":"Release 1.2"}`)
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("X-Webhook-Signature", signJWS(body, "mysec"))
	if err := p.VerifyWebhook(r, body, "mysec"); err != nil {
		t.Fatal(err)
	}
	ev, err := p.ParseWebhook(body, nil)
	if err != nil || ev.Status != core.StatusSuccess || ev.Version != "abcdef1" || ev.CommitSHA != "abcdef1234567" {
		t.Fatalf("unexpected webhook: %+v %v", ev, err)
	}

	orig := core.DefaultClient
	defer func() { core.DefaultClient = orig }()
	core.DefaultClient = &http.Client{
		Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`[{"id":"dep2","state":"building","context":"production","branch":"main","commit_ref":"9999999abcdef"}]`)),
			}, nil
		}),
	}
	evs, err := p.CheckStatus(context.Background(), map[string]any{
		"api_token": "tok",
		"targets":   []any{map[string]any{"product": "p1", "site_id": "site-1"}},
	})
	if err != nil || len(evs) != 1 || evs[0].Status != core.StatusInProgress || evs[0].Progress != 50 {
		t.Fatalf("unexpected poll: %+v %v", evs, err)
	}
}
