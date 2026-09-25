package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/RBen19/devswitch/internal/profile"
	"github.com/spf13/cobra"
)

func uninstallCommand() *cobra.Command {
	var purge bool
	var yes bool
	command := &cobra.Command{
		Use:     "uninstall",
		Short:   "Remove devswitch shell integration",
		Long:    "Remove the PATH entry managed by devswitch. Profiles are preserved unless --purge is provided.",
		Args:    cobra.NoArgs,
		Example: "  devswitch uninstall\n  devswitch uninstall --purge",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return uninstall(cmd.InOrStdin(), cmd.OutOrStdout(), purge, yes)
		},
	}
	command.Flags().BoolVar(&purge, "purge", false, "also delete all local profiles and settings")
	command.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return command
}

func uninstall(in io.Reader, out io.Writer, purge, yes bool) error {
	settings, err := loadSettings()
	if err != nil {
		return err
	}
	shells := settings.Shells
	if len(shells) == 0 {
		sh, err := resolveShell("")
		if err == nil {
			shells = append(shells, sh)
		}
	}
	root, err := profile.DefaultRoot()
	if err != nil {
		return err
	}

	if !yes {
		message := "Remove devswitch PATH integration?"
		if purge {
			message += " This will also permanently delete " + root + "."
		}
		fmt.Fprintf(out, "%s [y/N] ", message)
		answer, readErr := bufio.NewReader(in).ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("read confirmation: %w", readErr)
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Uninstall cancelled.")
			return nil
		}
	}

	for _, sh := range shells {
		if err := removePathIntegration(sh.Config); err != nil {
			return err
		}
		completion := filepath.Join(root, "shell", "completion."+sh.Name)
		if err := os.Remove(completion); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Fprintf(out, "Removed PATH, completion, and aliases from %s\n", sh.Config)
	}
	settings.Shells = nil
	if !purge {
		if err := saveSettings(settings); err != nil {
			return err
		}
	}

	if purge {
		if err := os.RemoveAll(root); err != nil {
			return fmt.Errorf("remove %s: %w", root, err)
		}
		fmt.Fprintf(out, "✓ Local profiles and settings removed from %s\n", root)
	} else {
		fmt.Fprintf(out, "Profiles were preserved in %s\n", root)
	}

	executable, err := os.Executable()
	if err == nil {
		fmt.Fprintf(out, "Remove the binary manually if needed: %s\n", executable)
	}
	return nil
}
