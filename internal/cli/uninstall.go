package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
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
	configFile, err := shellConfigFile()
	if err != nil {
		return err
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

	if err := removePathIntegration(configFile); err != nil {
		return err
	}
	fmt.Fprintf(out, "✓ PATH integration removed from %s\n", configFile)

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

func removePathIntegration(configFile string) error {
	data, err := os.ReadFile(configFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", configFile, err)
	}
	lines := strings.Split(string(data), "\n")
	filtered := make([]string, 0, len(lines))
	for index := 0; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) == pathMarker {
			if index+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[index+1]), "export PATH=") {
				index++
			}
			continue
		}
		filtered = append(filtered, lines[index])
	}
	updated := strings.Join(filtered, "\n")
	if err := os.WriteFile(configFile, []byte(updated), 0o600); err != nil {
		return fmt.Errorf("update %s: %w", configFile, err)
	}
	return nil
}
