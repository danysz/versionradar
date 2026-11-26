package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	APIPrefix   = "/api/v1"
	maxBodySize = 1 << 20
)

type Service struct {
	App          core.App
	Registry     *Registry
	Sources      *SourceRegistry
	Strategies   *StrategyRegistry
	Notifiers    *NotifierRegistry
	Entitlements EntitlementChecker
	Cipher       *Cipher
	PublicURL    string // optional override (env VR_PUBLIC_URL)
}

func (s *Service) entitlements() EntitlementChecker {
	if s.Entitlements != nil {
		return s.Entitlements
	}
	return CommunityEntitlements{}
}

// Bind registers the v1 routes and the products / user_plugins lifecycle hooks.
func (s *Service) Bind(app core.App) {
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		if err := s.ensureAdmin(); err != nil {
			return err
		}
		if err := s.ensureAuth(); err != nil {
			return err
		}
		g := se.Router.Group(APIPrefix)
		g.GET("/plugins", s.listPlugins)
		g.POST("/webhooks/{userId}/{pluginName}", s.handleWebhook)
		// Credentials go through here, not the records API: config_json is a hidden field and PocketBase
		// refuses client writes to hidden fields.
		g.POST("/plugins/{pluginName}/config", s.saveConfig).Bind(apis.RequireAuth("users"))
		// Manual poll for both deployment providers and Git version sources.
		g.POST("/sync", func(e *core.RequestEvent) error {
			n, srcN, problems, err := s.SyncUser(e.Auth.Id)
			if err != nil {
				return err
			}
			return e.JSON(http.StatusOK, map[string]any{
				"events":   n,
				"sources":  srcN,
				"problems": problems,
			})
		}).Bind(apis.RequireAuth("users"))
		// Webhook automation endpoints (one-click connect + fallback CLI script plans).
		g.GET("/webhooks/plans", s.webhookPlans).Bind(apis.RequireAuth("users"))
		g.POST("/webhooks/connect-all", s.connectAllWebhooks).Bind(apis.RequireAuth("users"))
		g.POST("/radars/{radarId}/webhook/connect", s.connectRadarWebhook).Bind(apis.RequireAuth("users"))
		return se.Next()
	})

	// Fixed 5-minute poll for providers and Git sources.
	app.Cron().MustAdd("vr-poll", "*/5 * * * *", s.SyncAll)

	// A product is a "radar" when plugin_name is set: it follows the provider resource described by target,
	// and optionally checks the latest expected version from a Git source + VersionStrategy plugin.
	validateRadar := func(e *core.RecordEvent) error {
		if n := e.Record.GetString("plugin_name"); n != "" {
			if _, ok := s.Registry.Get(n); !ok {
				return apis.NewBadRequestError("unknown plugin", nil)
			}
		}
		if t := e.Record.GetString("target"); t != "" {
			var m map[string]any
			if json.Unmarshal([]byte(t), &m) != nil {
				return apis.NewBadRequestError("target must be a JSON object", nil)
			}
		}
		if sp := e.Record.GetString("source_plugin"); sp != "" {
			if _, ok := s.Sources.Get(sp); !ok {
				return apis.NewBadRequestError("unknown source plugin", nil)
			}
		}
		if st := e.Record.GetString("source_target"); st != "" {
			var m map[string]any
			if json.Unmarshal([]byte(st), &m) != nil {
				return apis.NewBadRequestError("source_target must be a JSON object", nil)
			}
		}
		if vs := e.Record.GetString("version_strategy"); vs != "" {
			if _, ok := s.Strategies.Get(vs); !ok {
				return apis.NewBadRequestError("unknown version strategy", nil)
			}
		}
		if vc := e.Record.GetString("version_config"); vc != "" {
			var m map[string]any
			if json.Unmarshal([]byte(vc), &m) != nil {
				return apis.NewBadRequestError("version_config must be a JSON object", nil)
			}
		}
		return e.Next()
	}
	app.OnRecordCreate("products").BindFunc(validateRadar)
	app.OnRecordUpdate("products").BindFunc(validateRadar)

	app.OnRecordCreate("user_plugins").BindFunc(func(e *core.RecordEvent) error {
		name := e.Record.GetString("plugin_name")
		ver, ok := s.pluginVersion(name)
		if !ok {
			return apis.NewBadRequestError("unknown plugin", nil)
		}
		if e.Record.GetString("webhook_secret") == "" {
			b := make([]byte, 24)
			rand.Read(b)
			e.Record.Set("webhook_secret", hex.EncodeToString(b))
		}
		e.Record.Set("plugin_version", ver)
		return s.encryptConfig(e.Record, e.Next)
	})
	app.OnRecordUpdate("user_plugins").BindFunc(func(e *core.RecordEvent) error {
		return s.encryptConfig(e.Record, e.Next)
	})
}

