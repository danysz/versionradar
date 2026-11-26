package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var DefaultClient = &http.Client{Timeout: 20 * time.Second}

// Do sends a request without a body and decodes a JSON response into out (nil to discard).
func Do(ctx context.Context, c *http.Client, method, url string, hdr map[string]string, out any) error {
	return DoWithBody(ctx, c, method, url, hdr, nil, out)
}

// DoWithBody sends a request (encoding reqBody as JSON when non-nil) and decodes a JSON response into out.
func DoWithBody(ctx context.Context, c *http.Client, method, url string, hdr map[string]string, reqBody, out any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: HTTP %d", method, req.URL.Host, resp.StatusCode) // no body: may echo credentials
	}
	if out == nil || len(body) == 0 {
		return nil
	}
	return json.Unmarshal(body, out)
}

// DoRaw sends a request and returns the raw response bytes.
func DoRaw(ctx context.Context, c *http.Client, method, url string, hdr map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s: HTTP %d", method, req.URL.Host, resp.StatusCode)
	}
	return body, nil
}

// JSONString accepts a config value that is either a JSON string or a JSON object and returns it as text.
func JSONString(v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	if v == nil {
		return "", fmt.Errorf("missing credentials")
	}
	b, err := json.Marshal(v)
	return string(b), err
}
