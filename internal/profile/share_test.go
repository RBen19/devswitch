package profile

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RBen19/devswitch/internal/provider"
)

func TestSharingPreservesDataAndIsIdempotent(t *testing.T) {
	for _, id := range []provider.ID{provider.Codex, provider.Claude} {
		t.Run(string(id), func(t *testing.T) {
			s := &Store{Root: t.TempDir()}
			p, _ := provider.Parse(string(id))
			source, err := s.Add(p, "personal")
			if err != nil {
				t.Fatal(err)
			}
			target, err := s.Add(p, "work")
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, filepath.Join(source.Home, "skills", "shared", "SKILL.md"), "shared")
			writeTestFile(t, filepath.Join(target.Home, "skills", "local", "SKILL.md"), "local")
			writeTestFile(t, filepath.Join(target.Home, "auth.json"), "target auth")
			writeTestFile(t, filepath.Join(source.Home, "auth.json"), "source auth")
			writeTestFile(t, filepath.Join(target.Home, "skills.devswitch-backup", "older"), "older")
			links, err := s.PlanShare(p, "personal", []string{"work", "work"}, nil)
			if err != nil {
				t.Fatal(err)
			}
			// Planning, including dry runs, must not create missing source paths.
			if _, err := os.Lstat(filepath.Join(source.Home, "rules")); !os.IsNotExist(err) {
				t.Fatal("planning changed source")
			}
			if err := ApplyShare(links); err != nil {
				t.Fatal(err)
			}
			assertTestFile(t, filepath.Join(target.Home, "skills", "shared", "SKILL.md"), "shared")
			assertTestFile(t, filepath.Join(target.Home, "skills.devswitch-backup.1", "local", "SKILL.md"), "local")
			assertTestFile(t, filepath.Join(target.Home, "skills.devswitch-backup", "older"), "older")
			assertTestFile(t, filepath.Join(target.Home, "auth.json"), "target auth")
			assertTestFile(t, filepath.Join(source.Home, "auth.json"), "source auth")
			writeTestFile(t, filepath.Join(target.Home, "rules", "new.md"), "new rule")
			assertTestFile(t, filepath.Join(source.Home, "rules", "new.md"), "new rule")
			for _, names := range [][]string{{"personal", "work"}, {"work", "personal"}} {
				again, err := s.PlanShare(p, names[0], names[1:], nil)
				if err != nil {
					t.Fatal(err)
				}
				if len(again) != 0 {
					t.Fatalf("repeat/reverse sharing produced %d links", len(again))
				}
			}
		})
	}
}

func TestSharingValidationAndSelection(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	p, _ := provider.Parse("codex")
	claude, _ := provider.Parse("claude")
	source, _ := s.Add(p, "personal")
	target, _ := s.Add(p, "work")
	_, _ = s.Add(claude, "claude-only")
	for _, request := range []struct{ targets, groups []string }{
		{[]string{"personal"}, nil},
		{[]string{"work", "missing"}, nil},
		{[]string{"claude-only"}, nil},
		{[]string{"work"}, []string{"auth.json"}},
		{nil, nil},
	} {
		if _, err := s.PlanShare(p, "personal", request.targets, request.groups); err == nil {
			t.Fatalf("accepted invalid request: %+v", request)
		}
	}
	links, err := s.PlanShare(p, "personal", []string{"work"}, []string{"skills"})
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 {
		t.Fatalf("expected skills only, got %+v", links)
	}
	if err := ApplyShare(links); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(target.Home, "sessions")); !os.IsNotExist(err) {
		t.Fatal("unselected sessions changed")
	}
	// Broken source links must fail before modifying targets.
	if err := os.Symlink(filepath.Join(source.Home, "missing"), filepath.Join(source.Home, "rules")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PlanShare(p, "personal", []string{"work"}, nil); err == nil {
		t.Fatal("accepted dangling source link")
	}
}

func TestShareRollback(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	target := filepath.Join(root, "target")
	writeTestFile(t, source, "shared")
	writeTestFile(t, target, "original")
	backup := target + ".devswitch-backup"
	err := ApplyShare([]ShareLink{
		{Source: source, Target: target, Backup: backup},
		{Source: source, Target: filepath.Join(root, "missing-parent", "target")},
	})
	if err == nil {
		t.Fatal("expected failure")
	}
	assertTestFile(t, target, "original")
	assertTestFile(t, source, "shared")
	if _, err := os.Lstat(backup); !os.IsNotExist(err) {
		t.Fatal("backup was not restored")
	}
}

func TestSharingAdoptedHomeAndMultipleTargets(t *testing.T) {
	s := &Store{Root: t.TempDir()}
	p, _ := provider.Parse("claude")
	home := t.TempDir()
	if _, err := s.Adopt(p, "personal", home); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(home, "CLAUDE.md"), "instructions")
	for _, name := range []string{"work", "client"} {
		if _, err := s.Add(p, name); err != nil {
			t.Fatal(err)
		}
	}
	links, err := s.PlanShare(p, "personal", []string{"work", "client"}, []string{"agents"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyShare(links); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"work", "client"} {
		item, _ := s.Find(p.ID, name)
		assertTestFile(t, filepath.Join(item.Home, "CLAUDE.md"), "instructions")
		link, err := os.Readlink(filepath.Join(item.Home, "agents"))
		if err != nil {
			t.Fatal(err)
		}
		resolvedHome, _ := filepath.EvalSymlinks(home)
		if link != filepath.Join(resolvedHome, "agents") {
			t.Fatalf("unexpected link: %s", link)
		}
	}
}

func TestSharingMergesCodexThreads(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not installed")
	}
	s := &Store{Root: t.TempDir()}
	p, _ := provider.Parse("codex")
	source, _ := s.Add(p, "personal")
	target, _ := s.Add(p, "work")
	for home, ids := range map[string]string{source.Home: "('a'),('b')", target.Home: "('b'),('c')"} {
		script := "PRAGMA journal_mode=WAL; CREATE TABLE threads (id TEXT PRIMARY KEY); INSERT INTO threads VALUES " + ids + ";"
		if err := sqlite(filepath.Join(home, "state_5.sqlite"), script); err != nil {
			t.Fatal(err)
		}
	}
	links, err := s.PlanShare(p, "personal", []string{"work"}, []string{"sessions"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyShare(links); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sqlite3", filepath.Join(target.Home, "state_5.sqlite"), "SELECT group_concat(id) FROM (SELECT id FROM threads ORDER BY id)").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "a,b,c" {
		t.Fatalf("threads: got %q", got)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertTestFile(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != expected {
		t.Fatalf("%s: got %q, want %q", path, data, expected)
	}
}
