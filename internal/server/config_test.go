package server

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigUsesBoundedStorageDefaults(t *testing.T) {
	t.Setenv("CONTINUITY_DATA_DIR", t.TempDir())
	t.Setenv("CONTINUITY_ADMIN_PASSWORD", "a-strong-test-password")
	for _, key := range []string{
		"CONTINUITY_MAX_UPLOAD_MIB",
		"CONTINUITY_HANDOFF_RETENTION_DAYS",
		"CONTINUITY_HANDOFF_VERSIONS",
		"CONTINUITY_USER_STORAGE_MIB",
		"CONTINUITY_TOTAL_STORAGE_MIB",
	} {
		t.Setenv(key, "")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HandoffRetention != 14*24*time.Hour || cfg.HandoffVersions != 2 {
		t.Fatalf(
			"unexpected retention defaults: duration=%s versions=%d",
			cfg.HandoffRetention,
			cfg.HandoffVersions,
		)
	}
	if cfg.UserStorageBytes != 2048*1024*1024 || cfg.TotalStorageBytes != 10240*1024*1024 {
		t.Fatalf(
			"unexpected storage defaults: user=%d total=%d",
			cfg.UserStorageBytes,
			cfg.TotalStorageBytes,
		)
	}
}

func TestLoadConfigRejectsUnsafeStorageLimits(t *testing.T) {
	tests := []struct {
		name, key, value, want string
	}{
		{"zero versions", "CONTINUITY_HANDOFF_VERSIONS", "0", "between 1 and 100"},
		{"user below upload", "CONTINUITY_USER_STORAGE_MIB", "499", "at least CONTINUITY_MAX_UPLOAD_MIB"},
		{"total below user", "CONTINUITY_TOTAL_STORAGE_MIB", "1024", "at least CONTINUITY_USER_STORAGE_MIB"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CONTINUITY_DATA_DIR", t.TempDir())
			t.Setenv("CONTINUITY_ADMIN_PASSWORD", "a-strong-test-password")
			t.Setenv(test.key, test.value)
			_, err := LoadConfig()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadConfig error = %v, want text %q", err, test.want)
			}
		})
	}
}
