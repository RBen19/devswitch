package cli

import (
	"bytes"
	"encoding/json"
	"github.com/RBen19/devswitch/internal/provider"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedShellBlocks(t *testing.T) {
	original := "# custom\nexport MY_SETTING=yes\nalias mine='echo preserved'\n"
	legacy := original + pathMarker + "\nexport PATH=\"/old:$PATH\"\nalias dvsw='devswitch'\necho tail\n"
	cleaned, err := stripIntegration(legacy)
	if err != nil || cleaned != original+"echo tail\n" {
		t.Fatalf("migration: %q %v", cleaned, err)
	}
	current := original + shellStart + "\nmanaged\n" + shellEnd + "\necho tail\n"
	cleaned, err = stripIntegration(current)
	if err != nil || cleaned != original+"echo tail\n" {
		t.Fatalf("cleanup: %q %v", cleaned, err)
	}
	for _, invalid := range []string{shellStart + "\n", shellEnd + "\n", shellStart + "\n" + shellStart + "\n" + shellEnd + "\n"} {
		if _, err := stripIntegration(invalid); err == nil {
			t.Fatalf("accepted malformed block %q", invalid)
		}
	}
}
func TestInstallShellIdempotenceAndSymlinkedConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"bash", "zsh", "fish"} {
		t.Run(name, func(t *testing.T) {
			actual := filepath.Join(home, name+"-dotfile")
			path := filepath.Join(home, name+"-config")
			if err := os.WriteFile(actual, []byte("# user configuration\n"), 0o640); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(actual, path); err != nil {
				t.Fatal(err)
			}
			sh := shellLocation{Name: name, Config: path}
			for i := 0; i < 2; i++ {
				if err := installShell(NewRootCommand(), sh, []Alias{{Name: "ds"}}); err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(actual)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(data), shellStart) != 1 || !strings.HasPrefix(string(data), "# user configuration\n") {
				t.Fatalf("unexpected shell config: %s", data)
			}
			if _, err := os.Readlink(path); err != nil {
				t.Fatal("replaced symlink", err)
			}
			info, _ := os.Stat(actual)
			if info.Mode().Perm() != 0o640 {
				t.Fatal("changed permissions")
			}
			if err := removePathIntegration(path); err != nil {
				t.Fatal(err)
			}
			data, _ = os.ReadFile(actual)
			if string(data) != "# user configuration\n" {
				t.Fatalf("removed user config: %s", data)
			}
		})
	}
}
func TestShellSelection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", filepath.Join(home, "zsh"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	for name, want := range map[string]string{"bash": filepath.Join(home, ".bashrc"), "zsh": filepath.Join(home, "zsh", ".zshrc"), "fish": filepath.Join(home, "config", "fish", "conf.d", "devswitch.fish")} {
		sh, err := resolveShell(name)
		if err != nil || sh.Config != want {
			t.Fatalf("%s: %+v %v", name, sh, err)
		}
	}
	if _, err := resolveShell("invalid"); err == nil {
		t.Fatal("accepted unsupported shell")
	}
}

func TestCompletionUsesProviderProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := getStore()
	if err != nil {
		t.Fatal(err)
	}
	codex, _ := provider.Parse("codex")
	claude, _ := provider.Parse("claude")
	if _, err := store.Add(codex, "work", false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(claude, "private", false); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"__complete", "run", "codex", ""}, {"__complete", "alias", "add", "my-ai", "codex", ""}, {"__complete", "cx", ""}} {
		root := NewRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "work") || strings.Contains(out.String(), "private") {
			t.Fatalf("wrong candidates for %v: %s", args, out.String())
		}
	}
}

func TestSetupUsesOnlyChosenShortcut(t *testing.T) {
	for _, input := range []string{"my-switch\n", "\n", "noninteractive"} {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if err := os.Mkdir(filepath.Join(home, ".claude"), 0o700); err != nil {
				t.Fatal(err)
			}
			args := []string{"install", "--shell", "bash"}
			if input == "noninteractive" {
				args = append(args, "--yes")
			}
			out, _ := runTestCLI(t, input, args...)
			settings, err := loadSettings()
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if input == "my-switch\n" {
				want = 1
				if !hasAlias(settings.Aliases, "my-switch") {
					t.Fatal("chosen shortcut was not saved")
				}
			}
			if len(settings.Aliases) != want {
				t.Fatalf("unexpected aliases: %+v", settings.Aliases)
			}
			if strings.Contains(out, "[y/N]") || strings.Contains(out, "[Y/n]") {
				t.Fatalf("single-account setup asked extra questions: %s", out)
			}
			out, _ = runTestCLI(t, "", args...)
			if strings.Contains(out, "Optional shortcut") {
				t.Fatal("update prompted again")
			}
		})
	}
}

func TestDoctorRepairsBeforeJSONAndPreservesIsolation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	bin := filepath.Join(home, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	runTestCLI(t, "", "add", "claude", "personal")
	runTestCLI(t, "", "add", "claude", "work")
	runTestCLI(t, "", "add", "claude", "solo", "--no-share")
	if err := os.Mkdir(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	runTestCLI(t, "", "adopt", "claude", "adopted", "--no-share")
	runTestCLI(t, "", "install", "--yes", "--shell", "bash")
	settings, _ := loadSettings()
	settings.Share = map[string]string{"claude": "personal"}
	if err := saveSettings(settings); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(home, ".devswitch", "profiles", "claude", "work")
	if err := os.Symlink(filepath.Join(home, "missing"), filepath.Join(work, "agents")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		out, stderr := runTestCLI(t, "", "doctor", "--fix", "--json")
		var checks []healthCheck
		if err := json.Unmarshal([]byte(out), &checks); err != nil {
			t.Fatalf("invalid JSON: %v: %s", err, out)
		}
		for _, check := range checks {
			if !check.OK {
				t.Fatalf("stale failure: %+v", check)
			}
		}
		if i == 0 && !strings.Contains(stderr, "Shared ") {
			t.Fatal("repair messages missing from stderr")
		}
	}
	runTestCLI(t, "", "install", "--yes", "--shell", "bash")
	for _, dir := range []string{filepath.Join(home, ".claude"), filepath.Join(home, ".devswitch", "profiles", "claude", "solo")} {
		if _, err := os.Lstat(filepath.Join(dir, "agents")); !os.IsNotExist(err) {
			t.Fatalf("isolated profile was shared: %s", dir)
		}
	}
}

func runTestCLI(t *testing.T, input string, args ...string) (string, string) {
	t.Helper()
	cmd := NewRootCommand()
	var out, stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s\n%s", args, err, &out, &stderr)
	}
	return out.String(), stderr.String()
}
