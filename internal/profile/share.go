package profile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RBen19/devswitch/internal/provider"
)

type ShareLink struct {
	Source    string
	Target    string
	Backup    string
	Directory bool
}

// PlanShare validates the complete request without changing any files.
func (s *Store) PlanShare(p provider.Provider, sourceName string, targetNames, groups []string) ([]ShareLink, error) {
	paths, err := p.SharedPaths(groups)
	if err != nil {
		return nil, err
	}
	if len(targetNames) == 0 {
		return nil, errors.New("at least one target profile is required")
	}
	source, err := s.Find(p.ID, sourceName)
	if err != nil {
		return nil, err
	}
	sourceHome, err := realHome(source.Home)
	if err != nil {
		return nil, err
	}
	var homes []string
	seen := make(map[string]bool)
	for _, name := range targetNames {
		target, err := s.Find(p.ID, name)
		if err != nil {
			return nil, err
		}
		home, err := realHome(target.Home)
		if err != nil {
			return nil, err
		}
		if overlaps(home, sourceHome) {
			return nil, fmt.Errorf("source and target profile homes overlap: %s", home)
		}
		if !seen[home] {
			homes = append(homes, home)
			seen[home] = true
		}
	}
	var links []ShareLink
	for _, entry := range paths {
		src := filepath.Join(sourceHome, entry.Name)
		info, err := os.Lstat(src)
		if errors.Is(err, os.ErrNotExist) {
			if !entry.Create {
				continue
			}
		} else if err != nil {
			return nil, err
		} else {
			// Resolve existing links once so reverse sharing cannot create a cycle.
			src, err = filepath.EvalSymlinks(src)
			if err != nil {
				return nil, fmt.Errorf("resolve source %s: %w", entry.Name, err)
			}
			info, err = os.Stat(src)
			if err != nil {
				return nil, err
			}
			if info.IsDir() != entry.Directory || (!info.IsDir() && !info.Mode().IsRegular()) {
				return nil, fmt.Errorf("unexpected file type at %s", src)
			}
		}
		for _, home := range homes {
			dst := filepath.Join(home, entry.Name)
			resolved, resolveErr := filepath.EvalSymlinks(dst)
			if resolveErr == nil && resolved == src {
				continue
			}
			if overlaps(src, dst) {
				return nil, fmt.Errorf("sharing would create overlapping paths: %s and %s", src, dst)
			}
			link := ShareLink{Source: src, Target: dst, Directory: entry.Directory}
			if _, err := os.Lstat(dst); err == nil {
				link.Backup, err = unusedBackup(dst)
				if err != nil {
					return nil, err
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			links = append(links, link)
		}
	}
	// No destination may contain another link's source or destination.
	for i, a := range links {
		for j, b := range links {
			if i != j && (overlaps(a.Target, b.Source) || overlaps(a.Target, b.Target)) {
				return nil, fmt.Errorf("sharing paths overlap at %s", a.Target)
			}
		}
	}
	return links, nil
}

func realHome(home string) (string, error) {
	absolute, err := filepath.Abs(home)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("profile home is not a directory: %s", home)
	}
	return resolved, nil
}

func overlaps(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+string(os.PathSeparator)) || strings.HasPrefix(b, a+string(os.PathSeparator))
}

func unusedBackup(path string) (string, error) {
	for n := 0; ; n++ {
		candidate := path + ".devswitch-backup"
		if n > 0 {
			candidate = fmt.Sprintf("%s.%d", candidate, n)
		}
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
	}
}

// ApplyShare rolls back completed replacements if any link fails. Newly created
// empty source paths may remain; no original profile data is removed.
func ApplyShare(links []ShareLink) (err error) {
	var completed []ShareLink
	defer func() {
		if err == nil {
			return
		}
		for i := len(completed) - 1; i >= 0; i-- {
			link := completed[i]
			if removeErr := os.Remove(link.Target); removeErr != nil {
				err = errors.Join(err, removeErr)
				continue
			}
			if link.Backup != "" {
				err = errors.Join(err, os.Rename(link.Backup, link.Target))
			}
		}
	}()
	for _, link := range links {
		if link.Directory {
			if err = os.MkdirAll(link.Source, 0o700); err != nil {
				return err
			}
		} else {
			file, createErr := os.OpenFile(link.Source, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if createErr == nil {
				err = file.Close()
			} else if !errors.Is(createErr, os.ErrExist) {
				err = createErr
			}
			if err != nil {
				return err
			}
		}
		if link.Backup != "" {
			if _, statErr := os.Lstat(link.Backup); !errors.Is(statErr, os.ErrNotExist) {
				return fmt.Errorf("backup path is no longer available: %s", link.Backup)
			}
			if err = os.Rename(link.Target, link.Backup); err != nil {
				return err
			}
		}
		if err = os.Symlink(link.Source, link.Target); err != nil {
			if link.Backup != "" {
				err = errors.Join(err, os.Rename(link.Backup, link.Target))
			}
			return err
		}
		completed = append(completed, link)
	}
	return nil
}
