package core

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
)

// Notification is the provider-neutral alert payload sent to NotifierPlugins (Slack, Discord, Telegram, etc.).
type Notification struct {
	Event         string // "deployment_failed", "deployment_succeeded", "deployment_started", "version_outdated"
	RadarID       string
	RadarName     string
	Platform      string
	Status        string
	Version       string
	LatestVersion string
	Stage         string
	Message       string
	Timestamp     time.Time
}

// NotifierPlugin sends outbound alerts when deployments finish/fail or drift from Git.
type NotifierPlugin interface {
	Name() string
	Version() string
	ConfigFields() []Field
	Notify(ctx context.Context, cfg map[string]any, n Notification) error
}

type NotifierRegistry struct{ notifiers map[string]NotifierPlugin }

func NewNotifierRegistry(ns ...NotifierPlugin) *NotifierRegistry {
	r := &NotifierRegistry{notifiers: map[string]NotifierPlugin{}}
	for _, n := range ns {
		r.notifiers[n.Name()] = n
	}
	return r
}

func (r *NotifierRegistry) Get(name string) (NotifierPlugin, bool) {
	if r == nil {
		return nil, false
	}
	n, ok := r.notifiers[name]
	return n, ok
}

func (r *NotifierRegistry) List() []NotifierPlugin {
	if r == nil {
		return nil
	}
	out := make([]NotifierPlugin, 0, len(r.notifiers))
	for _, n := range r.notifiers {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// notifyUser dispatches an alert to every enabled NotifierPlugin configured by userID.
func (s *Service) notifyUser(userID string, n Notification) {
	if s.Notifiers == nil || len(s.Notifiers.List()) == 0 {
		return
	}
	recs, err := s.App.FindAllRecords("user_plugins", dbx.HashExp{"user": userID, "is_enabled": true})
	if err != nil {
		return
	}
	for _, r := range recs {
		np, ok := s.Notifiers.Get(r.GetString("plugin_name"))
		if !ok {
			continue
		}
		cfg := s.decryptPluginConfig(r)
		if !notifierMatchesEvent(cfg, n.Event) {
			continue
		}
		go func(plugin NotifierPlugin, config map[string]any, alert Notification) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := plugin.Notify(ctx, config, alert); err != nil {
				s.App.Logger().Warn("notifier failed", "notifier", plugin.Name(), "radar", alert.RadarID, "err", err)
			}
		}(np, cfg, n)
	}
}

func notifierMatchesEvent(cfg map[string]any, event string) bool {
	filter := strings.TrimSpace(Str(cfg, "events"))
	if filter == "" || strings.EqualFold(filter, "all") {
		return true
	}
	for _, tok := range strings.Split(filter, ",") {
		t := strings.TrimSpace(strings.ToLower(tok))
		if t == "" {
			continue
		}
		if t == event || strings.Contains(event, t) {
			return true
		}
	}
	return false
}
