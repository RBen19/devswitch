package profile

import (
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
	created, err := store.Add(provider.Provider{ID: provider.Claude}, "perso")
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
