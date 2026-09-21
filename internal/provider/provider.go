package provider

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type ID string

const (
	Claude ID = "claude"
	Codex  ID = "codex"
)

type Provider struct {
	ID         ID
	Binary     string
	HomeEnvVar string
	LoginArgs  []string
}

func Parse(value string) (Provider, error) {
	switch ID(value) {
	case Claude:
		return Provider{ID: Claude, Binary: "claude", HomeEnvVar: "CLAUDE_CONFIG_DIR"}, nil
	case Codex:
		return Provider{ID: Codex, Binary: "codex", HomeEnvVar: "CODEX_HOME", LoginArgs: []string{"login"}}, nil
	default:
		return Provider{}, fmt.Errorf("unknown provider %q (accepted values: claude, codex)", value)
	}
}

func (p Provider) Available() error {
	if _, err := exec.LookPath(p.Binary); err != nil {
		return fmt.Errorf("%s was not found in PATH; install %s first", p.Binary, p.Binary)
	}
	return nil
}

func (p Provider) DefaultHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, "."+string(p.ID)), nil
}
