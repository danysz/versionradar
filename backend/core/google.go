package core

import (
	"context"
	"net/http"

	"golang.org/x/oauth2/google"
)

// GoogleClient returns an HTTP client authenticated as the service account in cfg["service_account_json"].
func GoogleClient(ctx context.Context, cfg map[string]any, scope string) (*http.Client, error) {
	key, err := JSONString(cfg["service_account_json"])
	if err != nil {
		return nil, err
	}
	jc, err := google.JWTConfigFromJSON([]byte(key), scope)
	if err != nil {
		return nil, err
	}
	return jc.Client(ctx), nil
}
