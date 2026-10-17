package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCentralSecurityEpochIsSessionScopedAndRefreshPreservesIt(t *testing.T) {
	var epoch atomic.Value
	epoch.Store(strings.Repeat("a", 43))
	var unavailable atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unavailable.Load() {
			w.WriteHeader(503)
			return
		}
		if id, key, ok := r.BasicAuth(); !ok || id != "fribench-codex-continuity" || key != "fixture" {
			t.Error("untrusted state request")
			w.WriteHeader(401)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"active": true, "security_version": epoch.Load()})
	}))
	defer provider.Close()
	cfg := Config{DataDir: t.TempDir(), AdminEmail: "operator@example.invalid", AdminPassword: "fixture-only", AdminName: "operator", IdentitySecret: "fixture", IdentityEndpoint: provider.URL + "/internal/identity"}
	cfg.DownloadDir = filepath.Join(cfg.DataDir, "downloads")
	store, err := OpenStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server := &HTTPServer{cfg: cfg, store: store, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	user, err := store.unifiedUser("fixture-owner", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "https://fixture.invalid/", nil)
	old, err := store.CreateClientSession(user.ID, time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	store.db.Exec("UPDATE client_sessions SET security_version=? WHERE access_hash=?", epoch.Load(), tokenDigest(old.AccessToken))
	if err = server.checkUnifiedSession(request, old.AccessToken, false); err != nil {
		t.Fatal(err)
	}
	epoch.Store(strings.Repeat("b", 43))
	fresh, err := store.CreateClientSession(user.ID, time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	store.db.Exec("UPDATE client_sessions SET security_version=? WHERE access_hash=?", epoch.Load(), tokenDigest(fresh.AccessToken))
	if server.checkUnifiedSession(request, old.AccessToken, false) == nil {
		t.Fatal("new login reactivated an old device")
	}
	if err = server.checkUnifiedSession(request, fresh.AccessToken, false); err != nil {
		t.Fatal(err)
	}
	rotated, err := store.RefreshClientSession(fresh.RefreshToken, time.Hour, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err = server.checkUnifiedSession(request, rotated.Session.AccessToken, false); err != nil {
		t.Fatal("refresh lost security epoch", err)
	}
	unavailable.Store(true)
	if server.checkUnifiedSession(request, rotated.Session.AccessToken, false) == nil {
		t.Fatal("provider failure allowed access")
	}
	unavailable.Store(false)
	if err = server.checkUnifiedSession(request, rotated.Session.AccessToken, false); err != nil {
		t.Fatal("transient failure deleted valid authorization", err)
	}
	var before string
	store.db.QueryRow("SELECT wrapped_key FROM users WHERE id=?", user.ID).Scan(&before)
	epoch.Store(strings.Repeat("c", 43))
	if server.checkUnifiedSession(request, rotated.Session.RefreshToken, true) == nil {
		t.Fatal("revoked refresh remained usable")
	}
	var after string
	store.db.QueryRow("SELECT wrapped_key FROM users WHERE id=?", user.ID).Scan(&after)
	if before != after {
		t.Fatal("revocation changed encryption key")
	}
}
