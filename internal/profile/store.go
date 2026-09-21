package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/RBen19/devswitch/internal/provider"
)

type Profile struct {
	Provider provider.ID `json:"provider"`
	Name     string      `json:"name"`
	Home     string      `json:"home"`
}

type Store struct {
	Root     string    `json:"-"`
	Profiles []Profile `json:"profiles"`
}

func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".devswitch"), nil
}

func Load(root string) (*Store, error) {
	store := &Store{Root: root}
	data, err := os.ReadFile(filepath.Join(root, "profiles.json"))
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read profile registry: %w", err)
	}
	if err := json.Unmarshal(data, store); err != nil {
		return nil, fmt.Errorf("decode profile registry: %w", err)
	}
	store.Root = root
	return store, nil
}

func (s *Store) Save() error {
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return fmt.Errorf("create devswitch directory: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profiles: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(s.Root, "profiles.json"), data, 0o600); err != nil {
		return fmt.Errorf("save profiles: %w", err)
	}
	return nil
}

func (s *Store) Add(p provider.Provider, name string) (Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, `/\\`) {
		return Profile{}, errors.New("profile name must be simple, for example: personal or work")
	}
	if _, err := s.Find(p.ID, name); err == nil {
		return Profile{}, fmt.Errorf("profile %s/%s already exists", p.ID, name)
	}
	home := filepath.Join(s.Root, "profiles", string(p.ID), name)
	if err := os.MkdirAll(home, 0o700); err != nil {
		return Profile{}, fmt.Errorf("create profile: %w", err)
	}
	item := Profile{Provider: p.ID, Name: name, Home: home}
	s.Profiles = append(s.Profiles, item)
	sort.Slice(s.Profiles, func(i, j int) bool {
		if s.Profiles[i].Provider == s.Profiles[j].Provider {
			return s.Profiles[i].Name < s.Profiles[j].Name
		}
		return s.Profiles[i].Provider < s.Profiles[j].Provider
	})
	return item, s.Save()
}

func (s *Store) Adopt(p provider.Provider, name, home string) (Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, `/\\`) {
		return Profile{}, errors.New("profile name must be simple, for example: personal or work")
	}
	if _, err := s.Find(p.ID, name); err == nil {
		return Profile{}, fmt.Errorf("profile %s/%s already exists", p.ID, name)
	}
	if info, err := os.Stat(home); err != nil || !info.IsDir() {
		return Profile{}, fmt.Errorf("existing provider configuration not found at %s", home)
	}
	item := Profile{Provider: p.ID, Name: name, Home: home}
	s.Profiles = append(s.Profiles, item)
	sort.Slice(s.Profiles, func(i, j int) bool {
		if s.Profiles[i].Provider == s.Profiles[j].Provider {
			return s.Profiles[i].Name < s.Profiles[j].Name
		}
		return s.Profiles[i].Provider < s.Profiles[j].Provider
	})
	return item, s.Save()
}

func (s *Store) Find(p provider.ID, name string) (Profile, error) {
	for _, item := range s.Profiles {
		if item.Provider == p && item.Name == name {
			return item, nil
		}
	}
	return Profile{}, fmt.Errorf("profile not found: %s/%s", p, name)
}
