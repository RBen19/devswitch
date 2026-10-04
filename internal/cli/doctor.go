package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/RBen19/devswitch/internal/profile"
	"github.com/RBen19/devswitch/internal/provider"
	"github.com/spf13/cobra"
)

type detection struct {
	Provider      string   `json:"provider"`
	Binary        string   `json:"binary,omitempty"`
	Home          string   `json:"home"`
	Configuration bool     `json:"configuration"`
	Profiles      []string `json:"profiles"`
}

func sameHome(a, b string) bool {
	aa, err := filepath.EvalSymlinks(a)
	if err == nil {
		a = aa
	}
	bb, err := filepath.EvalSymlinks(b)
	if err == nil {
		b = bb
	}
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	return a == b
}
func detectProviders() ([]detection, error) {
	store, err := getStore()
	if err != nil {
		return nil, err
	}
	var result []detection
	for _, id := range []string{"claude", "codex", "gemini"} {
		p, _ := provider.Parse(id)
		home, err := p.DefaultHome()
		if err != nil {
			return nil, err
		}
		binary, _ := exec.LookPath(p.Binary)
		info, statErr := os.Stat(home)
		if statErr != nil && !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("inspect %s: %w", home, statErr)
		}
		item := detection{Provider: id, Binary: binary, Home: home, Configuration: statErr == nil && info.IsDir(), Profiles: []string{}}
		for _, profile := range store.Profiles {
			if profile.Provider == p.ID {
				item.Profiles = append(item.Profiles, profile.Name)
			}
		}
		result = append(result, item)
	}
	return result, nil
}

type healthCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func doctorCommand() *cobra.Command {
	var jsonOutput, fix bool
	cmd := &cobra.Command{Use: "doctor", Short: "Check providers, profiles, and shell setup", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		var checks []healthCheck
		detections, err := detectProviders()
		if err != nil {
			return err
		}
		available := 0
		for _, d := range detections {
			if d.Binary != "" {
				available++
				checks = append(checks, healthCheck{d.Provider, true, d.Binary})
			}
		}
		if available == 0 {
			checks = append(checks, healthCheck{"providers", false, "No supported CLI found. Install Claude Code or Codex and ensure it is in PATH."})
		}
		store, err := getStore()
		if err != nil {
			return err
		}
		if len(store.Profiles) == 0 {
			checks = append(checks, healthCheck{"profiles", false, "No profiles configured. Run devswitch install or devswitch add <provider> <name>."})
		}
		for _, p := range store.Profiles {
			info, err := os.Stat(p.Home)
			ok := err == nil && info.IsDir()
			detail := p.Home
			if !ok {
				detail += " (missing or inaccessible)"
			}
			checks = append(checks, healthCheck{string(p.Provider) + "/" + p.Name, ok, detail})
			if !ok {
				continue
			}
			entries, err := os.ReadDir(p.Home)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if entry.Type()&os.ModeSymlink != 0 {
					path := filepath.Join(p.Home, entry.Name())
					if _, err := os.Stat(path); err != nil {
						checks = append(checks, healthCheck{"shared link", false, path + ": " + err.Error()})
					}
				}
			}
		}
		settings, err := loadSettings()
		if err != nil {
			return err
		}
		pending, err := pendingShares(settings)
		if err != nil {
			return err
		}
		if len(pending) > 0 && fix {
			if err := refreshShares(cmd.OutOrStdout(), settings); err != nil {
				return err
			}
			pending = nil
		}
		for _, link := range pending {
			checks = append(checks, healthCheck{"unshared", false, link.Target + " (close agents, then run devswitch doctor --fix)"})
		}
		if len(settings.Shells) == 0 {
			checks = append(checks, healthCheck{"shell", false, "Run devswitch install to enable PATH, aliases, and completion."})
		}
		for _, sh := range settings.Shells {
			data, err := os.ReadFile(sh.Config)
			ok := err == nil && strings.Contains(string(data), shellStart) && strings.Contains(string(data), shellEnd)
			checks = append(checks, healthCheck{sh.Name + " integration", ok, sh.Config})
			stateRoot, err := profile.DefaultRoot()
			if err != nil {
				return err
			}
			completion := filepath.Join(stateRoot, "shell", "completion."+sh.Name)
			info, err := os.Stat(completion)
			checks = append(checks, healthCheck{sh.Name + " completion", err == nil && info.Mode().IsRegular() && info.Size() > 0, completion})
		}
		allOK := true
		for _, c := range checks {
			if !c.OK {
				allOK = false
			}
		}
		if jsonOutput {
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(checks); err != nil {
				return err
			}
		} else {
			for _, c := range checks {
				status := "OK"
				if !c.OK {
					status = "FIX"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-3s %-20s %s\n", status, c.Name, c.Detail)
			}
		}
		if !allOK {
			return fmt.Errorf("health checks found issues; see the suggested fixes above")
		}
		return nil
	}}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "print machine-readable checks")
	cmd.Flags().BoolVar(&fix, "fix", false, "share paths missing from profiles shared during setup")
	return cmd
}
