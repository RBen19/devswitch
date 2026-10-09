package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/RBen19/devswitch/internal/provider"
	"github.com/spf13/cobra"
)

func discoverCommand() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{Use: "discover", Short: "Detect provider CLIs, existing configurations, and profiles", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jsonOutput {
				detections, err := detectProviders()
				if err != nil {
					return err
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(detections)
			}
			return discoverProviders(cmd.OutOrStdout())
		}}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable detection results")
	return cmd
}
func discoverProviders(out io.Writer) error {
	detections, err := detectProviders()
	if err != nil {
		return err
	}
	for _, d := range detections {
		if d.Binary == "" {
			fmt.Fprintf(out, "%s: CLI not found in PATH\n", d.Provider)
		} else {
			fmt.Fprintf(out, "%s: %s\n", d.Provider, d.Binary)
		}
		if d.Configuration {
			fmt.Fprintf(out, "  Existing configuration: %s\n", d.Home)
		}
		if len(d.Profiles) > 0 {
			fmt.Fprintf(out, "  Profiles: %v\n", d.Profiles)
		} else if d.Configuration {
			fmt.Fprintf(out, "  Next: devswitch adopt %s personal\n", d.Provider)
		} else {
			fmt.Fprintf(out, "  Next: devswitch add %s personal\n", d.Provider)
		}
	}
	return nil
}

func adoptCommand() *cobra.Command {
	var noShare bool
	command := &cobra.Command{
		Use:     "adopt <claude|codex|gemini> <name>",
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
			item, err := store.Adopt(p, args[1], home, noShare)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Adopted %s as %s/%s\n  directory: %s\n", p.ID, p.ID, item.Name, item.Home)
			if noShare {
				return nil
			}
			return shareNewProfile(cmd.OutOrStdout(), store, p, item.Name)
		},
	}
	command.Flags().BoolVar(&noShare, "no-share", false, "keep this profile isolated even if setup enabled sharing")
	return command
}

func shortcutCommand(use string, p provider.ID) *cobra.Command {
	return &cobra.Command{
		Use:                use + " <profile> [args...]",
		Aliases:            []string{string(p)},
		Short:              "Run " + string(p) + " with a short command",
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true,
		Example:            "  dvsw " + use + " p\n  dvsw " + use + " work",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if name == "p" {
				name = "personal"
			}
			if name == "w" {
				name = "work"
			}
			return launch(cmd.Context(), cmd.OutOrStdout(), string(p), name, false, providerArgs(args[1:]))
		},
	}
}
