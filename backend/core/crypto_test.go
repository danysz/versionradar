package core

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCipherRoundTrip(t *testing.T) {
	c, err := NewCipher(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err != nil {
		t.Fatal(err)
	}
	enc, _ := c.Encrypt(`{"token":"x"}`)
	if !IsEncrypted(enc) || strings.Contains(enc, "token") {
		t.Fatal("not encrypted")
	}
	if got, err := c.Decrypt(enc); err != nil || got != `{"token":"x"}` {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := NewCipher("short"); err == nil {
		t.Fatal("bad key accepted")
	}
}
