package cli

import (
	"github.com/RBen19/devswitch/internal/provider"
	"github.com/spf13/cobra"
	"strings"
)

func configureCompletion(root *cobra.Command) {
	for _, cmd := range root.Commands() {
		switch cmd.Name() {
		case "add", "adopt", "run", "login", "share":
			cmd.ValidArgsFunction = completeProfiles
		case "cx", "cl", "gm":
			id := provider.Codex
			switch cmd.Name() {
			case "cl":
				id = provider.Claude
			case "gm":
				id = provider.Gemini
			}
			cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
				if len(args) > 0 {
					return nil, cobra.ShellCompDirectiveNoFileComp
				}
				return profileNames(id, prefix), cobra.ShellCompDirectiveNoFileComp
			}
		case "alias":
			for _, sub := range cmd.Commands() {
				switch sub.Name() {
				case "remove":
					sub.ValidArgsFunction = func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
						var result []string
						if len(args) == 0 {
							settings, err := loadSettings()
							if err == nil {
								for _, a := range settings.Aliases {
									if strings.HasPrefix(a.Name, prefix) {
										result = append(result, a.Name)
									}
								}
							}
						}
						return result, cobra.ShellCompDirectiveNoFileComp
					}
				case "add":
					sub.ValidArgsFunction = func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
						if len(args) == 0 {
							return nil, cobra.ShellCompDirectiveNoFileComp
						}
						return completeProfiles(cmd, args[1:], prefix)
					}
				}
			}
		}
		if cmd.Flags().Lookup("shell") != nil {
			_ = cmd.RegisterFlagCompletionFunc("shell", cobra.FixedCompletions([]string{"bash", "zsh", "fish"}, cobra.ShellCompDirectiveNoFileComp))
		}
		if cmd.Flags().Lookup("only") != nil {
			_ = cmd.RegisterFlagCompletionFunc("only", cobra.FixedCompletions([]string{"sessions", "skills", "agents", "rules", "commands"}, cobra.ShellCompDirectiveNoFileComp))
		}
	}
}
func completeProfiles(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return []string{"claude\tClaude Code", "codex\tOpenAI Codex", "gemini\tGemini CLI"}, cobra.ShellCompDirectiveNoFileComp
	}
	if cmd.Name() == "adopt" || (cmd.Name() == "add" && cmd.Parent() == cmd.Root()) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if len(args) > 1 && cmd.Name() != "share" {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	p, err := provider.Parse(args[0])
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var result []string
	for _, name := range profileNames(p.ID, prefix) {
		used := false
		for _, arg := range args[1:] {
			if name == arg {
				used = true
			}
		}
		if !used {
			result = append(result, name)
		}
	}
	return result, cobra.ShellCompDirectiveNoFileComp
}
func profileNames(id provider.ID, prefix string) []string {
	store, err := getStore()
	if err != nil {
		return nil
	}
	var names []string
	for _, p := range store.Profiles {
		if p.Provider == id && strings.HasPrefix(p.Name, prefix) {
			names = append(names, p.Name)
		}
	}
	return names
}
