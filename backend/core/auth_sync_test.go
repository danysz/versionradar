package core_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	pbcore "github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/danysz/versionradar/backend/core"
	_ "github.com/danysz/versionradar/backend/pb_migrations"
)

func newAuthTestApp(t *testing.T) (*tests.TestApp, http.Handler) {
	t.Helper()

	app, err := tests.NewTestApp(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create test app: %v", err)
	}
	t.Cleanup(app.Cleanup)

	cipher, err := core.NewCipher(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err != nil {
		t.Fatalf("failed to create cipher: %v", err)
	}

	svc := &core.Service{
		App:        app,
		Cipher:     cipher,
		Registry:   core.NewRegistry(),
		Sources:    core.NewSourceRegistry(),
		Strategies: core.NewStrategyRegistry(),
		Notifiers:  core.NewNotifierRegistry(),
	}
	svc.Bind(app)

	pbRouter, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("failed to create router: %v", err)
	}

	var handler http.Handler
	serveEvent := &pbcore.ServeEvent{App: app, Router: pbRouter}
	if err := app.OnServe().Trigger(serveEvent, func(e *pbcore.ServeEvent) error {
		var buildErr error
		handler, buildErr = e.Router.BuildMux()
		return buildErr
	}); err != nil {
		t.Fatalf("OnServe failed: %v", err)
	}

	return app, handler
}

func authWithPassword(t *testing.T, handler http.Handler, email, password string) (int, string, string) {
	t.Helper()

	body, err := json.Marshal(map[string]string{
		"identity": email,
		"password": password,
	})
	if err != nil {
		t.Fatalf("failed to marshal login payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/collections/users/auth-with-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp.Token, rec.Body.String()
}

func TestSuperuserSyncToUsersAndPasswordAuth(t *testing.T) {
	// Ensure password auth works by default without VR_DEV_PASSWORD_AUTH or Google OAuth env vars.
	t.Setenv("VR_DEV_PASSWORD_AUTH", "")
	t.Setenv("VR_GOOGLE_CLIENT_ID", "")
	t.Setenv("VR_GOOGLE_CLIENT_SECRET", "")

	app, handler := newAuthTestApp(t)

	usersCol, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("failed to find users collection: %v", err)
	}
	if !usersCol.PasswordAuth.Enabled {
		t.Fatal("expected users PasswordAuth.Enabled to be true by default")
	}
	if usersCol.CreateRule == nil || *usersCol.CreateRule != "" {
		t.Fatalf("expected users CreateRule to be empty string pointer for local signup, got %v", usersCol.CreateRule)
	}

	superCol, err := app.FindCollectionByNameOrId(pbcore.CollectionNameSuperusers)
	if err != nil {
		t.Fatalf("failed to find superusers collection: %v", err)
	}

	email := "admin@versionradar.local"
	pass1 := "super-secret-password-1"
	super := pbcore.NewRecord(superCol)
	super.SetEmail(email)
	super.SetPassword(pass1)
	if err := app.Save(super); err != nil {
		t.Fatalf("failed to save superuser: %v", err)
	}

	user, err := app.FindAuthRecordByEmail("users", email)
	if err != nil {
		t.Fatalf("expected synced user record for %s, got error: %v", email, err)
	}
	if !user.Verified() {
		t.Fatal("expected synced user to be verified")
	}
	superHash := super.GetString("password:hash")
	userHash := user.GetString("password:hash")
	if superHash == "" || userHash != superHash {
		t.Fatalf("expected synced user password hash %q to match superuser hash %q", userHash, superHash)
	}
	if !user.ValidatePassword(pass1) {
		t.Fatal("expected synced user ValidatePassword to succeed with initial password")
	}

	status, token, rawBody := authWithPassword(t, handler, email, pass1)
	if status != http.StatusOK || token == "" {
		t.Fatalf("expected HTTP 200 and non-empty token for synced user login, got status=%d token=%q body=%s", status, token, rawBody)
	}

	// Updating the superuser password should sync the new hash into users.
	pass2 := "updated-super-secret-2"
	super.SetPassword(pass2)
	if err := app.Save(super); err != nil {
		t.Fatalf("failed to update superuser password: %v", err)
	}

	oldStatus, _, _ := authWithPassword(t, handler, email, pass1)
	if oldStatus == http.StatusOK {
		t.Fatal("expected old password to fail after superuser password update")
	}
	newStatus, newToken, newBody := authWithPassword(t, handler, email, pass2)
	if newStatus != http.StatusOK || newToken == "" {
		t.Fatalf("expected HTTP 200 and non-empty token after password update, got status=%d token=%q body=%s", newStatus, newToken, newBody)
	}
}

func TestEnsureAdminSyncsToUsers(t *testing.T) {
	email := "envadmin@versionradar.local"
	pass := "env-admin-pass-123"
	t.Setenv("VR_ADMIN_EMAIL", email)
	t.Setenv("VR_ADMIN_PASSWORD", pass)

	app, handler := newAuthTestApp(t)

	super, err := app.FindAuthRecordByEmail(pbcore.CollectionNameSuperusers, email)
	if err != nil {
		t.Fatalf("expected ensureAdmin to create superuser: %v", err)
	}
	user, err := app.FindAuthRecordByEmail("users", email)
	if err != nil {
		t.Fatalf("expected ensureAdmin to sync superuser to users: %v", err)
	}
	if user.GetString("password:hash") != super.GetString("password:hash") {
		t.Fatal("expected synced user password hash to match ensureAdmin superuser")
	}

	status, token, rawBody := authWithPassword(t, handler, email, pass)
	if status != http.StatusOK || token == "" {
		t.Fatalf("expected HTTP 200 and token for ensureAdmin user, got status=%d token=%q body=%s", status, token, rawBody)
	}
}
