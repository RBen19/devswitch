package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/RBen19/devswitch/internal/fileutil"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/RBen19/devswitch/internal/profile"
	"github.com/RBen19/devswitch/internal/provider"
	"github.com/spf13/cobra"
)

const (
	appName = "devswitch"
	byline  = "RBen19"
)

var version = "dev"

func Execute() {
	if err := NewRootCommand().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           appName,
		Short:         "Switch cleanly between Claude Code, Codex, and Gemini CLI profiles",
		Long:          "devswitch keeps Claude Code, Codex, and Gemini CLI logins isolated, with optional sharing of sessions, skills and agent files.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
		Example:       "  devswitch install                  # guided setup\n  devswitch run codex work           # launch a profile\n  devswitch alias add work-ai codex work\n  devswitch doctor",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.AddCommand(aliasCommand(), doctorCommand(), shareCommand(), addCommand(), listCommand(), loginCommand(), runCommand(), installCommand(), uninstallCommand(), discoverCommand(), adoptCommand(), shortcutCommand("cl", provider.Claude), shortcutCommand("cx", provider.Codex), shortcutCommand("gm", provider.Gemini))
	for _, cmd := range root.Commands() {
		if cmd.Name() == "install" || cmd.Name() == "uninstall" {
			serializeShellCommand(cmd)
		}
		if cmd.Name() == "alias" {
			for _, sub := range cmd.Commands() {
				if sub.Name() != "list" {
					serializeShellCommand(sub)
				}
			}
		}
	}
	configureCompletion(root)
	return root
}

func getStore() (*profile.Store, error) {
	root, err := profile.DefaultRoot()
	if err != nil {
		return nil, err
	}
	return profile.Load(root)
}

func addCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "add <claude|codex|gemini> <name>",
		Short:   "Create an isolated profile",
		Args:    cobra.ExactArgs(2),
		Example: "  devswitch add claude personal\n  devswitch add codex work",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := provider.Parse(args[0])
			if err != nil {
				return err
			}
			store, err := getStore()
			if err != nil {
				return err
			}
			created, err := store.Add(p, args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Profile %s/%s created\n  directory: %s\n", p.ID, created.Name, created.Home)
			fmt.Fprintf(cmd.OutOrStdout(), "  next step: devswitch login %s %s\n", p.ID, created.Name)
			return nil
		},
	}
}

func listCommand() *cobra.Command {
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List available profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := getStore()
			if err != nil {
				return err
			}
			if jsonOutput {
				if store.Profiles == nil {
					store.Profiles = []profile.Profile{}
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(store.Profiles)
			}
			if len(store.Profiles) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No profiles found. Start with: devswitch add claude personal")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "PROVIDER  PROFILE   DIRECTORY")
			for _, item := range store.Profiles {
				fmt.Fprintf(cmd.OutOrStdout(), "%-9s %-9s %s\n", item.Provider, item.Name, item.Home)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable profiles")
	return command
}

func loginCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "login <claude|codex|gemini> <name>",
		Short:   "Open the official login flow for a profile",
		Args:    cobra.ExactArgs(2),
		Example: "  devswitch login claude personal\n  devswitch login codex work",
		RunE: func(cmd *cobra.Command, args []string) error {
			return launch(cmd.Context(), cmd.OutOrStdout(), args[0], args[1], true, nil)
		},
	}
}

func runCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "run <claude|codex|gemini> <name> [-- args...]",
		Short:   "Run a tool with a profile",
		Args:    cobra.MinimumNArgs(2),
		Example: "  devswitch run claude personal\n  devswitch run codex work -- --full-auto",
		RunE: func(cmd *cobra.Command, args []string) error {
			return launch(cmd.Context(), cmd.OutOrStdout(), args[0], args[1], false, args[2:])
		},
	}
}

func launch(ctx context.Context, out io.Writer, providerName, name string, login bool, args []string) error {
	p, err := provider.Parse(providerName)
	if err != nil {
		return err
	}
	if err := p.Available(); err != nil {
		return err
	}
	store, err := getStore()
	if err != nil {
		return err
	}
	item, err := store.Find(p.ID, name)
	if err != nil {
		return err
	}

	commandArgs := args
	if login {
		commandArgs = p.LoginArgs
	}
	if p.ID == provider.Claude && login {
		fmt.Fprintln(out, "Claude Code is starting. Type /login in the session to authenticate.")
	}
	command := exec.CommandContext(ctx, p.Binary, commandArgs...)
	command.Env = withoutEnv(os.Environ(), p.HomeEnvVar)
	defaultHome, defaultHomeErr := p.DefaultHome()
	if defaultHomeErr != nil || !sameHome(item.Home, defaultHome) {
		command.Env = append(command.Env, p.HomeEnvVar+"="+item.Home)
	}
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("run %s/%s: %w", p.ID, name, err)
	}
	return nil
}

// Avoid inheriting a different profile's home from a parent agent session.
func withoutEnv(env []string, key string) []string {
	result := make([]string, 0, len(env))
	for _, value := range env {
		if !strings.HasPrefix(value, key+"=") {
			result = append(result, value)
		}
	}
	return result
}

func serializeShellCommand(cmd *cobra.Command) {
	run := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		root, err := profile.DefaultRoot()
		if err != nil {
			return err
		}
		return fileutil.WithLock(filepath.Join(root, "shell.lock"), func() error { return run(cmd, args) })
	}
}
