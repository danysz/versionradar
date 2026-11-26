package core

import (
	"context"
	"errors"
	"net/http"
	"time"
)

const (
	StatusSuccess    = "success"
	StatusFailed     = "failed"
	StatusInProgress = "in_progress"
)

// ErrNotImplemented is returned by plugin stubs.
var ErrNotImplemented = errors.New("not implemented")

// DeploymentEvent is the provider-neutral shape every plugin parses into.
type DeploymentEvent struct {
	ProductID  string // set by pollers from the config target; webhooks get it from ?product=
	ExternalID string // provider-side id; lets repeated events/polls update one row instead of adding rows
	Platform   string
	Status     string // one of the Status* constants
	Version    string
	CommitSHA  string // optional git commit SHA for accurate comparison with Git sources
	Stage      string // optional build/release stage (e.g. "queued", "build", "deploy", "release")
	Progress   int    // optional 0-100 build/release progress percentage
	Message    string
	Timestamp  time.Time // zero means "now"
}

// ProviderPlugin is implemented once per provider (cloudflare, gcp, googleplay, appstore, firebase, heroku).
// cfg is the user's decrypted config_json; secret is the per-user webhook secret.
type ProviderPlugin interface {
	Name() string
	// Version is the plugin's semver, exposed by GET /api/v1/plugins and pinned per user.
	Version() string
	// VerifyWebhook authenticates the request the way the provider supports (header token, HMAC, JWT).
	VerifyWebhook(r *http.Request, body []byte, secret string) error
	ParseWebhook(body []byte, cfg map[string]any) (*DeploymentEvent, error)
	// CheckStatus polls the provider with the user's read-only credentials. Each target in
	// cfg["targets"] is one radar (its "product" is the radar id); returned events carry that ProductID.
	CheckStatus(ctx context.Context, cfg map[string]any) ([]DeploymentEvent, error)
	// ConfigFields are the credential inputs a user fills in (stored together as config_json).
	ConfigFields() []Field
	// TargetFields describe a radar's target: what to watch (non-secret), e.g. a Pages project.
	TargetFields() []Field
}

// WebhookStatus describes how a plugin's webhook can be connected (automatically or via fallback CLI/manual steps).
type WebhookStatus struct {
	Plugin      string `json:"plugin"`
	RadarID     string `json:"radar_id,omitempty"`
	RadarName   string `json:"radar_name,omitempty"`
	Kind        string `json:"kind,omitempty"` // "provider" or "source"
	Supported   bool   `json:"supported"`
	Automatable bool   `json:"automatable"`
	Connected   bool   `json:"connected"`
	WebhookURL  string `json:"webhook_url,omitempty"`
	ExternalID  string `json:"external_id,omitempty"`
	CLICommand  string `json:"cli_command,omitempty"`
	ManualSteps string `json:"manual_steps,omitempty"`
	Note        string `json:"note,omitempty"`
	Error       string `json:"error,omitempty"`
}

// WebhookAutomator is an optional interface implemented by plugins that support webhooks
// (either automated API creation, fallback CLI/manual instructions, or both).
type WebhookAutomator interface {
	// WebhookPlan returns the connection plan (URL, fallback CLI command, manual instructions).
	WebhookPlan(webhookURL, secret string, target map[string]any) WebhookStatus
	// ConnectWebhook creates or updates the webhook at the provider via its API.
	// Returns the provider-side webhook/subscription ID.
	ConnectWebhook(ctx context.Context, cfg, target map[string]any, webhookURL, secret string) (string, error)
}

// Field is one input in the plugin forms. JSON fields are pasted as JSON text and stored as an object.
type Field struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Help      string   `json:"help,omitempty"`
	Secret    bool     `json:"secret,omitempty"`
	Multiline bool     `json:"multiline,omitempty"`
	JSON      bool     `json:"json,omitempty"`
	Optional  bool     `json:"optional,omitempty"`
	Default   string   `json:"default,omitempty"`
	Options   []string `json:"options,omitempty"`
}

// Stub can be embedded by plugins that are not built yet or don't support webhooks.
type Stub struct{}

func (Stub) VerifyWebhook(*http.Request, []byte, string) error { return ErrNotImplemented }
func (Stub) ParseWebhook([]byte, map[string]any) (*DeploymentEvent, error) {
	return nil, ErrNotImplemented
}
func (Stub) CheckStatus(context.Context, map[string]any) ([]DeploymentEvent, error) {
	return nil, ErrNotImplemented
}
func (Stub) ConfigFields() []Field { return nil }
func (Stub) TargetFields() []Field { return nil }

// Targets returns cfg["targets"] as a list of maps (each has at least "product").
func Targets(cfg map[string]any) []map[string]any {
	var out []map[string]any
	list, _ := cfg["targets"].([]any)
	for _, t := range list {
		if m, ok := t.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// Str reads a string field from a config/target map.
func Str(m map[string]any, k string) string { s, _ := m[k].(string); return s }
