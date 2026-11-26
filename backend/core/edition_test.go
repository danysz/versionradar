package core

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestVerifyLicenseKey(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(LicenseClaims{
		Org:       "Acme Corp",
		Plan:      "pro",
		Seats:     10,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	sig := ed25519.Sign(priv, payload)
	key := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sig)

	claims, err := VerifyLicenseKey(key, pub)
	if err != nil || claims.Org != "Acme Corp" || claims.Plan != "pro" || claims.Seats != 10 {
		t.Fatalf("unexpected claims: %+v err=%v", claims, err)
	}
	if _, err := VerifyLicenseKey(key+"x", pub); err == nil {
		t.Fatal("tampered key accepted")
	}
}

func TestNotifierMatchesEvent(t *testing.T) {
	if !notifierMatchesEvent(map[string]any{}, "deployment_failed") {
		t.Fatal("empty filter should match all")
	}
	if !notifierMatchesEvent(map[string]any{"events": "failed,outdated"}, "deployment_failed") {
		t.Fatal("failed filter should match deployment_failed")
	}
	if notifierMatchesEvent(map[string]any{"events": "failed"}, "deployment_succeeded") {
		t.Fatal("failed-only filter should not match deployment_succeeded")
	}
}
