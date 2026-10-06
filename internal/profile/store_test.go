package profile

import (
	"fmt"
	"path/filepath"
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
