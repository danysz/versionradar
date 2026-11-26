package core

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

var errNoCreds = errors.New("credentials not set: open Plugins and fill them in")

// SyncAll polls every enabled plugin of every user AND updates Git version sources for all configured radars.
func (s *Service) SyncAll() {
	recs, err := s.App.FindAllRecords("user_plugins", nil)
	if err != nil {
		s.App.Logger().Error("sync: list user_plugins", "err", err)
	} else {
		for _, r := range recs {
			if r.GetBool("is_enabled") {
				if _, ok := s.Registry.Get(r.GetString("plugin_name")); !ok {
					continue
				}
				if _, err := s.syncOne(r); err != nil && !errors.Is(err, errNoCreds) {
					s.App.Logger().Warn("sync failed", "user", r.GetString("user"), "plugin", r.GetString("plugin_name"), "err", err)
				}
			}
		}
	}

	radars, err := s.App.FindAllRecords("products", nil)
	if err != nil {
		s.App.Logger().Error("sync: list products", "err", err)
		return
	}
	for _, r := range radars {
		if r.GetString("source_plugin") != "" && r.GetString("version_strategy") != "" {
			if err := s.SyncRadarSource(r); err != nil {
				s.App.Logger().Warn("source sync failed", "radar", r.Id, "source", r.GetString("source_plugin"), "err", err)
			}
		}
	}
}

// SyncUser polls the enabled deployment plugins and Git sources for one user.
func (s *Service) SyncUser(userID string) (int, int, []string, error) {
	recs, err := s.App.FindAllRecords("user_plugins", nil)
	if err != nil {
		return 0, 0, nil, err
	}
	total := 0
	problems := []string{}
	for _, r := range recs {
		if r.GetString("user") != userID || !r.GetBool("is_enabled") {
			continue
		}
		if _, ok := s.Registry.Get(r.GetString("plugin_name")); !ok {
			continue
		}
		n, err := s.syncOne(r)
		if err != nil {
			s.App.Logger().Warn("sync failed", "user", userID, "plugin", r.GetString("plugin_name"), "err", err)
			problems = append(problems, r.GetString("plugin_name")+": "+err.Error())
		}
		total += n
	}

	radars, err := s.App.FindAllRecords("products", dbx.HashExp{"user": userID})
	if err != nil {
		return total, 0, problems, err
	}
	sourcesChecked := 0
	for _, r := range radars {
		if r.GetString("source_plugin") == "" || r.GetString("version_strategy") == "" {
			continue
		}
		if err := s.SyncRadarSource(r); err != nil {
			s.App.Logger().Warn("source sync failed", "user", userID, "radar", r.Id, "err", err)
			problems = append(problems, r.GetString("name")+" (git): "+err.Error())
		} else {
			sourcesChecked++
		}
	}
	return total, sourcesChecked, problems, nil
}

// SyncRadarSource resolves the latest expected version from the radar's Git source + VersionStrategy
// and saves it back onto the radar record (pushing a realtime update to connected clients).
func (s *Service) SyncRadarSource(radar *core.Record) error {
	srcName := radar.GetString("source_plugin")
	stratName := radar.GetString("version_strategy")
	if srcName == "" || stratName == "" {
		return nil
	}
	sp, ok := s.Sources.Get(srcName)
	if !ok {
		return errors.New("unknown source plugin: " + srcName)
	}
	strat, ok := s.Strategies.Get(stratName)
	if !ok {
		return errors.New("unknown version strategy: " + stratName)
	}

	// Credentials are optional for public Git repositories; if configured in user_plugins, decrypt and pass them.
	cfg := map[string]any{}
	if up, err := s.App.FindFirstRecordByFilter("user_plugins", "user = {:u} && plugin_name = {:n}",
		map[string]any{"u": radar.GetString("user"), "n": srcName}); err == nil {
		if !up.GetBool("is_enabled") && up.GetString("config_json") != "" {
			return nil
		}
		cfg = s.decryptPluginConfig(up)
	}

	srcTarget := map[string]any{}
	if raw := radar.GetString("source_target"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &srcTarget); err != nil {
			return err
		}
	}
	verCfg := map[string]any{}
	if raw := radar.GetString("version_config"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &verCfg); err != nil {
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repo, err := sp.OpenRepo(ctx, cfg, srcTarget)
	if err != nil {
		radar.Set("source_error", err.Error())
		radar.Set("latest_checked_at", types.NowDateTime())
		_ = s.App.Save(radar)
		return err
	}
	info, err := strat.Resolve(ctx, repo, verCfg)
	if err != nil {
		radar.Set("source_error", err.Error())
		radar.Set("latest_checked_at", types.NowDateTime())
		_ = s.App.Save(radar)
		return err
	}
	radar.Set("latest_version", info.Version)
	radar.Set("latest_commit", info.CommitSHA)
	radar.Set("latest_ref", info.Ref)
	radar.Set("latest_checked_at", types.NowDateTime())
	radar.Set("source_error", "")
	return s.App.Save(radar)
}

