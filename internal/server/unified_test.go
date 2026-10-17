package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/neonet/codex-continuity/internal/model"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUnifiedPostgresIsolationAndOnceClaim(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required for real PostgreSQL")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("continuity_verify_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	parsed, _ := url.Parse(dsn)
	q := parsed.Query()
	q.Set("search_path", schema)
	parsed.RawQuery = q.Encode()
	root := t.TempDir()
	cfg := Config{DatabaseURL: parsed.String(), DataDir: root, DownloadDir: filepath.Join(root, "downloads"), WebDir: filepath.Join(root, "web"), AdminEmail: "operator@example.invalid", AdminPassword: "fixture-admin-only", AdminName: "operator", ClientAccessTTL: time.Minute, ClientRefreshTTL: time.Hour, IdentitySecret: "fixture-secret", PublicURL: "https://continuity.fribench.cn", MaxUploadBytes: 1 << 20}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, secret, ok := r.BasicAuth()
		if !ok || id != "fribench-codex-continuity" || secret != cfg.IdentitySecret || r.Host != "continuity.fribench.cn" {
			w.WriteHeader(401)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/account-state") {
			json.NewEncoder(w).Encode(map[string]any{"active":true,"security_version":strings.Repeat("s",43)})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/exchange") {
			json.NewEncoder(w).Encode(map[string]string{"sub": "fixture-account-a", "name": "甲","security_version":strings.Repeat("s",43)})
			return
		}
		cookie, e := r.Cookie("__Host-fribench_codex_continuity")
		if e != nil || cookie.Value != "fixture-cookie" {
			w.WriteHeader(401)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"authenticated": true, "sub": "fixture-account-a", "name": "甲"})
	}))
	defer provider.Close()
	cfg.IdentityEndpoint = provider.URL + "/internal/identity"
	store, err := OpenStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	handler := httptest.NewServer(NewHTTPServer(cfg, store, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer handler.Close()
	res, err := http.Post(handler.URL+"/api/v1/client/auth/fribench", "application/json", strings.NewReader(`{"code":"fixture","code_verifier":"fixture","redirect_uri":"fixture","keyHash":"`+strings.Repeat("a", 64)+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	var auth struct {
		User    model.User `json:"user"`
		Access  string     `json:"accessToken"`
		Refresh string     `json:"refreshToken"`
		Wrapped string     `json:"wrappedKey"`
	}
	json.NewDecoder(res.Body).Decode(&auth)
	res.Body.Close()
	if res.StatusCode != 200 || auth.User.Role != "member" || auth.Access == "" {
		t.Fatalf("grant failed status=%d", res.StatusCode)
	}
	same, err := store.unifiedUser("fixture-account-a", "甲")
	if err != nil || same.ID != auth.User.ID {
		t.Fatal("unstable identity", err)
	}
	other, err := store.unifiedUser("fixture-account-b", "乙")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", handler.URL+"/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "__Host-fribench_codex_continuity", Value: "fixture-cookie"})
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("central cookie status=%d", res.StatusCode)
	}
	device, err := store.UpsertDevice(same.ID, model.Device{ID: "fixture-device", Name: "fixture", OS: "test"})
	if err != nil {
		t.Fatal(err)
	}
	preview := bearerJSONRequest(t, "POST", handler.URL+"/api/v1/client/guest-preview", `{"guest":"fixture-guest-installation-1234","sha256":"`+strings.Repeat("b", 64)+`","size":42}`, auth.Access)
	res, err = http.DefaultClient.Do(preview)
	if err != nil {
		t.Fatal(err)
	}
	var stage struct {
		Stage string `json:"stage"`
	}
	json.NewDecoder(res.Body).Decode(&stage)
	res.Body.Close()
	if res.StatusCode != 200 || stage.Stage == "" {
		t.Fatalf("preview status=%d", res.StatusCode)
	}
	var count int
	store.db.QueryRow("SELECT COUNT(*) FROM continuity_guest_claims").Scan(&count)
	if count != 0 {
		t.Fatal("preview consumed claim")
	}
	params := CreateHandoffParams{UserID: same.ID, SourceDeviceID: device.ID, ProjectName: "fixture", WorkspaceKey: "fixture", Manifest: json.RawMessage(`{}`), BlobPath: "fixture.ccx", BlobSize: 42}
	bad := params
	bad.UserID = other.ID
	if _, _, err = store.createOwnedHandoff(bad, stage.Stage, strings.Repeat("b", 64)); err == nil {
		t.Fatal("other account accepted stage")
	}
	if _, _, err = store.createOwnedHandoff(params, stage.Stage, strings.Repeat("c", 64)); err == nil {
		t.Fatal("changed payload accepted")
	}
	// An insert failure must roll back the already-inserted claim.
	_, err = store.db.Exec(`CREATE FUNCTION fail_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture'; END $$; CREATE TRIGGER fail_fixture BEFORE INSERT ON handoffs FOR EACH ROW EXECUTE FUNCTION fail_fixture()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.createOwnedHandoff(params, stage.Stage, strings.Repeat("b", 64)); err == nil {
		t.Fatal("injected failure accepted")
	}
	store.db.QueryRow("SELECT COUNT(*) FROM continuity_guest_claims").Scan(&count)
	if count != 0 {
		t.Fatal("failed write consumed claim")
	}
	store.db.Exec("DROP TRIGGER fail_fixture ON handoffs")
	h, duplicate, err := store.createOwnedHandoff(params, stage.Stage, strings.Repeat("b", 64))
	if err != nil || duplicate {
		t.Fatal("claim failed", err)
	}
	again, duplicate, err := store.createOwnedHandoff(params, stage.Stage, strings.Repeat("b", 64))
	if err != nil || !duplicate || again.ID != h.ID {
		t.Fatal("retry not idempotent", err)
	}
	if _, _, err = store.HandoffByID(other.ID, h.ID); err == nil {
		t.Fatal("cross-account read")
	}
	if _, _, err = store.createOwnedHandoff(params, "", ""); err != nil {
		t.Fatal("normal save blocked after claim", err)
	}
	rotated, err := store.RefreshClientSession(auth.Refresh, time.Minute, time.Hour)
	if err != nil || rotated.Session.RefreshToken == auth.Refresh {
		t.Fatal("rotation failed", err)
	}
	if _, err = store.UserByClientAccessToken(auth.Access); err == nil {
		t.Fatal("old access still accepted")
	}
	store.DeleteClientSession(rotated.Session.RefreshToken)
	if _, err = store.UserByClientAccessToken(rotated.Session.AccessToken); err == nil {
		t.Fatal("revoked access still accepted")
	}
	deviceSession, err := store.CreateClientSession(same.ID, time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	otherSession, err := store.CreateClientSession(other.ID, time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	revoke, _ := http.NewRequest("POST", handler.URL+"/api/v1/client-sessions/revoke", strings.NewReader(`{"confirm":true}`))
	revoke.Header.Set("Content-Type", "application/json")
	revoke.AddCookie(&http.Cookie{Name: "__Host-fribench_codex_continuity", Value: "fixture-cookie"})
	res, err = http.DefaultClient.Do(revoke)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatal("originless revocation accepted")
	}
	revoke, _ = http.NewRequest("POST", handler.URL+"/api/v1/client-sessions/revoke", strings.NewReader(`{"confirm":true}`))
	revoke.Header.Set("Content-Type", "application/json")
	revoke.Header.Set("Origin", cfg.PublicURL)
	revoke.AddCookie(&http.Cookie{Name: "__Host-fribench_codex_continuity", Value: "fixture-cookie"})
	res, err = http.DefaultClient.Do(revoke)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("own revocation failed")
	}
	if _, err = store.UserByClientAccessToken(deviceSession.AccessToken); err == nil {
		t.Fatal("revoked desktop active")
	}
	if _, err = store.UserByClientAccessToken(otherSession.AccessToken); err != nil {
		t.Fatal("other account revoked", err)
	}
}
