package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

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
		os.Exit(1)
	}
}

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           appName,
		Short:         "Switch cleanly between Claude Code and Codex profiles",
		Long:          "devswitch keeps your Claude Code and Codex sessions isolated, easy to launch, and safe to share.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
		RunE: func(cmd *cobra.Command, _ []string) error {
			printBanner(cmd.OutOrStdout())
			return cmd.Help()
		},
	}
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		printBanner(cmd.OutOrStdout())
		fmt.Fprintln(cmd.OutOrStdout(), "")
		fmt.Fprintln(cmd.OutOrStdout(), cmd.UsageString())
	})
	root.AddCommand(addCommand(), listCommand(), loginCommand(), runCommand(), installCommand())
	return root
}

func printBanner(w io.Writer) {
	fmt.Fprintln(w, `
██████╗ ███████╗██╗   ██╗███████╗██╗    ██╗██╗████████╗ ██████╗██╗  ██╗
██╔══██╗██╔════╝██║   ██║██╔════╝██║    ██║██║╚══██╔══╝██╔════╝██║  ██║
██║  ██║█████╗  ██║   ██║███████╗██║ █╗ ██║██║   ██║   ██║     ███████║
██║  ██║██╔══╝  ╚██╗ ██╔╝╚════██║██║███╗██║██║   ██║   ██║     ██╔══██║
██████╔╝███████╗ ╚████╔╝ ███████║╚███╔███╔╝██║   ██║   ╚██████╗██║  ██║
╚═════╝ ╚══════╝  ╚═══╝  ╚══════╝ ╚══╝╚══╝ ╚═╝   ╚═╝    ╚═════╝╚═╝  ╚═╝`)
	fmt.Fprintf(w, "\ndevswitch %s · by %s\n", version, byline)
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
		Use:     "add <claude|codex> <name>",
		Short:   "Create an isolated profile",
		Args:    cobra.ExactArgs(2),
		Example: "  devswitch add claude perso\n  devswitch add codex travail",
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
	return &cobra.Command{
		Use:   "list",
		Short: "List available profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := getStore()
			if err != nil {
				return err
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
}

func loginCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "login <claude|codex> <name>",
		Short:   "Open the official login flow for a profile",
		Args:    cobra.ExactArgs(2),
		Example: "  devswitch login claude perso\n  devswitch login codex travail",
		RunE: func(cmd *cobra.Command, args []string) error {
			return launch(cmd.Context(), cmd.OutOrStdout(), args[0], args[1], true, nil)
		},
	}
}

func runCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "run <claude|codex> <name> [-- args...]",
		Short:   "Run a tool with a profile",
		Args:    cobra.MinimumNArgs(2),
		Example: "  devswitch run claude perso\n  devswitch run codex travail -- --full-auto",
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
	command.Env = append(os.Environ(), p.HomeEnvVar+"="+item.Home)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("run %s/%s: %w", p.ID, name, err)
	}
	return nil
}