func (s *Service) pluginVersion(name string) (string, bool) {
	if p, ok := s.Registry.Get(name); ok {
		return p.Version(), true
	}
	if sp, ok := s.Sources.Get(name); ok {
		return sp.Version(), true
	}
	if np, ok := s.Notifiers.Get(name); ok {
		return np.Version(), true
	}
	return "", false
}

func (s *Service) pluginConfigFields(name string) ([]Field, string, bool) {
	if p, ok := s.Registry.Get(name); ok {
		return p.ConfigFields(), PluginTier(p), true
	}
	if sp, ok := s.Sources.Get(name); ok {
		return sp.ConfigFields(), PluginTier(sp), true
	}
	if np, ok := s.Notifiers.Get(name); ok {
		return np.ConfigFields(), PluginTier(np), true
	}
	return nil, "", false
}

// encryptConfig encrypts plaintext config_json before it reaches SQLite.
func (s *Service) encryptConfig(r *core.Record, next func() error) error {
	if v := r.GetString("config_json"); v != "" && !IsEncrypted(v) {
		if !json.Valid([]byte(v)) {
			return apis.NewBadRequestError("config_json must be valid JSON", nil)
		}
		enc, err := s.Cipher.Encrypt(v)
		if err != nil {
			return err
		}
		r.Set("config_json", enc)
	}
	return next()
}

func (s *Service) listPlugins(e *core.RequestEvent) error {
	type item struct {
		Name               string  `json:"name"`
		Kind               string  `json:"kind"`
		Tier               string  `json:"tier"`
		Locked             bool    `json:"locked,omitempty"`
		Version            string  `json:"version"`
		ConfigFields       []Field `json:"config_fields"`
		TargetFields       []Field `json:"target_fields,omitempty"`
		WebhookSupported   bool    `json:"webhook_supported"`
		WebhookAutomatable bool    `json:"webhook_automatable"`
		InstalledVersion   string  `json:"installed_version,omitempty"`
		Enabled            *bool   `json:"enabled,omitempty"`
	}
	type strategyItem struct {
		Name         string  `json:"name"`
		Tier         string  `json:"tier"`
		Label        string  `json:"label"`
		Description  string  `json:"description"`
		ConfigFields []Field `json:"config_fields"`
	}

	ent := s.entitlements()
	attachUserState := func(it *item) {
		it.Locked = !ent.CanUsePlugin(e.Auth, it.Name, it.Tier)
		if e.Auth == nil {
			return
		}
		if r, err := s.App.FindFirstRecordByFilter("user_plugins",
			"user = {:u} && plugin_name = {:n}", map[string]any{"u": e.Auth.Id, "n": it.Name}); err == nil {
			en := r.GetBool("is_enabled")
			it.InstalledVersion, it.Enabled = r.GetString("plugin_version"), &en
		}
	}

	items := []item{}
	for _, p := range s.Registry.List() {
		it := item{
			Name:         p.Name(),
			Kind:         "provider",
			Tier:         PluginTier(p),
			Version:      p.Version(),
			ConfigFields: p.ConfigFields(),
			TargetFields: p.TargetFields(),
		}
		if wa, ok := p.(WebhookAutomator); ok {
			plan := wa.WebhookPlan("", "", nil)
			it.WebhookSupported = plan.Supported
			it.WebhookAutomatable = plan.Automatable
		}
		attachUserState(&it)
		items = append(items, it)
	}

	sources := []item{}
	for _, sp := range s.Sources.List() {
		it := item{
			Name:         sp.Name(),
			Kind:         "source",
			Tier:         PluginTier(sp),
			Version:      sp.Version(),
			ConfigFields: sp.ConfigFields(),
			TargetFields: sp.TargetFields(),
		}
		if wa, ok := sp.(WebhookAutomator); ok {
			plan := wa.WebhookPlan("", "", nil)
			it.WebhookSupported = plan.Supported
			it.WebhookAutomatable = plan.Automatable
		}
		attachUserState(&it)
		sources = append(sources, it)
	}

	notifiers := []item{}
	for _, np := range s.Notifiers.List() {
		it := item{
			Name:         np.Name(),
			Kind:         "notifier",
			Tier:         PluginTier(np),
			Version:      np.Version(),
			ConfigFields: np.ConfigFields(),
		}
		attachUserState(&it)
		notifiers = append(notifiers, it)
	}

	strats := []strategyItem{}
	for _, st := range s.Strategies.List() {
		strats = append(strats, strategyItem{
			Name:         st.Name(),
			Tier:         PluginTier(st),
			Label:        st.Label(),
			Description:  st.Description(),
			ConfigFields: st.ConfigFields(),
		})
	}

	return e.JSON(http.StatusOK, map[string]any{
		"api_version":        "v1",
		"edition":            ent.Edition(),
		"plugins":            items,
		"sources":            sources,
		"notifiers":          notifiers,
		"version_strategies": strats,
	})
}

