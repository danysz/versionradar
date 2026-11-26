package gcp

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"
)

func TestVerifyAndParse(t *testing.T) {
	r := httptest.NewRequest("POST", "/?token=s", nil)
	if err := (Plugin{}).VerifyWebhook(r, nil, "s"); err != nil {
		t.Fatal(err)
	}
	if (Plugin{}).VerifyWebhook(httptest.NewRequest("POST", "/", nil), nil, "s") == nil {
		t.Fatal("no token accepted")
	}
	data := base64.StdEncoding.EncodeToString([]byte(`{"id":"b1","status":"SUCCESS","substitutions":{"TAG_NAME":"v1.2"}}`))
	ev, err := (Plugin{}).ParseWebhook([]byte(`{"message":{"data":"`+data+`"}}`), nil)
	if err != nil || ev.Status != "success" || ev.Version != "v1.2" {
		t.Fatalf("%+v %v", ev, err)
	}
}
