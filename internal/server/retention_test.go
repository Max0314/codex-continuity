package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neonet/codex-continuity/internal/model"
)

func TestCreateHandoffKeepsOnlyNewestVersionsAndDeletesBlobs(t *testing.T) {
	store, userID, deviceID := newRetentionTestStore(t, handoffRetentionPolicy{
		maxAge:        14 * 24 * time.Hour,
		maxVersions:   2,
		maxUserBytes:  1 << 20,
		maxTotalBytes: 2 << 20,
	})

	firstPath := writeRetentionTestBlob(t, store, "first.ccx", 8)
	first := createRetentionTestHandoff(t, store, userID, deviceID, firstPath, 8)
	secondPath := writeRetentionTestBlob(t, store, "second.ccx", 8)
	createRetentionTestHandoff(t, store, userID, deviceID, secondPath, 8)
	thirdPath := writeRetentionTestBlob(t, store, "third.ccx", 8)
	createRetentionTestHandoff(t, store, userID, deviceID, thirdPath, 8)

	if _, _, err := store.HandoffByID(userID, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("oldest handoff error = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(filepath.Join(store.dataDir, firstPath)); !os.IsNotExist(err) {
		t.Fatalf("oldest blob still exists: %v", err)
	}
	handoffs, err := store.ListHandoffs(userID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(handoffs) != 2 {
		t.Fatalf("handoff count = %d, want 2", len(handoffs))
	}
}

func TestCreateHandoffEnforcesUserAndTotalStorageQuotas(t *testing.T) {
	store, userID, deviceID := newRetentionTestStore(t, handoffRetentionPolicy{
		maxAge:        14 * 24 * time.Hour,
		maxVersions:   100,
		maxUserBytes:  10,
		maxTotalBytes: 100,
	})
	firstPath := writeRetentionTestBlob(t, store, "quota-first.ccx", 6)
	first := createRetentionTestHandoff(t, store, userID, deviceID, firstPath, 6)
	secondPath := writeRetentionTestBlob(t, store, "quota-second.ccx", 6)
	createRetentionTestHandoff(t, store, userID, deviceID, secondPath, 6)
	if _, _, err := store.HandoffByID(userID, first.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("handoff above user quota error = %v, want ErrNotFound", err)
	}

	store.retention.maxUserBytes = 100
	store.retention.maxTotalBytes = 10
	rejectedPath := writeRetentionTestBlob(t, store, "quota-rejected.ccx", 6)
	_, _, err := store.createOwnedHandoff(CreateHandoffParams{
		UserID:         userID,
		ProjectName:    "配额测试",
		WorkspaceKey:   "another-workspace",
		SourceDeviceID: deviceID,
		Manifest:       json.RawMessage(`{"version":1}`),
		BlobPath:       rejectedPath,
		BlobSize:       6,
	}, "", "")
	if !errors.Is(err, ErrStorageQuota) {
		t.Fatalf("total quota error = %v, want ErrStorageQuota", err)
	}
	var total, count int64
	if err := store.db.QueryRow("SELECT COALESCE(SUM(blob_size),0),COUNT(*) FROM handoffs").Scan(&total, &count); err != nil {
		t.Fatal(err)
	}
	if total != 6 || count != 1 {
		t.Fatalf("storage after rejected upload = %d bytes across %d rows, want 6 across 1", total, count)
	}
}

func TestMaintenanceRemovesExpiredMissingAndOrphanBlobs(t *testing.T) {
	store, userID, deviceID := newRetentionTestStore(t, handoffRetentionPolicy{
		maxAge:        14 * 24 * time.Hour,
		maxVersions:   10,
		maxUserBytes:  1 << 20,
		maxTotalBytes: 2 << 20,
	})
	now := time.Now().UTC()
	expiredPath := writeRetentionTestBlob(t, store, "expired.ccx", 11)
	expired := createRetentionTestHandoff(t, store, userID, deviceID, expiredPath, 11)
	missingPath := writeRetentionTestBlob(t, store, "missing.ccx", 7)
	missing := createRetentionTestHandoff(t, store, userID, deviceID, missingPath, 7)
	if _, err := store.db.Exec(
		"UPDATE handoffs SET created_at=? WHERE id=?",
		now.Add(-20*24*time.Hour).Format(time.RFC3339Nano),
		expired.ID,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(store.dataDir, missingPath)); err != nil {
		t.Fatal(err)
	}
	orphanPath := filepath.Join(store.dataDir, "blobs", "orphan.ccx")
	if err := os.WriteFile(orphanPath, bytes.Repeat([]byte{0x42}, 13), 0o600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-2 * time.Hour)
	if err := os.Chtimes(orphanPath, old, old); err != nil {
		t.Fatal(err)
	}

	result, err := store.Maintenance(now)
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedHandoffs != 2 || result.DeletedBlobs != 2 || result.ReclaimedBytes != 24 {
		t.Fatalf("unexpected maintenance result: %#v", result)
	}
	for _, id := range []string{expired.ID, missing.ID} {
		if _, _, err := store.HandoffByID(userID, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("cleaned handoff %s error = %v, want ErrNotFound", id, err)
		}
	}
	for _, path := range []string{filepath.Join(store.dataDir, expiredPath), orphanPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("cleaned blob still exists %s: %v", path, err)
		}
	}
}

func TestSafeBlobPathRejectsTraversal(t *testing.T) {
	store, _, _ := newRetentionTestStore(t, handoffRetentionPolicy{})
	if _, err := store.safeBlobPath(filepath.Join("..", "outside.ccx")); err == nil {
		t.Fatal("path traversal was accepted")
	}
}

func TestOwnedHandoffQuotaFailurePreservesOldBlobAndGuestClaim(t *testing.T) {
	store, userID, deviceID := newRetentionTestStore(t, handoffRetentionPolicy{
		maxVersions: 1, maxUserBytes: 100, maxTotalBytes: 10,
	})
	oldPath := writeRetentionTestBlob(t, store, "rollback-old.ccx", 6)
	old := createRetentionTestHandoff(t, store, userID, deviceID, oldPath, 6)
	store.retention.maxTotalBytes = 5
	newPath := writeRetentionTestBlob(t, store, "rollback-new.ccx", 7)
	params := CreateHandoffParams{
		UserID: userID, SourceDeviceID: deviceID, WorkspaceKey: "retention-workspace",
		Manifest: json.RawMessage(`{}`), BlobPath: newPath, BlobSize: 7,
	}
	if _, _, err := store.createOwnedHandoff(params, "", ""); !errors.Is(err, ErrStorageQuota) {
		t.Fatalf("quota error = %v", err)
	}
	if _, _, err := store.HandoffByID(userID, old.ID); err != nil {
		t.Fatalf("rejected upload deleted the old record: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.dataDir, oldPath)); err != nil {
		t.Fatalf("rejected upload deleted the old blob: %v", err)
	}

	// Removing the old row makes a guest claim eligible; rejection must roll back
	// its once-only claim and leave the preview reusable.
	if _, err := store.db.Exec("DELETE FROM handoffs WHERE user_id=?", userID); err != nil {
		t.Fatal(err)
	}
	stage, digest := "quota-stage", "quota-digest"
	if _, err := store.db.Exec(
		"INSERT INTO continuity_guest_stages(id,user_id,guest_hash,digest,size,expires_at) VALUES(?,?,?,?,?,?)",
		stage, userID, "guest-fixture", digest, params.BlobSize, time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano),
	); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.createOwnedHandoff(params, stage, digest); !errors.Is(err, ErrStorageQuota) {
		t.Fatalf("guest quota error = %v", err)
	}
	var claims, previews int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM continuity_guest_claims WHERE user_id=?", userID).Scan(&claims); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM continuity_guest_stages WHERE id=?", stage).Scan(&previews); err != nil {
		t.Fatal(err)
	}
	if claims != 0 || previews != 1 {
		t.Fatalf("quota failure consumed guest state: claims=%d previews=%d", claims, previews)
	}
}

