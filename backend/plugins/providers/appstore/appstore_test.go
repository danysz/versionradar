package appstore

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyAndParse(t *testing.T) {
	body := []byte(`{"data":{"type":"x","attributes":{"newValue":"REJECTED","appVersionId":"v1"}}}`)
	m := hmac.New(sha256.New, []byte("s"))
	m.Write(body)
	r := httptest.NewRequest("POST", "/", nil)
	if (Plugin{}).VerifyWebhook(r, body, "s") == nil {
		t.Fatal("unsigned accepted")
	}
	r.Header.Set("X-Apple-Signature", "hmacsha256="+hex.EncodeToString(m.Sum(nil)))
	if err := (Plugin{}).VerifyWebhook(r, body, "s"); err != nil {
		t.Fatal(err)
	}
	if ev, _ := (Plugin{}).ParseWebhook(body, nil); ev.Status != "failed" || ev.Version != "v1" {
		t.Fatalf("%+v", ev)
	}
}

func TestTokenSignature(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	tok, err := token("iss", "kid", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if len(sig) != 64 || !ecdsa.Verify(&key.PublicKey, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("bad JWT signature")
	}
}
