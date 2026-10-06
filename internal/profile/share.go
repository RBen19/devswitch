package profile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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
				if entry.Group != "agents" {
					continue
				}
				found := false
				for _, home := range homes {
					if target, err := os.Stat(filepath.Join(home, entry.Name)); err == nil && target.Mode().IsRegular() {
						found = true
						break
					}
				}
				if !found {
					continue
				}
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

// ApplyShare merges each replaced target into the source and keeps the original
// as a backup. On failure, links are rolled back; copies already merged remain.
func ApplyShare(links []ShareLink) (err error) {
	for _, link := range links {
		if link.Directory || filepath.Ext(link.Source) != ".md" {
			continue
		}
		if _, statErr := os.Lstat(link.Source); errors.Is(statErr, os.ErrNotExist) {
			if target, statErr := os.Stat(link.Target); statErr == nil && target.Mode().IsRegular() {
				if err := copyFile(link.Target, link.Source, os.O_CREATE|os.O_EXCL|os.O_WRONLY, target.Mode().Perm()); err != nil {
					return err
				}
			}
		}
	}
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
		if link.Backup != "" && filepath.Ext(link.Target) == ".sqlite" {
			// Fold the -wal file into the database, which would be orphaned by the rename.
			if err = sqlite(link.Target, "PRAGMA wal_checkpoint(TRUNCATE);"); err != nil {
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
		if link.Backup != "" {
			if err = mergeInto(link.Backup, link.Source); err != nil {
				return fmt.Errorf("merge %s: %w", link.Target, err)
			}
		}
	}
	return nil
}

// mergeInto copies from into to without overwriting: directories merge
// recursively, JSONL histories are appended, other clashes stay only in from.
func mergeInto(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	existing, err := os.Lstat(to)
	missing := errors.Is(err, os.ErrNotExist)
	if err != nil && !missing {
		return err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		if !missing {
			return nil
		}
		target, err := os.Readlink(from)
		if err != nil {
			return err
		}
		return os.Symlink(target, to)
	case info.IsDir():
		if missing {
			if err := os.Mkdir(to, info.Mode().Perm()); err != nil {
				return err
			}
		} else if !existing.IsDir() {
			return nil
		}
		entries, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := mergeInto(filepath.Join(from, entry.Name()), filepath.Join(to, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	case !info.Mode().IsRegular():
		return nil
	case missing:
		return copyFile(from, to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	case existing.Mode().IsRegular() && filepath.Ext(to) == ".jsonl":
		return copyFile(from, to, os.O_APPEND|os.O_WRONLY, 0)
	case existing.Mode().IsRegular() && filepath.Ext(to) == ".sqlite":
		return sqlite(to, "ATTACH "+sqlQuote(from)+" AS other; INSERT OR IGNORE INTO threads SELECT * FROM other.threads;")
	}
	return nil
}

// sqlite uses the system sqlite3 CLI to avoid embedding a database driver.
func sqlite(db, script string) error {
	out, err := exec.Command("sqlite3", db, script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sqlite3 %s: %w: %s", db, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func sqlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func copyFile(from, to string, flag int, perm os.FileMode) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, flag, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return errors.Join(err, out.Close())
}
