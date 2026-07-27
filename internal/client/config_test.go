package client

import (
	"strings"
	"testing"
)

func TestEffectiveSyncScopeUsesSafeDefaultsForLegacyConfig(t *testing.T) {
	t.Parallel()
	scope := (Config{}).EffectiveSyncScope()
	if scope.Days != 7 || scope.IncludeArchived || scope.MaxBundleMiB != 500 || len(scope.ProjectPaths) != 0 {
		t.Fatalf("unexpected legacy defaults: %#v", scope)
	}
}

func TestProjectAndSessionSelection(t *testing.T) {
	t.Parallel()
	selected := normalizedProjectSet([]string{"bi_center", "nested/service"})
	cases := []struct {
		path string
		want bool
	}{
		{"bi_center", true},
		{"bi_center/backend", true},
		{"nested/service", true},
		{"nested/service/api", true},
		{"other", false},
	}
	for _, test := range cases {
		if got := sessionProjectSelected(test.path, selected); got != test.want {
			t.Errorf("sessionProjectSelected(%q) = %v, want %v", test.path, got, test.want)
		}
	}
	if !projectPathSelected("bi_center", selected) || projectPathSelected("bi_center/backend", selected) {
		t.Fatal("project selection must match exact Git project paths")
	}
}

func TestConfigRejectsBundleLimitAbove500MiB(t *testing.T) {
	t.Parallel()
	cfg := Config{
		ServerURL:     "https://continuity.example.com",
		Token:         "token",
		Root:          "D:/code_CPL",
		DeviceName:    "电脑",
		EncryptionKey: strings.Repeat("A", 43),
		SyncScope: SyncScope{
			Days:         7,
			MaxBundleMiB: 501,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted a bundle limit above 500 MiB")
	}
}
