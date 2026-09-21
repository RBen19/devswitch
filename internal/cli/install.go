package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const pathMarker = "# devswitch: managed PATH"

func installCommand() *cobra.Command {
	var yes bool
	command := &cobra.Command{
		Use:     "install",
		Short:   "Add the devswitch directory to PATH",
		Long:    "Prepare your shell to use devswitch from any directory.",
		Args:    cobra.NoArgs,
		Example: "  devswitch install\n  devswitch install --yes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return installPath(cmd.InOrStdin(), cmd.OutOrStdout(), yes)
		},
	}
	command.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return command
}

func installPath(in io.Reader, out io.Writer, yes bool) error {
	pathDir, err := executableDir()
	if err != nil {
		return err
	}
	configFile, err := shellConfigFile()
	if err != nil {
		return err
	}
	line := fmt.Sprintf("%s\nexport PATH=\"%s:$PATH\"\nalias dvsw='devswitch'\n", pathMarker, pathDir)

	content, err := os.ReadFile(configFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("lire %s: %w", configFile, err)
	}
	if strings.Contains(string(content), pathMarker) || strings.Contains(string(content), "export PATH=\""+pathDir+":$PATH\"") {
		if !strings.Contains(string(content), "alias dvsw='devswitch'") {
			file, openErr := os.OpenFile(configFile, os.O_APPEND|os.O_WRONLY, 0o600)
			if openErr != nil {
				return fmt.Errorf("open %s: %w", configFile, openErr)
			}
			if _, writeErr := file.WriteString("alias dvsw='devswitch'\n"); writeErr != nil {
				file.Close()
				return fmt.Errorf("update %s: %w", configFile, writeErr)
			}
			file.Close()
			fmt.Fprintf(out, "✓ Added dvsw shortcut to %s\n", configFile)
		}
		fmt.Fprintf(out, "✓ PATH is already configured in %s\n", configFile)
		return nil
	}

	if !yes {
		fmt.Fprintf(out, "Add %s to PATH through %s? [y/N] ", pathDir, configFile)
		answer, readErr := bufio.NewReader(in).ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("lire la confirmation: %w", readErr)
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(out, "Installation cancelled.")
			return nil
		}
	}

	if err := os.MkdirAll(filepath.Dir(configFile), 0o700); err != nil {
		return fmt.Errorf("create shell directory: %w", err)
	}
	file, err := os.OpenFile(configFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("ouvrir %s: %w", configFile, err)
	}
	defer file.Close()
	if _, err := file.WriteString("\n" + line); err != nil {
		return fmt.Errorf("update %s: %w", configFile, err)
	}
	fmt.Fprintf(out, "✓ PATH configured in %s\n", configFile)
	fmt.Fprintln(out, "Open a new terminal, or run: source", configFile)
	fmt.Fprintln(out, "Scanning for existing Claude Code, Codex, and Gemini CLI installations...")
	return discoverProviders(out)
}

func executableDir() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve devswitch executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err == nil {
		executable = resolved
	}
	return filepath.Dir(executable), nil
}

func shellConfigFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	shell := filepath.Base(os.Getenv("SHELL"))
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zshrc"), nil
	case "bash":
		return filepath.Join(home, ".bashrc"), nil
	default:
		return "", fmt.Errorf("shell %q is not supported automatically; add PATH manually", shell)
	}
}
