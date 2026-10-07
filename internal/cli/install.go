package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/RBen19/devswitch/internal/provider"
	"github.com/spf13/cobra"
)

func installCommand() *cobra.Command {
	var yes bool
	var shell string
	command := &cobra.Command{
		Use: "install", Aliases: []string{"setup"}, Short: "Set up PATH, completion, aliases, and detected profiles",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sh, err := resolveShell(shell)
			if err != nil {
				return err
			}
			reader := bufio.NewReader(cmd.InOrStdin())
			out := cmd.OutOrStdout()
			settings, err := loadSettings()
			if err != nil {
				return err
			}
			if slices.Contains(settings.Shells, sh) {
				if err := refreshShells(cmd.Root(), settings); err != nil {
					return err
				}
				if err := refreshShares(out, settings); err != nil {
					return err
				}
				fmt.Fprintf(out, "devswitch is already set up for %s; shell integration refreshed.\n", sh.Name)
				return nil
			}
			fmt.Fprintf(out, "Shell: %s\nConfiguration: %s\n", sh.Name, sh.Config)
			store, err := getStore()
			if err != nil {
				return err
			}
			for _, id := range []string{"claude", "codex", "gemini"} {
				p, _ := provider.Parse(id)
				home, err := p.DefaultHome()
				if err != nil {
					return err
				}
				if p.Available() != nil {
					fmt.Fprintf(out, "%s: CLI not found in PATH\n", id)
				} else {
					fmt.Fprintf(out, "%s: CLI detected\n", id)
				}
				info, err := os.Stat(home)
				if err != nil || !info.IsDir() {
					continue
				}
				adopted := false
				for _, item := range store.Profiles {
					if item.Provider == p.ID && sameHome(item.Home, home) {
						adopted = true
					}
				}
				if adopted {
					continue
				}
				if _, err := store.Find(p.ID, "personal"); err == nil {
					fmt.Fprintf(out, "%s: found %s; use 'devswitch adopt %s <name>' to choose an unused name\n", id, home, id)
					continue
				}
				if _, err := store.Adopt(p, "personal", home, false); err != nil {
					return err
				}
				fmt.Fprintf(out, "Adopted %s/personal (existing login preserved).\n", id)
			}
			for _, id := range []provider.ID{provider.Claude, provider.Codex} {
				if yes || settings.Share[string(id)] != "" {
					continue
				}
				var names []string
				for _, item := range store.Profiles {
					if item.Provider == id && !item.NoShare {
						names = append(names, item.Name)
					}
				}
				if len(names) < 2 {
					continue
				}
				source := names[0]
				if slices.Contains(names, "personal") {
					source = "personal"
				}
				if !confirm(reader, out, fmt.Sprintf("Share sessions, memory, skills and agents of %s/%s with all other %s profiles, including new ones? Close running agents first.", id, source, id), false) {
					continue
				}
				p, _ := provider.Parse(string(id))
				if targets := slices.DeleteFunc(names, func(name string) bool { return name == source }); len(targets) > 0 {
					if err := shareAll(out, store, p, source, targets); err != nil {
						return err
					}
				}
				if settings.Share == nil {
					settings.Share = map[string]string{}
				}
				settings.Share[string(id)] = source
			}
			if !yes {
				fmt.Fprint(out, "Optional shortcut for devswitch (enter your own name, or press Enter to skip): ")
				line, err := reader.ReadString('\n')
				if err != nil && err != io.EOF {
					return err
				}
				if name := strings.TrimSpace(line); name != "" && !hasAlias(settings.Aliases, name) {
					if err := validateAlias(name); err != nil {
						return err
					}
					settings.Aliases = append(settings.Aliases, Alias{Name: name})
				}
			}
			if err := installShell(cmd.Root(), sh, settings.Aliases); err != nil {
				return err
			}
			settings.Shells = addShell(settings.Shells, sh)
			if sh.Name == "bash" {
				login, err := bashLoginShell()
				if err != nil {
					return err
				}
				if err := installShell(cmd.Root(), login, settings.Aliases); err != nil {
					return err
				}
				settings.Shells = addShell(settings.Shells, login)
				fmt.Fprintf(out, "Login shell: %s\n", login.Config)
			}
			if err := saveSettings(settings); err != nil {
				return err
			}
			fmt.Fprintf(out, "Ready. Open a new terminal, or run:\n  source %s\n\nNext: devswitch list\nAliases: devswitch alias --help\nHealth check: devswitch doctor\n", quoteForShell(sh.Name, sh.Config))
			return nil
		},
	}
	command.Flags().BoolVarP(&yes, "yes", "y", false, "set up completion and detected profiles without prompts or new aliases")
	command.Flags().StringVar(&shell, "shell", "", "shell to configure (bash, zsh, fish; default: SHELL)")
	return command
}

func confirm(in *bufio.Reader, out io.Writer, prompt string, defaultYes bool) bool {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	fmt.Fprintf(out, "%s %s ", prompt, suffix)
	var answer string
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(out)
		return false
	}
	_, _ = fmt.Sscan(line, &answer)
	return answer == "y" || answer == "Y" || answer == "yes" || (answer == "" && defaultYes)
}
