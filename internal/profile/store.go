package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/RBen19/devswitch/internal/fileutil"

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
	seen := make(map[string]bool)
	for _, item := range store.Profiles {
		if _, err := provider.Parse(string(item.Provider)); err != nil {
			return nil, fmt.Errorf("invalid profile registry: %w", err)
		}
		if err := validateName(item.Name); err != nil {
			return nil, fmt.Errorf("invalid profile registry: %w", err)
		}
		if item.Home == "" || !filepath.IsAbs(item.Home) {
			return nil, fmt.Errorf("invalid profile registry: %s/%s needs an absolute home directory", item.Provider, item.Name)
		}
		key := string(item.Provider) + "/" + item.Name
		if seen[key] {
			return nil, fmt.Errorf("invalid profile registry: duplicate %s", key)
		}
		seen[key] = true
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
	if err := fileutil.Write(filepath.Join(s.Root, "profiles.json"), data, 0o600); err != nil {
		return fmt.Errorf("save profiles: %w", err)
	}
	return nil
}

func (s *Store) Add(p provider.Provider, name string) (Profile, error) {
	return s.create(p, name, "")
}

func (s *Store) Adopt(p provider.Provider, name, home string) (Profile, error) {
	absolute, err := filepath.Abs(home)
	if err != nil {
		return Profile{}, err
	}
	return s.create(p, name, absolute)
}

func validateName(name string) error {
	if name == "" || name == "." || name == ".." || strings.HasPrefix(name, "-") || strings.ContainsAny(name, `/\\`) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return errors.New("profile name must be simple, for example: personal or work (no path components or control characters)")
	}
	return nil
}

func (s *Store) create(p provider.Provider, name, adoptedHome string) (Profile, error) {
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return Profile{}, err
	}
	if _, err := provider.Parse(string(p.ID)); err != nil {
		return Profile{}, err
	}
	var item Profile
	err := fileutil.WithLock(filepath.Join(s.Root, "profiles.lock"), func() error {
		current, err := Load(s.Root)
		if err != nil {
			return err
		}
		if _, err := current.Find(p.ID, name); err == nil {
			return fmt.Errorf("profile %s/%s already exists", p.ID, name)
		}
		home := adoptedHome
		created := false
		if home == "" {
			home, err = filepath.Abs(filepath.Join(s.Root, "profiles", string(p.ID), name))
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(home), 0o700); err != nil {
				return err
			}
			if err := os.Mkdir(home, 0o700); err != nil {
				return fmt.Errorf("create profile (existing directories are never reused): %w", err)
			}
			created = true
		} else {
			if info, err := os.Stat(home); err != nil || !info.IsDir() {
				return fmt.Errorf("existing provider configuration not found at %s", home)
			}
			canonical, err := filepath.EvalSymlinks(home)
			if err != nil {
				return err
			}
			for _, profile := range current.Profiles {
				existing, err := filepath.EvalSymlinks(profile.Home)
				if err == nil && existing == canonical {
					return fmt.Errorf("directory already belongs to %s/%s", profile.Provider, profile.Name)
				}
			}
		}
		item = Profile{Provider: p.ID, Name: name, Home: home}
		current.Profiles = append(current.Profiles, item)
		sort.Slice(current.Profiles, func(i, j int) bool {
			if current.Profiles[i].Provider == current.Profiles[j].Provider {
				return current.Profiles[i].Name < current.Profiles[j].Name
			}
			return current.Profiles[i].Provider < current.Profiles[j].Provider
		})
		if err := current.Save(); err != nil {
			if created {
				_ = os.Remove(home)
			}
			return err
		}
		s.Profiles = current.Profiles
		return nil
	})
	return item, err
}

func (s *Store) Find(p provider.ID, name string) (Profile, error) {
	for _, item := range s.Profiles {
		if item.Provider == p && item.Name == name {
			return item, nil
		}
	}
	return Profile{}, fmt.Errorf("profile not found: %s/%s", p, name)
}
