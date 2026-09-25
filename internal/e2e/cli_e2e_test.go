package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProfileLifecycleWithCompiledBinary(t *testing.T) {
	root := projectRoot(t)
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	providerBin := filepath.Join(temp, "providers")
	outputFile := filepath.Join(temp, "provider-output")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providerBin, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, binary := range []string{"claude", "codex"} {
		writeFakeProvider(t, filepath.Join(providerBin, binary))
	}

	devswitch := filepath.Join(temp, "devswitch")
	build := exec.Command("go", "build", "-o", devswitch, "./cmd/devswitch")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, output)
	}

	env := append(os.Environ(),
		"HOME="+home,
		"PATH="+providerBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DEVSWITCH_E2E_OUTPUT="+outputFile,
	)

	assertCLI(t, devswitch, env, "add", "claude", "personal")
	assertCLI(t, devswitch, env, "add", "codex", "work")

	assertCLI(t, devswitch, env, "login", "claude", "personal")
	claudeOutput := readProviderOutput(t, outputFile)
	if !strings.Contains(claudeOutput, "CLAUDE_CONFIG_DIR="+filepath.Join(home, ".devswitch", "profiles", "claude", "personal")) {
		t.Fatalf("Claude profile environment was not isolated:\n%s", claudeOutput)
	}

	assertCLI(t, devswitch, env, "login", "codex", "work")
	codexOutput := readProviderOutput(t, outputFile)
	if !strings.Contains(codexOutput, "CODEX_HOME="+filepath.Join(home, ".devswitch", "profiles", "codex", "work")) {
		t.Fatalf("Codex profile environment was not isolated:\n%s", codexOutput)
	}
	if !strings.Contains(codexOutput, "ARGS=login") {
		t.Fatalf("Codex login command did not receive the login argument:\n%s", codexOutput)
	}

	for _, provider := range []string{"claude", "codex"} {
		assertCLI(t, devswitch, env, "add", provider, "shared-target")
		source := "personal"
		if provider == "codex" {
			source = "work"
		}
		targetHome := filepath.Join(home, ".devswitch", "profiles", provider, "shared-target")
		preview := assertCLI(t, devswitch, env, "share", provider, source, "shared-target", "--dry-run")
		if !strings.Contains(preview, "Dry run") {
			t.Fatal("missing dry-run output")
		}
		if _, err := os.Lstat(filepath.Join(targetHome, "skills")); !os.IsNotExist(err) {
			t.Fatal("dry run changed target")
		}
		assertCLI(t, devswitch, env, "share", provider, source, "shared-target")
		sourceHome := filepath.Join(home, ".devswitch", "profiles", provider, source)
		if err := os.WriteFile(filepath.Join(sourceHome, "skills", "example.md"), []byte("shared skill"), 0o600); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(targetHome, "skills", "example.md"))
		if err != nil || string(data) != "shared skill" {
			t.Fatalf("skill was not shared: %s, %v", data, err)
		}
		assertCLI(t, devswitch, env, "share", provider, source, "shared-target")
		assertCLI(t, devswitch, env, "run", provider, "shared-target")
		if !strings.Contains(readProviderOutput(t, outputFile), targetHome) {
			t.Fatal("sharing changed the login home")
		}
	}

	listOutput := assertCLI(t, devswitch, env, "list")
	for _, expected := range []string{"claude", "personal", "codex", "work"} {
		if !strings.Contains(listOutput, expected) {
			t.Fatalf("list output does not contain %q:\n%s", expected, listOutput)
		}
	}
}

func projectRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate e2e test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func writeFakeProvider(t *testing.T, path string) {
	t.Helper()
	script := `#!/bin/sh
printf 'CLAUDE_CONFIG_DIR=%s\n' "$CLAUDE_CONFIG_DIR" > "$DEVSWITCH_E2E_OUTPUT"
printf 'CODEX_HOME=%s\n' "$CODEX_HOME" >> "$DEVSWITCH_E2E_OUTPUT"
printf 'ARGS=%s\n' "$*" >> "$DEVSWITCH_E2E_OUTPUT"
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}

func assertCLI(t *testing.T, binary string, env []string, args ...string) string {
	t.Helper()
	command := exec.Command(binary, args...)
	command.Env = env
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("devswitch %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func readProviderOutput(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
