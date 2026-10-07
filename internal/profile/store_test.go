package profile

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RBen19/devswitch/internal/provider"
)

func TestStoreAddAndLoad(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".devswitch")
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Add(provider.Provider{ID: provider.Claude}, "perso", false)
	if err != nil {
		t.Fatal(err)
	}
	if created.Home == "" {
		t.Fatal("expected profile home")
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loaded.Find(provider.Claude, "perso"); err != nil {
		t.Fatalf("profile was not persisted: %v", err)
	}
}

func TestConcurrentProfileCreation(t *testing.T) {
	root := t.TempDir()
	errors := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func(i int) {
			store, err := Load(root)
			if err == nil {
				_, err = store.Add(provider.Provider{ID: provider.Codex}, fmt.Sprintf("profile-%d", i), false)
			}
			errors <- err
		}(i)
	}
	for i := 0; i < 12; i++ {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	store, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Profiles) != 12 {
		t.Fatalf("lost profiles: got %d", len(store.Profiles))
	}
}

func TestProfileNamesCannotEscapeStorage(t *testing.T) {
	store := &Store{Root: t.TempDir()}
	for _, name := range []string{".", "..", "../outside", "-flag", "a\nb", "a\x00b"} {
		if _, err := store.Add(provider.Provider{ID: provider.Codex}, name, false); err == nil {
			t.Fatalf("accepted name %q", name)
		}
	}
}

func TestFindExplainsMissingProfile(t *testing.T) {
	store, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claude, _ := provider.Parse("claude")

	_, err = store.Find(provider.Claude, "wrk")
	if err == nil || err.Error() != `no Claude Code account named "wrk" yet. Create it with: devswitch add claude wrk` {
		t.Fatalf("empty store: %v", err)
	}

	for _, name := range []string{"work", "personal"} {
		if _, err := store.Add(claude, name, true); err != nil {
			t.Fatal(err)
		}
	}
	_, err = store.Find(provider.Claude, "wrk")
	if err == nil || err.Error() != `no Claude Code account named "wrk". Your Claude Code accounts: personal, work` {
		t.Fatalf("populated store: %v", err)
	}
	if _, err := store.Find(provider.Codex, "work"); err == nil || !strings.Contains(err.Error(), "devswitch add codex work") {
		t.Fatalf("other provider's profiles must not be listed: %v", err)
	}
}
