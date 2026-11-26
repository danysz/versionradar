package core

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	pbcore "github.com/pocketbase/pocketbase/core"
)

const (
	TierCommunity = "community"
	TierPro       = "pro"
)

// Tiered can be implemented by any ProviderPlugin, SourcePlugin, VersionStrategy, or NotifierPlugin
// to declare whether it belongs to the Community (open-source) or Pro (paid) edition.
// Plugins that do not implement Tiered default to TierCommunity.
type Tiered interface {
	Tier() string
}

// PluginTier returns a plugin's declared tier (defaulting to TierCommunity).
func PluginTier(p any) string {
	if t, ok := p.(Tiered); ok && t.Tier() != "" {
		return t.Tier()
	}
	return TierCommunity
}

// EntitlementChecker gates Pro plugins and features (e.g. Stripe SaaS plans or offline Ed25519 license keys).
// Community edition defaults to CommunityEntitlements; the private Pro repo injects its own implementation.
type EntitlementChecker interface {
	Edition() string // "community" or "pro"
	CanUsePlugin(user *pbcore.Record, pluginName, tier string) bool
	CanAutoConnectWebhook(user *pbcore.Record) bool
	MaxRadars(user *pbcore.Record) int // 0 = unlimited
}

// CommunityEntitlements allows all community plugins and unlimited self-hosted radars.
type CommunityEntitlements struct{}

func (CommunityEntitlements) Edition() string { return TierCommunity }
func (CommunityEntitlements) CanUsePlugin(_ *pbcore.Record, _ string, tier string) bool {
	return tier != TierPro
}
func (CommunityEntitlements) CanAutoConnectWebhook(*pbcore.Record) bool { return true }
func (CommunityEntitlements) MaxRadars(*pbcore.Record) int              { return 0 }

// LicenseClaims is the payload signed into an offline self-hosted Pro license key (VR_LICENSE_KEY).
type LicenseClaims struct {
	Org       string    `json:"org"`
	Plan      string    `json:"plan"` // e.g. "pro", "team", "enterprise"
	Seats     int       `json:"seats"`
	ExpiresAt time.Time `json:"expires_at"`
}

// VerifyLicenseKey verifies a "<base64url-payload>.<base64url-ed25519-sig>" offline license key.
func VerifyLicenseKey(key string, pub ed25519.PublicKey) (*LicenseClaims, error) {
	if len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("invalid license public key")
	}
	payloadB64, sigB64, ok := strings.Cut(strings.TrimSpace(key), ".")
	if !ok {
		return nil, errors.New("malformed license key")
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, errors.New("invalid license payload encoding")
	}
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, errors.New("invalid license signature encoding")
	}
	if !ed25519.Verify(pub, payload, sig) {
		return nil, errors.New("invalid license signature")
	}
	var claims LicenseClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	if !claims.ExpiresAt.IsZero() && time.Now().After(claims.ExpiresAt) {
		return nil, errors.New("license expired")
	}
	return &claims, nil
}
