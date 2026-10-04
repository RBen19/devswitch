package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"slices"

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
			if !yes && !confirm(reader, out, "Install PATH and tab completion?", true) {
				fmt.Fprintln(out, "Setup cancelled.")
				return nil
			}
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
				if yes || confirm(reader, out, fmt.Sprintf("Use existing %s configuration as %s/personal?", home, id), true) {
					if _, err := store.Adopt(p, "personal", home); err != nil {
						return err
					}
					fmt.Fprintf(out, "Adopted %s/personal (existing login preserved).\n", id)
				}
			}
			for _, id := range []provider.ID{provider.Claude, provider.Codex} {
				if yes || settings.Share[string(id)] != "" {
					continue
				}
				var names []string
				for _, item := range store.Profiles {
					if item.Provider == id {
						names = append(names, item.Name)
					}
				}
				if len(names) == 0 {
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
			if !hasAlias(settings.Aliases, "dvsw") && (yes || confirm(reader, out, "Add 'dvsw' as a shortcut for devswitch?", true)) {
				if err := validateAlias("dvsw"); err != nil {
					fmt.Fprintf(out, "Skipped dvsw: %v\n", err)
				} else {
					settings.Aliases = append(settings.Aliases, Alias{Name: "dvsw"})
				}
			}
			if !yes {
				for _, item := range store.Profiles {
					prefix := "cx"
					switch item.Provider {
					case provider.Claude:
						prefix = "cl"
					case provider.Gemini:
						prefix = "gm"
					}
					name := prefix + "-" + item.Name
					if hasAlias(settings.Aliases, name) {
						continue
					}
					if validateAlias(name) != nil {
						continue
					}
					if confirm(reader, out, fmt.Sprintf("Add '%s' to launch %s/%s?", name, item.Provider, item.Name), false) {
						settings.Aliases = append(settings.Aliases, Alias{Name: name, Provider: string(item.Provider), Profile: item.Name})
					}
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
	command.Flags().BoolVarP(&yes, "yes", "y", false, "accept defaults: completion, detected profiles, and dvsw shortcut")
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
