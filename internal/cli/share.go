package cli

import (
	"fmt"
	"io"
	"strings"

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
		Long:    "Share selected data using symlinks to a source profile. Close the affected agents first. Existing target data is merged into the source and the original kept as a backup. Credentials and provider settings remain separate.",
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

func shareAll(out io.Writer, store *profile.Store, p provider.Provider, source string, targets []string) error {
	links, err := store.PlanShare(p, source, targets, nil)
	if err != nil {
		return err
	}
	if err := profile.ApplyShare(links); err != nil {
		return fmt.Errorf("share profiles: %w", err)
	}
	fmt.Fprintf(out, "Shared %s/%s with %s; previous data merged, originals kept as *.devswitch-backup.\n", p.ID, source, strings.Join(targets, ", "))
	return nil
}

// refreshShares links paths added to the sharing list since setup, so updates
// through the installer extend existing shares.
func refreshShares(out io.Writer, settings shellSettings) error {
	links, err := pendingShares(settings)
	if err != nil || len(links) == 0 {
		return err
	}
	if err := profile.ApplyShare(links); err != nil {
		return fmt.Errorf("share profiles: %w", err)
	}
	for _, link := range links {
		fmt.Fprintf(out, "Shared %s -> %s; previous data merged, original kept as backup.\n", link.Target, link.Source)
	}
	return nil
}

// pendingShares lists links missing between each sharing source chosen in setup and its other profiles.
func pendingShares(settings shellSettings) ([]profile.ShareLink, error) {
	store, err := getStore()
	if err != nil {
		return nil, err
	}
	var pending []profile.ShareLink
	for id, source := range settings.Share {
		p, err := provider.Parse(id)
		if err != nil {
			return nil, err
		}
		var targets []string
		for _, item := range store.Profiles {
			if item.Provider == p.ID && item.Name != source {
				targets = append(targets, item.Name)
			}
		}
		if len(targets) == 0 {
			continue
		}
		links, err := store.PlanShare(p, source, targets, nil)
		if err != nil {
			return nil, err
		}
		pending = append(pending, links...)
	}
	return pending, nil
}

// shareNewProfile links a new profile to the source chosen during setup.
func shareNewProfile(out io.Writer, store *profile.Store, p provider.Provider, name string) error {
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	source := settings.Share[string(p.ID)]
	if source == "" || source == name {
		return nil
	}
	if err := shareAll(out, store, p, source, []string{name}); err != nil {
		return fmt.Errorf("profile created, but sharing with %s/%s failed: %w", p.ID, source, err)
	}
	return nil
}
