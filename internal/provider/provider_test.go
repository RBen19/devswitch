package provider

import (
	"strings"
	"testing"
)

func TestMissingProviderExplainsHowToInstall(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, id := range []string{"claude", "codex", "gemini"} {
		p, err := Parse(id)
		if err != nil {
			t.Fatal(err)
		}
		err = p.Available()
		if err == nil {
			t.Fatalf("%s: expected an error with an empty PATH", id)
		}
		msg := err.Error()
		if !strings.Contains(msg, p.Name+" isn't installed yet") || !strings.Contains(msg, p.InstallURL) || p.InstallURL == "" {
			t.Fatalf("%s: unhelpful message: %s", id, msg)
		}
	}
}
