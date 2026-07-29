//go:build !windows

package tuttiagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTuttiAgentAuthSnapshotFollowsSymlinkWithoutReplacingIt(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(filepath.Join(home, ".tutti-agent"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(shared, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(shared, "auth.json")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	targetParent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	resolvedTarget := filepath.Join(targetParent, filepath.Base(target))
	link := filepath.Join(home, ".tutti-agent", "auth.json")
	if err := os.Symlink(filepath.Join("..", "..", "shared", "auth.json"), link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	snapshot, err := captureTuttiAgentAuthSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.path != resolvedTarget {
		t.Fatalf("snapshot path = %q, want %q", snapshot.path, resolvedTarget)
	}
	if err := os.WriteFile(target, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Restore(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("auth symlink was replaced: info=%v err=%v", info, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "old" {
		t.Fatalf("restored target = %q, %v; want old", got, err)
	}

	lock, err := acquireTuttiAgentAuthMutationLock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()
	if want := resolvedTarget + ".refresh.lock"; lock.Path() != want {
		t.Fatalf("lock path = %q, want %q", lock.Path(), want)
	}
}

func TestResolveTuttiAgentAuthFileTargetRejectsSymlinkLoop(t *testing.T) {
	root := t.TempDir()
	auth := filepath.Join(root, "auth.json")
	next := filepath.Join(root, "auth.next.json")
	if err := os.Symlink(filepath.Base(next), auth); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(auth), next); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTuttiAgentAuthFileTarget(auth); err == nil {
		t.Fatal("resolveTuttiAgentAuthFileTarget() succeeded for symlink loop")
	}
}
