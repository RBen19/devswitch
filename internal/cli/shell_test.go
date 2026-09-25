package cli

import (
	"bytes"
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
	if _, err := store.Add(codex, "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(claude, "private"); err != nil {
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
