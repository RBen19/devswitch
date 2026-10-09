package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/RBen19/devswitch/internal/fileutil"
	"github.com/RBen19/devswitch/internal/profile"
	"github.com/RBen19/devswitch/internal/provider"
	"github.com/spf13/cobra"
)

// launchSettings stores provider-specific default arguments. They are always
// passed before arguments from the command line, so an explicit CLI argument
// can override a configured default when the provider supports that behavior.
type launchSettings struct {
	Providers map[string]providerLaunchSettings `json:"providers"`
}

type providerLaunchSettings struct {
	Args []string `json:"args"`
}

func launchSettingsPath() (string, error) {
	root, err := profile.DefaultRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "settings.json"), nil
}

func loadLaunchSettings() (launchSettings, error) {
	settings := launchSettings{Providers: map[string]providerLaunchSettings{}}
	path, err := launchSettingsPath()
	if err != nil {
		return settings, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return settings, fmt.Errorf("read launch settings: %w", err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return settings, fmt.Errorf("read launch settings: %w", err)
	}
	if settings.Providers == nil {
		settings.Providers = map[string]providerLaunchSettings{}
	}
	for id := range settings.Providers {
		if _, err := provider.Parse(id); err != nil {
			return settings, fmt.Errorf("invalid settings.json: %w", err)
		}
	}
	return settings, nil
}

func saveLaunchSettings(settings launchSettings) error {
	path, err := launchSettingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create devswitch directory: %w", err)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode launch settings: %w", err)
	}
	if err := fileutil.Write(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("save launch settings: %w", err)
	}
	return nil
}

func settingsCommand() *cobra.Command {
	command := &cobra.Command{Use: "settings", Short: "Manage provider launch defaults"}
	command.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Show the provider settings file location",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := launchSettingsPath()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), path)
			return err
		},
	})
	command.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create a provider settings file with empty argument lists",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := launchSettingsPath()
			if err != nil {
				return err
			}
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("settings file already exists: %s", path)
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			settings := launchSettings{Providers: map[string]providerLaunchSettings{
				"claude": {Args: []string{}},
				"codex":  {Args: []string{}},
				"gemini": {Args: []string{}},
			}}
			if err := saveLaunchSettings(settings); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s\nEdit the args arrays to set provider defaults.\n", path)
			return nil
		},
	})
	return command
}

// optionsCommand asks the installed provider CLI for its own current help
// instead of maintaining a potentially stale copy of its option list.
func optionsCommand() *cobra.Command {
	return &cobra.Command{
		Use:       "options <claude|codex|gemini>",
		Short:     "Show options supported by the installed provider CLI",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"claude", "codex", "gemini"},
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := provider.Parse(args[0])
			if err != nil {
				return err
			}
			if err := p.Available(); err != nil {
				return err
			}
			call := exec.CommandContext(cmd.Context(), p.Binary, "--help")
			call.Stdout = cmd.OutOrStdout()
			call.Stderr = cmd.ErrOrStderr()
			if err := call.Run(); err != nil {
				return fmt.Errorf("read %s options: %w", p.Name, err)
			}
			return nil
		},
	}
}
