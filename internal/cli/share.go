package cli

import (
	"fmt"

	"github.com/RBen19/devswitch/internal/profile"
	"github.com/RBen19/devswitch/internal/provider"
	"github.com/spf13/cobra"
)

func shareCommand() *cobra.Command {
	var only []string
	var dryRun bool
	command := &cobra.Command{
		Use:     "share <claude|codex> <source> <target> [targets...]",
		Short:   "Symlink sessions, skills and agent files between same-provider profiles",
		Long:    "Share selected data using symlinks to a source profile. Close the affected agents first. Existing target paths are backed up, not merged. Credentials and provider settings remain separate.",
		Args:    cobra.MinimumNArgs(3),
		Example: "  devswitch share codex personal work\n  devswitch share claude personal work client --only skills,agents\n  devswitch share codex personal work --dry-run",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := provider.Parse(args[0])
			if err != nil {
				return err
			}
			store, err := getStore()
			if err != nil {
				return err
			}
			links, err := store.PlanShare(p, args[1], args[2:], only)
			if err != nil {
				return err
			}
			for _, link := range links {
				fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", link.Target, link.Source)
				if link.Backup != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  backup: %s\n", link.Backup)
				}
			}
			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "Dry run: no files changed.")
				return nil
			}
			if err := profile.ApplyShare(links); err != nil {
				return fmt.Errorf("share profiles: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Created %d shared links for %s (existing links left in place)\n", len(links), p.ID)
			return nil
		},
	}
	command.Flags().StringSliceVar(&only, "only", nil, "share only these categories: sessions,skills,agents,rules,commands")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "preview links and backups without changing files")
	return command
}
