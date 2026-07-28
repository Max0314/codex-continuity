package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncSessionPathKeepsOutsidePathsPrivate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()

	relative, underRoot, selected := syncSessionPath(project, root, false)
	if !underRoot || !selected || filepath.ToSlash(relative) != "project" {
		t.Fatalf("inside path = (%q, %v, %v)", relative, underRoot, selected)
	}

	relative, underRoot, selected = syncSessionPath(outside, root, false)
	if relative != unassignedPath || underRoot || selected {
		t.Fatalf("disabled outside path = (%q, %v, %v)", relative, underRoot, selected)
	}

	relative, underRoot, selected = syncSessionPath(outside, root, true)
	if relative != unassignedPath || underRoot || !selected {
		t.Fatalf("enabled outside path = (%q, %v, %v)", relative, underRoot, selected)
	}
	if relative == filepath.ToSlash(outside) {
		t.Fatal("outside absolute path leaked into sync metadata")
	}
}
