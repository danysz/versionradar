package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

func (s *Service) basePublicURL(e *core.RequestEvent) string {
	if s.PublicURL != "" {
		return strings.TrimRight(s.PublicURL, "/")
	}
	if env := strings.TrimSpace(os.Getenv("VR_PUBLIC_URL")); env != "" {
		return strings.TrimRight(env, "/")
	}
	proto := e.Request.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if e.Request.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := e.Request.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = e.Request.Host
	}
	return fmt.Sprintf("%s://%s", proto, host)
}

// ensureUserPlugin fetches or creates a user_plugins record so the user always has a webhook_secret.
func (s *Service) ensureUserPlugin(userID, pluginName string) (*core.Record, error) {
	up, err := s.App.FindFirstRecordByFilter("user_plugins", "user = {:u} && plugin_name = {:n}",
		map[string]any{"u": userID, "n": pluginName})
	if err == nil {
		return up, nil
	}
	col, err := s.App.FindCollectionByNameOrId("user_plugins")
	if err != nil {
		return nil, err
	}
	up = core.NewRecord(col)
	up.Set("user", userID)
	up.Set("plugin_name", pluginName)
	up.Set("is_enabled", true)
	if err := s.App.Save(up); err != nil {
		return nil, err
	}
	return up, nil
}

func (s *Service) automatorFor(kind, pluginName string) (WebhookAutomator, bool) {
	if kind == "source" {
		if sp, ok := s.Sources.Get(pluginName); ok {
			wa, ok := sp.(WebhookAutomator)
			return wa, ok
		}
		return nil, false
	}
	if p, ok := s.Registry.Get(pluginName); ok {
		wa, ok := p.(WebhookAutomator)
		return wa, ok
	}
	return nil, false
}

func (s *Service) planForRadar(baseURL, userID string, radar *core.Record, kind string) (*WebhookStatus, error) {
	pluginName := radar.GetString("plugin_name")
	rawTarget := radar.GetString("target")
	connected := radar.GetBool("webhook_connected")
	extID := radar.GetString("webhook_external_id")
	if kind == "source" {
		pluginName = radar.GetString("source_plugin")
		rawTarget = radar.GetString("source_target")
		connected = radar.GetBool("source_webhook_connected")
		extID = radar.GetString("source_webhook_external_id")
	}
	if pluginName == "" {
		return nil, nil
	}
	target := map[string]any{}
	if rawTarget != "" {
		_ = json.Unmarshal([]byte(rawTarget), &target)
	}
	target["product"] = radar.Id

	secret := ""
	if up, err := s.ensureUserPlugin(userID, pluginName); err == nil {
		secret = up.GetString("webhook_secret")
	}
	webhookURL := fmt.Sprintf("%s%s/webhooks/%s/%s?product=%s", baseURL, APIPrefix, userID, pluginName, radar.Id)

	var st WebhookStatus
	if wa, ok := s.automatorFor(kind, pluginName); ok {
		st = wa.WebhookPlan(webhookURL, secret, target)
	} else {
		st = WebhookStatus{
			Plugin:     pluginName,
			Kind:       kind,
			Supported:  false,
			WebhookURL: webhookURL,
			Note:       "This plugin does not support webhooks.",
		}
	}
	st.Plugin = pluginName
	st.Kind = kind
	st.RadarID = radar.Id
	st.RadarName = radar.GetString("name")
	st.Connected = connected
	st.ExternalID = extID
	if st.WebhookURL == "" && st.Supported {
		st.WebhookURL = webhookURL
	}
	return &st, nil
}

func buildFallbackScript(plans []WebhookStatus) string {
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\n")
	b.WriteString("# VersionRadar fallback webhook setup script.\n")
	b.WriteString("# Run any section below if one-click API automation could not connect (e.g. read-only token).\n")
	b.WriteString("set -euo pipefail\n\n")
	count := 0
	for _, p := range plans {
		if !p.Supported || p.CLICommand == "" {
			continue
		}
		count++
		fmt.Fprintf(&b, "# --- %s (%s / %s) ---\n", p.RadarName, p.Plugin, p.Kind)
		b.WriteString(p.CLICommand)
		b.WriteString("\n\n")
	}
	if count == 0 {
		return "# No webhook-capable radars configured yet.\n"
	}
	return b.String()
}

// webhookPlans: GET /api/v1/webhooks/plans
// Returns webhook status, CLI fallback commands, manual steps, and a combined setup script for all of the user's radars.
func (s *Service) webhookPlans(e *core.RequestEvent) error {
	radars, err := s.App.FindAllRecords("products", dbx.HashExp{"user": e.Auth.Id})
	if err != nil {
		return err
	}
	baseURL := s.basePublicURL(e)
	plans := []WebhookStatus{}
	for _, r := range radars {
		if st, err := s.planForRadar(baseURL, e.Auth.Id, r, "provider"); err == nil && st != nil {
			plans = append(plans, *st)
		}
		if st, err := s.planForRadar(baseURL, e.Auth.Id, r, "source"); err == nil && st != nil {
			plans = append(plans, *st)
		}
	}
	return e.JSON(http.StatusOK, map[string]any{
		"plans":  plans,
		"script": buildFallbackScript(plans),
	})
}