func newRetentionTestStore(
	t *testing.T,
	policy handoffRetentionPolicy,
) (*Store, string, string) {
	t.Helper()
	dataDir := t.TempDir()
	cfg := Config{
		Address:           ":0",
		DataDir:           dataDir,
		WebDir:            filepath.Join(dataDir, "missing-web"),
		AdminEmail:        "admin@example.com",
		AdminPassword:     "a-strong-test-password",
		AdminName:         "测试管理员",
		SessionTTL:        24 * time.Hour,
		ClientAccessTTL:   15 * time.Minute,
		ClientRefreshTTL:  30 * 24 * time.Hour,
		MaxUploadBytes:    20 << 20,
		HandoffRetention:  policy.maxAge,
		HandoffVersions:   policy.maxVersions,
		UserStorageBytes:  policy.maxUserBytes,
		TotalStorageBytes: policy.maxTotalBytes,
		DownloadDir:       filepath.Join(dataDir, "downloads"),
	}
	store, err := OpenStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	user, err := store.Authenticate(cfg.AdminEmail, cfg.AdminPassword)
	if err != nil {
		t.Fatal(err)
	}
	device, err := store.UpsertDevice(user.ID, model.Device{
		ID:            "retention-device",
		Name:          "保留策略测试设备",
		Hostname:      "test-host",
		OS:            "windows/amd64",
		ClientVersion: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, user.ID, device.ID
}

func writeRetentionTestBlob(t *testing.T, store *Store, name string, size int) string {
	t.Helper()
	relativePath := filepath.Join("blobs", name)
	if err := os.WriteFile(
		filepath.Join(store.dataDir, relativePath),
		bytes.Repeat([]byte{0x42}, size),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	return relativePath
}

func createRetentionTestHandoff(
	t *testing.T,
	store *Store,
	userID string,
	deviceID string,
	blobPath string,
	blobSize int64,
) model.Handoff {
	t.Helper()
	handoff, _, err := store.createOwnedHandoff(CreateHandoffParams{
		UserID:         userID,
		ProjectName:    "保留策略测试",
		WorkspaceKey:   "retention-workspace",
		SourceDeviceID: deviceID,
		Manifest:       json.RawMessage(`{"version":1}`),
		BlobPath:       blobPath,
		BlobSize:       blobSize,
	}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return handoff
}
