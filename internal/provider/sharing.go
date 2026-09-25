package provider

import "fmt"

// SharedPath is an explicit allowlist entry; never share the entire provider home.
type SharedPath struct {
	Name      string
	Group     string
	Directory bool
	Create    bool
}

func (p Provider) SharedPaths(groups []string) ([]SharedPath, error) {
	paths := []SharedPath{
		{"skills", "skills", true, true},
		{"agents", "agents", true, true},
		{"rules", "rules", true, true},
		{"history.jsonl", "sessions", false, true},
	}
	switch p.ID {
	case Codex:
		paths = append(paths,
			SharedPath{"sessions", "sessions", true, true},
			SharedPath{"archived_sessions", "sessions", true, true},
			SharedPath{"session_index.jsonl", "sessions", false, true},
			SharedPath{"AGENTS.md", "agents", false, false},
			SharedPath{"AGENTS.override.md", "agents", false, false},
			SharedPath{"prompts", "commands", true, true})
	case Claude:
		paths = append(paths,
			SharedPath{"projects", "sessions", true, true},
			SharedPath{"file-history", "sessions", true, true},
			SharedPath{"tasks", "sessions", true, true},
			SharedPath{"plans", "sessions", true, true},
			SharedPath{"agent-memory", "agents", true, true},
			SharedPath{"CLAUDE.md", "agents", false, false},
			SharedPath{"commands", "commands", true, true})
	default:
		return nil, fmt.Errorf("unsupported provider %q", p.ID)
	}
	if len(groups) == 0 {
		return paths, nil
	}
	selected := make(map[string]bool)
	for _, group := range groups {
		switch group {
		case "sessions", "skills", "agents", "rules", "commands":
			selected[group] = true
		default:
			return nil, fmt.Errorf("unknown sharing category %q (use sessions, skills, agents, rules, commands)", group)
		}
	}
	var result []SharedPath
	for _, path := range paths {
		if selected[path.Group] {
			result = append(result, path)
		}
	}
	return result, nil
}