func (s *Service) doConnectWebhook(ctx context.Context, baseURL, userID string, radar *core.Record, kind string, overrideCfg map[string]any) WebhookStatus {
	stPtr, _ := s.planForRadar(baseURL, userID, radar, kind)
	if stPtr == nil {
		return WebhookStatus{RadarID: radar.Id, RadarName: radar.GetString("name"), Kind: kind, Error: "no plugin configured"}
	}
	st := *stPtr
	if !st.Supported || !st.Automatable {
		return st
	}
	pluginName := st.Plugin
	wa, ok := s.automatorFor(kind, pluginName)
	if !ok {
		st.Error = "webhook automation not supported by plugin"
		return st
	}
	up, err := s.ensureUserPlugin(userID, pluginName)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	cfg := s.decryptPluginConfig(up)
	for k, v := range overrideCfg {
		if v != nil && v != "" {
			cfg[k] = v
		}
	}
	rawTarget := radar.GetString("target")
	if kind == "source" {
		rawTarget = radar.GetString("source_target")
	}
	target := map[string]any{}
	if rawTarget != "" {
		_ = json.Unmarshal([]byte(rawTarget), &target)
	}
	target["product"] = radar.Id

	secret := up.GetString("webhook_secret")
	webhookURL := fmt.Sprintf("%s%s/webhooks/%s/%s?product=%s", baseURL, APIPrefix, userID, pluginName, radar.Id)

	extID, err := wa.ConnectWebhook(ctx, cfg, target, webhookURL, secret)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	if kind == "source" {
		radar.Set("source_webhook_connected", true)
		radar.Set("source_webhook_external_id", extID)
	} else {
		radar.Set("webhook_connected", true)
		radar.Set("webhook_external_id", extID)
	}
	_ = s.App.Save(radar)
	st.Connected = true
	st.ExternalID = extID
	return st
}

// connectRadarWebhook: POST /api/v1/radars/{radarId}/webhook/connect
// Body: {"kind": "provider"|"source", "override_config": {...}?}
func (s *Service) connectRadarWebhook(e *core.RequestEvent) error {
	radarID := e.Request.PathValue("radarId")
	radar, err := s.App.FindFirstRecordByFilter("products", "id = {:id} && user = {:u}",
		map[string]any{"id": radarID, "u": e.Auth.Id})
	if err != nil {
		return apis.NewNotFoundError("radar not found", nil)
	}
	var body struct {
		Kind           string         `json:"kind"`
		OverrideConfig map[string]any `json:"override_config"`
	}
	_ = e.BindBody(&body)
	if body.Kind == "" {
		body.Kind = "provider"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	st := s.doConnectWebhook(ctx, s.basePublicURL(e), e.Auth.Id, radar, body.Kind, body.OverrideConfig)
	return e.JSON(http.StatusOK, st)
}

// connectAllWebhooks: POST /api/v1/webhooks/connect-all
// Body: {"force": bool?, "overrides": {"cloudflare": {...}, ...}?}
func (s *Service) connectAllWebhooks(e *core.RequestEvent) error {
	var body struct {
		Force     bool                      `json:"force"`
		Overrides map[string]map[string]any `json:"overrides"`
	}
	_ = e.BindBody(&body)

	radars, err := s.App.FindAllRecords("products", dbx.HashExp{"user": e.Auth.Id})
	if err != nil {
		return err
	}
	baseURL := s.basePublicURL(e)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	results := []WebhookStatus{}
	connectedCount := 0
	for _, r := range radars {
		for _, kind := range []string{"provider", "source"} {
			pluginName := r.GetString("plugin_name")
			already := r.GetBool("webhook_connected")
			if kind == "source" {
				pluginName = r.GetString("source_plugin")
				already = r.GetBool("source_webhook_connected")
			}
			if pluginName == "" {
				continue
			}
			if already && !body.Force {
				if st, err := s.planForRadar(baseURL, e.Auth.Id, r, kind); err == nil && st != nil {
					results = append(results, *st)
					connectedCount++
				}
				continue
			}
			var ov map[string]any
			if body.Overrides != nil {
				ov = body.Overrides[pluginName]
			}
			st := s.doConnectWebhook(ctx, baseURL, e.Auth.Id, r, kind, ov)
			if st.Connected {
				connectedCount++
			}
			results = append(results, st)
		}
	}
	return e.JSON(http.StatusOK, map[string]any{
		"connected": connectedCount,
		"results":   results,
		"script":    buildFallbackScript(results),
	})
}
