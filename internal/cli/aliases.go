package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/RBen19/devswitch/internal/fileutil"
	"github.com/RBen19/devswitch/internal/profile"
	"github.com/RBen19/devswitch/internal/provider"
	"github.com/spf13/cobra"
)

type Alias struct {
	Name     string `json:"name"`
	Provider string `json:"provider,omitempty"`
	Profile  string `json:"profile,omitempty"`
}
type shellSettings struct {
	Aliases []Alias         `json:"aliases"`
	Shells  []shellLocation `json:"shells"`
}

func loadSettings() (shellSettings, error) {
	var settings shellSettings
	root, err := profile.DefaultRoot()
	if err != nil {
		return settings, err
	}
	data, err := os.ReadFile(filepath.Join(root, "shell.json"))
	if os.IsNotExist(err) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return settings, fmt.Errorf("read shell settings: %w", err)
	}
	for _, a := range settings.Aliases {
		if !aliasPattern.MatchString(a.Name) {
			return settings, fmt.Errorf("invalid stored alias %q", a.Name)
		}
	}
	for _, sh := range settings.Shells {
		if sh.Name != "bash" && sh.Name != "zsh" && sh.Name != "fish" {
			return settings, fmt.Errorf("invalid stored shell %q", sh.Name)
		}
	}
	return settings, nil
}
func saveSettings(settings shellSettings) error {
	root, err := profile.DefaultRoot()
	if err != nil {
		return err
	}
	sort.Slice(settings.Aliases, func(i, j int) bool { return settings.Aliases[i].Name < settings.Aliases[j].Name })
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return fileutil.Write(filepath.Join(root, "shell.json"), append(data, '\n'), 0o600)
}
func addShell(shells []shellLocation, sh shellLocation) []shellLocation {
	for _, item := range shells {
		if item == sh {
			return shells
		}
	}
	return append(shells, sh)
}
func hasAlias(aliases []Alias, name string) bool {
	for _, a := range aliases {
		if a.Name == name {
			return true
		}
	}
	return false
}

var aliasPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

func validateAlias(name string) error {
	if !aliasPattern.MatchString(name) {
		return fmt.Errorf("alias must start with a letter and contain only letters, digits, '-' or '_'")
	}
	reserved := map[string]bool{"devswitch": true, "claude": true, "codex": true, "cd": true, "source": true, "alias": true, "unalias": true, "exec": true, "exit": true, "export": true, "set": true, "function": true, "if": true, "then": true, "else": true, "fi": true, "for": true, "while": true, "do": true, "done": true, "return": true, "eval": true, "test": true, "command": true, "type": true, "read": true, "printf": true, "echo": true, "true": true, "false": true}
	if reserved[name] {
		return fmt.Errorf("%q is a reserved command", name)
	}
	if _, err := exec.LookPath(name); err == nil {
		return fmt.Errorf("%q already exists in PATH; choose another name", name)
	}
	return nil
}
func refreshShells(root *cobra.Command, settings shellSettings) error {
	if len(settings.Shells) == 0 {
		return fmt.Errorf("shell integration is not installed; run 'devswitch install' first")
	}
	for _, sh := range settings.Shells {
		if err := installShell(root, sh, settings.Aliases); err != nil {
			return err
		}
	}
	return saveSettings(settings)
}
func aliasCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "alias", Short: "Create, list, or remove your own shell shortcuts"}
	cmd.AddCommand(&cobra.Command{
		Use: "add <name> [<claude|codex|gemini> <profile>]", Short: "Add a devswitch shortcut or a profile launcher",
		Example: "  devswitch alias add ds\n  devswitch alias add work-ai codex work",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 && len(args) != 3 {
				return fmt.Errorf("use: devswitch alias add <name> [<claude|codex|gemini> <profile>]")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateAlias(args[0]); err != nil {
				return err
			}
			settings, err := loadSettings()
			if err != nil {
				return err
			}
			if hasAlias(settings.Aliases, args[0]) {
				return fmt.Errorf("alias %q already exists; remove it first", args[0])
			}
			alias := Alias{Name: args[0]}
			if len(args) == 3 {
				p, err := provider.Parse(args[1])
				if err != nil {
					return err
				}
				store, err := getStore()
				if err != nil {
					return err
				}
				if _, err := store.Find(p.ID, args[2]); err != nil {
					return err
				}
				alias.Provider, alias.Profile = args[1], args[2]
			}
			settings.Aliases = append(settings.Aliases, alias)
			if err := refreshShells(cmd.Root(), settings); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added %s. Open a new terminal to use it.\n", alias.Name)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List managed shell shortcuts", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		settings, err := loadSettings()
		if err != nil {
			return err
		}
		if len(settings.Aliases) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No aliases. Create one with: devswitch alias add ds")
			return nil
		}
		for _, a := range settings.Aliases {
			target := "devswitch"
			if a.Provider != "" {
				target += " run " + a.Provider + " " + a.Profile
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s → %s\n", a.Name, target)
		}
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "remove <name>", Aliases: []string{"rm"}, Short: "Remove a managed shell shortcut", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		settings, err := loadSettings()
		if err != nil {
			return err
		}
		if !hasAlias(settings.Aliases, args[0]) {
			return fmt.Errorf("alias %q not found", args[0])
		}
		var kept []Alias
		for _, a := range settings.Aliases {
			if a.Name != args[0] {
				kept = append(kept, a)
			}
		}
		settings.Aliases = kept
		if err := refreshShells(cmd.Root(), settings); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed %s. In this terminal run: unalias %s\n", args[0], args[0])
		return nil
	}})
	return cmd
}