// handleWebhook: POST /api/v1/webhooks/{userId}/{pluginName}?product={productId}
// Handles both deployment provider webhooks and Git source (GitHub/GitLab) push/tag webhooks.
func (s *Service) handleWebhook(e *core.RequestEvent) error {
	notFound := apis.NewNotFoundError("", nil) // same answer for every miss, no enumeration
	pluginName := e.Request.PathValue("pluginName")
	userID := e.Request.PathValue("userId")

	if sp, ok := s.Sources.Get(pluginName); ok {
		return s.handleSourceWebhook(e, sp, userID)
	}

	p, ok := s.Registry.Get(pluginName)
	if !ok {
		return notFound
	}
	up, err := s.App.FindFirstRecordByFilter("user_plugins",
		"user = {:u} && plugin_name = {:n} && is_enabled = true",
		map[string]any{"u": userID, "n": p.Name()})
	if err != nil {
		return notFound
	}
	body, err := io.ReadAll(http.MaxBytesReader(e.Response, e.Request.Body, maxBodySize))
	if err != nil {
		return apis.NewBadRequestError("body too large", nil)
	}
	if err := p.VerifyWebhook(e.Request, body, up.GetString("webhook_secret")); err != nil {
		return apis.NewUnauthorizedError("invalid webhook credentials", nil)
	}
	product, err := s.App.FindFirstRecordByFilter("products", "(id = {:p} || name = {:p}) && user = {:u}",
		map[string]any{"p": e.Request.URL.Query().Get("product"), "u": up.GetString("user")})
	if err != nil {
		return apis.NewBadRequestError("unknown product", nil)
	}
	cfg := s.decryptPluginConfig(up)
	ev, err := p.ParseWebhook(body, cfg)
	if err != nil {
		return apis.NewBadRequestError("unparseable payload: "+err.Error(), nil)
	}
	if err := s.Dispatch(product.Id, ev); err != nil {
		return err
	}
	return e.JSON(http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (s *Service) handleSourceWebhook(e *core.RequestEvent, sp SourcePlugin, userID string) error {
	notFound := apis.NewNotFoundError("", nil)
	up, err := s.App.FindFirstRecordByFilter("user_plugins",
		"user = {:u} && plugin_name = {:n}",
		map[string]any{"u": userID, "n": sp.Name()})
	if err != nil {
		return notFound
	}
	body, err := io.ReadAll(http.MaxBytesReader(e.Response, e.Request.Body, maxBodySize))
	if err != nil {
		return apis.NewBadRequestError("body too large", nil)
	}
	if err := sp.VerifyWebhook(e.Request, body, up.GetString("webhook_secret")); err != nil {
		return apis.NewUnauthorizedError("invalid webhook credentials", nil)
	}
	if _, _, err := sp.ParsePushWebhook(body); err != nil {
		return apis.NewBadRequestError("unparseable payload: "+err.Error(), nil)
	}

	var radars []*core.Record
	if pid := e.Request.URL.Query().Get("product"); pid != "" {
		if prod, perr := s.App.FindFirstRecordByFilter("products", "(id = {:p} || name = {:p}) && user = {:u}",
			map[string]any{"p": pid, "u": userID}); perr == nil {
			radars = append(radars, prod)
		}
	} else {
		radars, _ = s.App.FindAllRecords("products", dbx.HashExp{"user": userID, "source_plugin": sp.Name()})
	}
	updated := 0
	for _, r := range radars {
		if err := s.SyncRadarSource(r); err == nil {
			updated++
		}
	}
	return e.JSON(http.StatusAccepted, map[string]any{"status": "accepted", "radars_updated": updated})
}

func (s *Service) decryptPluginConfig(up *core.Record) map[string]any {
	cfg := map[string]any{}
	if up == nil {
		return cfg
	}
	if v := up.GetString("config_json"); v != "" {
		if plain, err := s.Cipher.Decrypt(v); err == nil {
			_ = json.Unmarshal([]byte(plain), &cfg)
		}
	}
	return cfg
}

// Dispatch saves the event as a deployment (updating the existing row when ExternalID matches),
// pushes realtime updates via PocketBase, and fires enabled NotifierPlugins on status transitions.
func (s *Service) Dispatch(productID string, ev *DeploymentEvent) error {
	col, err := s.App.FindCollectionByNameOrId("deployments")
	if err != nil {
		return err
	}
	var r *core.Record
	prevStatus := ""
	if ev.ExternalID != "" {
		r, _ = s.App.FindFirstRecordByFilter("deployments", "product = {:p} && platform = {:pl} && external_id = {:e}",
			map[string]any{"p": productID, "pl": ev.Platform, "e": ev.ExternalID})
	}
	if r == nil {
		r = core.NewRecord(col)
		r.Set("product", productID)
		r.Set("platform", ev.Platform)
		r.Set("external_id", ev.ExternalID)
		r.Set("timestamp", types.NowDateTime())
	} else {
		prevStatus = r.GetString("status")
	}
	if !ev.Timestamp.IsZero() {
		dt, _ := types.ParseDateTime(ev.Timestamp)
		r.Set("timestamp", dt)
	}
	r.Set("status", ev.Status)
	if ev.Version != "" {
		r.Set("version", ev.Version)
	}
	if ev.CommitSHA != "" {
		r.Set("commit_sha", ev.CommitSHA)
	}
	if ev.Stage != "" {
		r.Set("stage", ev.Stage)
	}
	if ev.Progress > 0 {
		p := ev.Progress
		if p > 100 {
			p = 100
		}
		r.Set("progress", p)
	} else if ev.Status == StatusSuccess || ev.Status == StatusFailed {
		r.Set("progress", 100)
	}
	r.Set("message", ev.Message)
	if err := s.App.Save(r); err != nil {
		return err
	}

	// Notify on new status transitions only (avoid spamming on repeated polls of the same state).
	if prevStatus != ev.Status && s.Notifiers != nil && len(s.Notifiers.List()) > 0 {
		if prod, perr := s.App.FindRecordById("products", productID); perr == nil {
			eventName := "deployment_started"
			if ev.Status == StatusSuccess {
				eventName = "deployment_succeeded"
			} else if ev.Status == StatusFailed {
				eventName = "deployment_failed"
			}
			ts := ev.Timestamp
			if ts.IsZero() {
				ts = time.Now().UTC()
			}
			s.notifyUser(prod.GetString("user"), Notification{
				Event:         eventName,
				RadarID:       prod.Id,
				RadarName:     prod.GetString("name"),
				Platform:      ev.Platform,
				Status:        ev.Status,
				Version:       r.GetString("version"),
				LatestVersion: prod.GetString("latest_version"),
				Stage:         r.GetString("stage"),
				Message:       ev.Message,
				Timestamp:     ts,
			})
		}
	}
	return nil
}

// ensureAdmin creates the first superuser from VR_ADMIN_EMAIL / VR_ADMIN_PASSWORD when that email does not exist yet.
func (s *Service) ensureAdmin() error {
	email, pass := os.Getenv("VR_ADMIN_EMAIL"), os.Getenv("VR_ADMIN_PASSWORD")
	if email == "" || pass == "" {
		return nil
	}
	if _, err := s.App.FindAuthRecordByEmail(core.CollectionNameSuperusers, email); err == nil {
		return nil
	}
	col, err := s.App.FindCachedCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		return err
	}
	r := core.NewRecord(col)
	r.SetEmail(email)
	r.SetPassword(pass)
	return s.App.Save(r)
}

// ensureAuth makes Google the only way to sign in: password auth is off and sign-up is only allowed through OAuth2.
func (s *Service) ensureAuth() error {
	c, err := s.App.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	dev := os.Getenv("VR_DEV_PASSWORD_AUTH") == "1"
	c.PasswordAuth.Enabled = dev
	if dev {
		c.CreateRule = types.Pointer("")
	} else {
		c.CreateRule = types.Pointer(`@request.context = "oauth2"`)
	}
	id, secret := os.Getenv("VR_GOOGLE_CLIENT_ID"), os.Getenv("VR_GOOGLE_CLIENT_SECRET")
	c.OAuth2.Enabled = id != "" && secret != ""
	c.OAuth2.Providers = nil
	if c.OAuth2.Enabled {
		c.OAuth2.Providers = []core.OAuth2ProviderConfig{{Name: "google", ClientId: id, ClientSecret: secret}}
	} else {
		s.App.Logger().Warn("VR_GOOGLE_CLIENT_ID/SECRET not set: nobody can sign in")
	}
	return s.App.Save(c)
}

// saveConfig: POST /api/v1/plugins/{pluginName}/config {"is_enabled": bool?, "config": {...}?}
// Creates the user's plugin record if needed and stores the credentials encrypted.
func (s *Service) saveConfig(e *core.RequestEvent) error {
	name := e.Request.PathValue("pluginName")
	fields, tier, ok := s.pluginConfigFields(name)
	if !ok {
		return apis.NewNotFoundError("", nil)
	}
	if !s.entitlements().CanUsePlugin(e.Auth, name, tier) {
		return apis.NewForbiddenError("This plugin requires VersionRadar Pro", nil)
	}
	var body struct {
		IsEnabled *bool          `json:"is_enabled"`
		Config    map[string]any `json:"config"`
	}
	if err := e.BindBody(&body); err != nil {
		return apis.NewBadRequestError("invalid body", err)
	}
	rec, err := s.App.FindFirstRecordByFilter("user_plugins", "user = {:u} && plugin_name = {:n}",
		map[string]any{"u": e.Auth.Id, "n": name})
	if err != nil {
		col, err := s.App.FindCollectionByNameOrId("user_plugins")
		if err != nil {
			return err
		}
		rec = core.NewRecord(col)
		rec.Set("user", e.Auth.Id)
		rec.Set("plugin_name", name)
	}
	if body.Config != nil {
		for _, f := range fields {
			if f.Optional {
				continue
			}
			if v, exists := body.Config[f.Key]; !exists || v == nil || v == "" {
				return apis.NewBadRequestError(f.Label+" is required", nil)
			}
		}
		raw, _ := json.Marshal(body.Config)
		rec.Set("config_json", string(raw)) // encrypted by the user_plugins save hook
	}
	if body.IsEnabled != nil {
		rec.Set("is_enabled", *body.IsEnabled)
	}
	if err := s.App.Save(rec); err != nil {
		return err
	}
	return e.JSON(http.StatusOK, map[string]string{"status": "saved"})
}
