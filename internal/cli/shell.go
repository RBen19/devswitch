package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RBen19/devswitch/internal/fileutil"
	"github.com/RBen19/devswitch/internal/profile"
	"github.com/spf13/cobra"
)

const pathMarker = "# devswitch: managed PATH" // pre-0.2 migration
const shellStart = "# >>> devswitch >>>"
const shellEnd = "# <<< devswitch <<<"

type shellLocation struct{ Name, Config string }

func resolveShell(name string) (shellLocation, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return shellLocation{}, err
	}
	if name == "" {
		name = filepath.Base(os.Getenv("SHELL"))
	}
	switch name {
	case "zsh":
		dir := os.Getenv("ZDOTDIR")
		if dir == "" {
			dir = home
		}
		return shellLocation{name, filepath.Join(dir, ".zshrc")}, nil
	case "bash":
		return shellLocation{name, filepath.Join(home, ".bashrc")}, nil
	case "fish":
		dir := os.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			dir = filepath.Join(home, ".config")
		}
		return shellLocation{name, filepath.Join(dir, "fish", "conf.d", "devswitch.fish")}, nil
	default:
		return shellLocation{}, fmt.Errorf("cannot configure shell %q; choose --shell bash, zsh, or fish", name)
	}
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
func quoteForShell(sh, s string) string {
	if sh == "fish" {
		return "'" + strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(s) + "'"
	}
	return shellQuote(s)
}

// stripIntegration preserves everything outside exactly delimited managed blocks.
func stripIntegration(data string) (string, error) {
	lines := strings.SplitAfter(data, "\n")
	var out strings.Builder
	inside := false
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == shellStart {
			if inside {
				return "", fmt.Errorf("nested devswitch shell block; repair the markers before retrying")
			}
			inside = true
			continue
		}
		if line == shellEnd {
			if !inside {
				return "", fmt.Errorf("unexpected devswitch shell end marker")
			}
			inside = false
			continue
		}
		if inside {
			continue
		}
		if line == pathMarker {
			// Migrate only the exact two lines emitted by the old installer.
			if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "export PATH=\"") && strings.HasSuffix(strings.TrimSpace(lines[i+1]), ":$PATH\"") {
				i++
			}
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "alias dvsw='devswitch'" {
				i++
			}
			continue
		}
		out.WriteString(lines[i])
	}
	if inside {
		return "", fmt.Errorf("unterminated devswitch shell block; repair the markers before retrying")
	}
	return out.String(), nil
}

func installShell(root *cobra.Command, sh shellLocation, aliases []Alias) error {
	stateRoot, err := profile.DefaultRoot()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	completionPath := filepath.Join(stateRoot, "shell", "completion."+sh.Name)
	var completion bytes.Buffer
	switch sh.Name {
	case "bash":
		err = root.GenBashCompletionV2(&completion, true)
	case "zsh":
		err = root.GenZshCompletion(&completion)
	case "fish":
		err = root.GenFishCompletion(&completion, true)
	}
	if err != nil {
		return err
	}
	data, err := os.ReadFile(sh.Config)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	clean, err := stripIntegration(string(data))
	if err != nil {
		return fmt.Errorf("%s: %w", sh.Config, err)
	}
	var block strings.Builder
	q := func(s string) string { return quoteForShell(sh.Name, s) }
	block.WriteString(shellStart + "\n")
	if sh.Name == "bash" {
		block.WriteString("if [ -n \"${BASH_VERSION:-}\" ]; then\n")
	}
	if sh.Name == "fish" {
		fmt.Fprintf(&block, "fish_add_path --prepend %s\n", q(filepath.Dir(exe)))
	} else {
		fmt.Fprintf(&block, "case \":$PATH:\" in *%s*) ;; *) export PATH=%s:\"$PATH\" ;; esac\n", q(":"+filepath.Dir(exe)+":"), q(filepath.Dir(exe)))
	}
	if sh.Name == "zsh" {
		block.WriteString("if ! (( $+functions[compdef] )); then\n  autoload -Uz compinit && compinit\nfi\n")
	}
	fmt.Fprintf(&block, "source %s\n", q(completionPath))
	for _, alias := range aliases {
		var args []string
		args = append(args, q(exe))
		if alias.Provider != "" {
			args = append(args, "run", q(alias.Provider), q(alias.Profile), "--")
		}
		body := strings.Join(args, " ")
		if sh.Name == "fish" {
			fmt.Fprintf(&block, "if not type -q %s\n", alias.Name)
		} else {
			fmt.Fprintf(&block, "if ! command -v %s >/dev/null 2>&1; then\n", alias.Name)
		}
		fmt.Fprintf(&block, "alias %s=%s\n", alias.Name, q(body))
		if alias.Provider == "" {
			switch sh.Name {
			case "bash":
				fmt.Fprintf(&block, "complete -o default -F __start_devswitch %s\n", alias.Name)
			case "zsh":
				fmt.Fprintf(&block, "compdef _devswitch %s\n", alias.Name)
			case "fish":
				fmt.Fprintf(&block, "complete -c %s -w devswitch\n", alias.Name)
			}
		}
		if sh.Name == "fish" {
			block.WriteString("end\n")
		} else {
			block.WriteString("fi\n")
		}
	}
	if sh.Name == "bash" {
		block.WriteString("fi\n")
	}
	block.WriteString(shellEnd + "\n")
	if clean != "" && !strings.HasSuffix(clean, "\n") {
		clean += "\n"
	}
	if err := fileutil.Write(completionPath, completion.Bytes(), 0o600); err != nil {
		return err
	}
	return fileutil.Write(sh.Config, []byte(clean+block.String()), 0o600)
}

func removePathIntegration(configFile string) error {
	data, err := os.ReadFile(configFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	clean, err := stripIntegration(string(data))
	if err != nil {
		return err
	}
	return fileutil.Write(configFile, []byte(clean), 0o600)
}

func bashLoginShell() (shellLocation, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return shellLocation{}, err
	}
	for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
		path := filepath.Join(home, name)
		if _, err := os.Stat(path); err == nil {
			return shellLocation{Name: "bash", Config: path}, nil
		} else if !os.IsNotExist(err) {
			return shellLocation{}, err
		}
	}
	return shellLocation{Name: "bash", Config: filepath.Join(home, ".bash_profile")}, nil
}
