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
	Gemini ID = "gemini"
)

type Provider struct {
	ID         ID
	Name       string
	Binary     string
	HomeEnvVar string
	LoginArgs  []string
	InstallURL string
}

func Parse(value string) (Provider, error) {
	switch ID(value) {
	case Claude:
		return Provider{ID: Claude, Name: "Claude Code", Binary: "claude", HomeEnvVar: "CLAUDE_CONFIG_DIR", InstallURL: "https://code.claude.com/docs/en/setup"}, nil
	case Codex:
		return Provider{ID: Codex, Name: "Codex", Binary: "codex", HomeEnvVar: "CODEX_HOME", LoginArgs: []string{"login"}, InstallURL: "https://github.com/openai/codex"}, nil
	case Gemini:
		return Provider{ID: Gemini, Name: "Gemini CLI", Binary: "agy", HomeEnvVar: "HOME", InstallURL: "https://antigravity.google"}, nil
	default:
		return Provider{}, fmt.Errorf("unknown provider %q (accepted values: claude, codex, gemini)", value)
	}
}

func (p Provider) Available() error {
	if _, err := exec.LookPath(p.Binary); err != nil {
		return fmt.Errorf("%s isn't installed yet (the %q command was not found). Install it from %s, then try again", p.Name, p.Binary, p.InstallURL)
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
