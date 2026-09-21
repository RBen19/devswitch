package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/RBen19/devswitch/internal/provider"
	"github.com/spf13/cobra"
)

func discoverCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "discover",
		Short: "Detect installed providers and existing configurations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return discoverProviders(cmd.OutOrStdout())
		},
	}
}

func discoverProviders(out io.Writer) error {
	for _, id := range []string{"claude", "codex"} {
		p, _ := provider.Parse(id)
		home, err := p.DefaultHome()
		if err != nil {
			return err
		}
		installed := p.Available() == nil
		_, configErr := os.Stat(home)
		if installed && configErr == nil {
			fmt.Fprintf(out, "✓ Found %s and its configuration at %s\n  adopt it with: devswitch adopt %s <personal|work>\n", p.ID, home, p.ID)
		} else if installed {
			fmt.Fprintf(out, "✓ Found %s (no default configuration at %s)\n", p.ID, home)
		}
	}
	return nil
}

func adoptCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "adopt <claude|codex> <name>",
		Short:   "Use an existing provider configuration as a profile",
		Args:    cobra.ExactArgs(2),
		Example: "  devswitch adopt claude personal\n  devswitch adopt codex work",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := provider.Parse(args[0])
			if err != nil {
				return err
			}
			home, err := p.DefaultHome()
			if err != nil {
				return err
			}
			store, err := getStore()
			if err != nil {
				return err
			}
			item, err := store.Adopt(p, args[1], home)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Adopted %s as %s/%s\n  directory: %s\n", p.ID, p.ID, item.Name, item.Home)
			return nil
		},
	}
}

func shortcutCommand(use string, p provider.ID) *cobra.Command {
	return &cobra.Command{
		Use:     use + " <profile>",
		Aliases: []string{string(p)},
		Short:   "Run " + string(p) + " with a short command",
		Args:    cobra.ExactArgs(1),
		Example: "  dvsw " + use + " p\n  dvsw " + use + " work",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if name == "p" {
				name = "personal"
			}
			if name == "w" {
				name = "work"
			}
			return launch(cmd.Context(), cmd.OutOrStdout(), string(p), name, false, nil)
		},
	}
}
