package e2e_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const ptyRunner = `import os, pty, sys
pid, fd = pty.fork()
if pid == 0:
    os.execvp(sys.argv[1], sys.argv[1:])
while True:
    try:
        data = os.read(fd, 4096)
    except OSError:
        break
    if not data:
        break
    os.write(1, data)
sys.exit(os.waitstatus_to_exitcode(os.waitpid(pid, 0)[1]))
`

func TestOnboardingShellsAliasesAndCompletion(t *testing.T) {
	temp := filepath.Join(t.TempDir(), "space ' $(touch SHOULD_NOT_EXIST)")
	if err := os.MkdirAll(temp, 0o700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(temp, "devswitch")
	build := exec.Command("go", "build", "-o", binary, "./cmd/devswitch")
	build.Dir = projectRoot(t)
	if data, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, data)
	}
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			shellBin, err := exec.LookPath(shell)
			if err != nil {
				t.Skipf("%s unavailable", shell)
			}
			home := filepath.Join(temp, shell)
			fakeBin := filepath.Join(home, "bin")
			if err := os.MkdirAll(fakeBin, 0o700); err != nil {
				t.Fatal(err)
			}
			for _, p := range []string{"claude", "codex"} {
				if err := os.MkdirAll(filepath.Join(home, "."+p), 0o700); err != nil {
					t.Fatal(err)
				}
				writeFakeProvider(t, filepath.Join(fakeBin, p))
			}
			output := filepath.Join(home, "provider-output")
			env := append(os.Environ(), "HOME="+home, "SHELL="+shellBin, "ZDOTDIR="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"), "PATH="+fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"), "DEVSWITCH_E2E_OUTPUT="+output, "CODEX_HOME=/wrong-profile", "CLAUDE_CONFIG_DIR=/wrong-profile")
			for i := 0; i < 2; i++ {
				assertCLI(t, binary, env, "install", "--yes", "--shell", shell)
			}
			profiles := assertCLI(t, binary, env, "list")
			if strings.Count(profiles, "personal") != 2 {
				t.Fatalf("adoption failed: %s", profiles)
			}
			assertCLI(t, binary, env, "alias", "add", "ds")
			assertCLI(t, binary, env, "alias", "add", "my-work", "codex", "personal")
			completion := assertCLI(t, binary, env, "__complete", "run", "codex", "")
			if !strings.Contains(completion, "personal") {
				t.Fatalf("missing profile completion: %s", completion)
			}
			assertCLI(t, binary, env, "doctor")
			config := filepath.Join(home, "."+shell+"rc")
			if shell == "fish" {
				config = filepath.Join(home, ".config", "fish", "conf.d", "devswitch.fish")
			}
			command := "source \"$CONFIG_FILE\"; ds list; my-work --version"
			if shell == "bash" {
				command = "shopt -s expand_aliases; source \"$CONFIG_FILE\"\nds list\nmy-work --version\ncomplete -p ds"
			}
			if shell == "zsh" {
				command = "set -e; source \"$CONFIG_FILE\"; eval 'ds list; my-work --version'; (( $+functions[compdef] ))"
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			run := exec.CommandContext(ctx, shellBin, "-c", command)
			if shell == "zsh" {
				// zsh drops interactive mode (and completion) without a terminal, so give it a pty.
				run = exec.CommandContext(ctx, "python3", "-c", ptyRunner, shellBin, "-i", "-c", command)
			}
			run.Env = append(env, "CONFIG_FILE="+config)
			if data, err := run.CombinedOutput(); err != nil {
				t.Fatalf("shell startup failed: %v\n%s", err, data)
			}
			providerOutput := readProviderOutput(t, output)
			if strings.Contains(providerOutput, "CODEX_HOME=/wrong-profile") || !strings.Contains(providerOutput, "ARGS=--version") {
				t.Fatalf("wrong alias launch: %s", providerOutput)
			}
			assertCLI(t, binary, env, "alias", "remove", "my-work")
			data, err := os.ReadFile(config)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "my-work") {
				t.Fatal("alias was not removed")
			}
			assertCLI(t, binary, env, "uninstall", "--yes")
			data, err = os.ReadFile(config)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "# >>> devswitch >>>") {
				t.Fatal("integration was not removed")
			}
		})
	}
}

func TestProviderExitCodeIsPreserved(t *testing.T) {
	temp := t.TempDir()
	binary := filepath.Join(temp, "devswitch")
	build := exec.Command("go", "build", "-o", binary, "./cmd/devswitch")
	build.Dir = projectRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	providerBin := filepath.Join(temp, "bin")
	if err := os.MkdirAll(providerBin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providerBin, "codex"), []byte("#!/bin/sh\nexit 42\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "HOME="+temp, "PATH="+providerBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	assertCLI(t, binary, env, "add", "codex", "work")
	command := exec.Command(binary, "run", "codex", "work")
	command.Env = env
	err := command.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 42 {
		t.Fatalf("got %v; want provider exit status 42", err)
	}
}