// SyncActiveBuilds polls only radars that currently have an in_progress deployment or whose
// latest Git commit has not yet appeared in their newest deployment.
func (s *Service) SyncActiveBuilds() (int, error) {
	activeRadars := map[string]*core.Record{}

	inProgDeps, err := s.App.FindRecordsByFilter(
		"deployments",
		"status = {:s}",
		"",
		0,
		0,
		dbx.Params{"s": StatusInProgress},
	)
	if err != nil {
		return 0, err
	}
	for _, d := range inProgDeps {
		pid := d.GetString("product")
		if pid == "" {
			continue
		}
		if _, exists := activeRadars[pid]; exists {
			continue
		}
		if r, rerr := s.App.FindRecordById("products", pid); rerr == nil {
			activeRadars[pid] = r
		}
	}

	radarsWithCommit, err := s.App.FindRecordsByFilter("products", "latest_commit != ''", "", 0, 0)
	if err != nil {
		return 0, err
	}
	for _, r := range radarsWithCommit {
		if _, exists := activeRadars[r.Id]; exists {
			continue
		}
		if r.GetString("plugin_name") == "" {
			continue
		}
		latestCommit := r.GetString("latest_commit")
		deps, derr := s.App.FindRecordsByFilter(
			"deployments",
			"product = {:p}",
			"-timestamp",
			1,
			0,
			dbx.Params{"p": r.Id},
		)
		if derr != nil {
			continue
		}
		if len(deps) == 0 {
			activeRadars[r.Id] = r
			continue
		}
		dep := deps[0]
		depCommit := dep.GetString("commit_sha")
		if commitMatches(depCommit, latestCommit) {
			continue
		}
		if depCommit == "" && VersionsMatch(dep.GetString("version"), "", r.GetString("latest_version"), latestCommit) {
			continue
		}
		activeRadars[r.Id] = r
	}

	if len(activeRadars) == 0 {
		return 0, nil
	}

	type groupKey struct {
		user   string
		plugin string
	}
	groups := map[groupKey][]*core.Record{}
	order := []groupKey{}
	for _, r := range activeRadars {
		pn := r.GetString("plugin_name")
		uid := r.GetString("user")
		if pn == "" || uid == "" {
			continue
		}
		if _, ok := s.Registry.Get(pn); !ok {
			continue
		}
		k := groupKey{user: uid, plugin: pn}
		if _, exists := groups[k]; !exists {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}

	total := 0
	var firstErr error
	for _, k := range order {
		up, uerr := s.App.FindFirstRecordByFilter(
			"user_plugins",
			"user = {:u} && plugin_name = {:n} && is_enabled = true",
			dbx.Params{"u": k.user, "n": k.plugin},
		)
		if uerr != nil {
			continue
		}
		n, perr := s.pollPluginRadars(up, groups[k])
		total += n
		if perr != nil && !errors.Is(perr, errNoCreds) {
			s.App.Logger().Warn("active build sync failed", "user", k.user, "plugin", k.plugin, "err", perr)
			if firstErr == nil {
				firstErr = perr
			}
		}
	}
	return total, firstErr
}

func commitMatches(depCommit, latestCommit string) bool {
	if depCommit == "" || latestCommit == "" {
		return false
	}
	return depCommit == latestCommit || shaPrefixMatch(depCommit, latestCommit)
}

// StartAdaptiveWatcher runs a ticker on the given interval (defaulting to 10s when <= 0)
// that polls active builds via SyncActiveBuilds until stopCh is closed.
func (s *Service) StartAdaptiveWatcher(stopCh <-chan struct{}, interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			if _, err := s.SyncActiveBuilds(); err != nil {
				s.App.Logger().Warn("adaptive watcher sync failed", "err", err)
			}
		}
	}
}

func (s *Service) syncOne(up *core.Record) (int, error) {
	if _, ok := s.Registry.Get(up.GetString("plugin_name")); !ok {
		return 0, nil
	}
	// Targets are the user's radars for this plugin: each radar's target fields plus its id as "product".
	radars, err := s.App.FindAllRecords("products", dbx.HashExp{"user": up.GetString("user"), "plugin_name": up.GetString("plugin_name")})
	if err != nil {
		return 0, err
	}
	return s.pollPluginRadars(up, radars)
}

func (s *Service) pollPluginRadars(up *core.Record, radars []*core.Record) (int, error) {
	p, ok := s.Registry.Get(up.GetString("plugin_name"))
	if !ok {
		return 0, nil
	}
	if up.GetString("config_json") == "" {
		return 0, errNoCreds
	}
	plain, err := s.Cipher.Decrypt(up.GetString("config_json"))
	if err != nil {
		return 0, err
	}
	cfg := map[string]any{}
	if err := json.Unmarshal([]byte(plain), &cfg); err != nil {
		return 0, err
	}
	targets := []any{}
	for _, r := range radars {
		t := map[string]any{}
		if raw := r.GetString("target"); raw != "" {
			if json.Unmarshal([]byte(raw), &t) != nil {
				continue
			}
		}
		t["product"] = r.Id
		targets = append(targets, t)
	}
	if len(targets) == 0 {
		return 0, nil // plugin enabled but no radar uses it: nothing to poll
	}
	cfg["targets"] = targets
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	evs, err := p.CheckStatus(ctx, cfg)
	n := 0
	for i := range evs {
		// Targets come from user-controlled config: only write into products the user owns (matched by id or name).
		prod, perr := s.App.FindFirstRecordByFilter("products", "(id = {:p} || name = {:p}) && user = {:u}",
			map[string]any{"p": evs[i].ProductID, "u": up.GetString("user")})
		if perr != nil {
			continue
		}
		if derr := s.Dispatch(prod.Id, &evs[i]); derr != nil {
			return n, derr
		}
		n++
	}
	return n, err
}
