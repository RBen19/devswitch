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

func TestSetupSharingMergesExistingAndNewProfiles(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	binary := filepath.Join(temp, "devswitch")
	build := exec.Command("go", "build", "-o", binary, "./cmd/devswitch")
	build.Dir = projectRoot(t)
	if data, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, data)
	}
	env := append(os.Environ(), "HOME="+home, "SHELL=/bin/bash")
	personal := filepath.Join(home, ".claude")
	work := filepath.Join(home, ".devswitch", "profiles", "claude", "work")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(personal, "projects", "repo", "a.jsonl"), "a\n")
	write(filepath.Join(personal, "projects", "repo", "memory", "MEMORY.md"), "personal memory")
	write(filepath.Join(personal, "history.jsonl"), "p\n")
	assertCLI(t, binary, env, "add", "claude", "work")
	write(filepath.Join(work, "projects", "repo", "b.jsonl"), "b\n")
	write(filepath.Join(work, "projects", "repo", "memory", "MEMORY.md"), "work memory")
	write(filepath.Join(work, "history.jsonl"), "w\n")

	setup := exec.Command(binary, "install", "--shell", "bash")
	setup.Env = env
	setup.Stdin = strings.NewReader("y\ny\ny\nn\nn\nn\n")
	if data, err := setup.CombinedOutput(); err != nil {
		t.Fatalf("setup: %v %s", err, data)
	}
	expect := map[string]string{
		filepath.Join(work, "projects", "repo", "a.jsonl"):                              "a\n",
		filepath.Join(work, "projects", "repo", "b.jsonl"):                              "b\n",
		filepath.Join(work, "projects", "repo", "memory", "MEMORY.md"):                  "personal memory",
		filepath.Join(work, "projects.devswitch-backup", "repo", "memory", "MEMORY.md"): "work memory",
		filepath.Join(work, "history.jsonl"):                                            "p\nw\n",
		filepath.Join(personal, "projects", "repo", "b.jsonl"):                          "b\n",
	}
	for path, want := range expect {
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Fatalf("%s: got %q, %v; want %q", path, data, err, want)
		}
	}

	assertCLI(t, binary, env, "add", "claude", "later")
	assertCLI(t, binary, env, "add", "claude", "solo", "--no-share")
	if info, err := os.Lstat(filepath.Join(home, ".devswitch", "profiles", "claude", "later", "projects")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("new profile was not shared: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".devswitch", "profiles", "claude", "solo", "projects")); !os.IsNotExist(err) {
		t.Fatal("--no-share profile was linked")
	}
	if again := assertCLI(t, binary, env, "install", "--shell", "bash"); !strings.Contains(again, "already set up") {
		t.Fatalf("setup ran twice: %s", again)
	}
}

func TestCodexThreadsSharedThroughSetupAndDoctor(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not installed")
	}
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	binary := filepath.Join(temp, "devswitch")
	build := exec.Command("go", "build", "-o", binary, "./cmd/devswitch")
	build.Dir = projectRoot(t)
	if data, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, data)
	}
	env := append(os.Environ(), "HOME="+home, "SHELL=/bin/bash")
	personal := filepath.Join(home, ".codex")
	work := filepath.Join(home, ".devswitch", "profiles", "codex", "work")
	sql := func(db, script string) string {
		t.Helper()
		data, err := exec.Command("sqlite3", db, script).CombinedOutput()
		if err != nil {
			t.Fatalf("sqlite3 %s: %v %s", db, err, data)
		}
		return strings.TrimSpace(string(data))
	}
	threads := func(home string) string {
		return sql(filepath.Join(home, "state_5.sqlite"), "SELECT group_concat(id) FROM (SELECT id FROM threads ORDER BY id)")
	}
	createDB := func(home, ids string) {
		sql(filepath.Join(home, "state_5.sqlite"), "PRAGMA journal_mode=WAL; CREATE TABLE threads (id TEXT PRIMARY KEY); INSERT INTO threads VALUES "+ids+";")
	}
	if err := os.MkdirAll(personal, 0o700); err != nil {
		t.Fatal(err)
	}
	createDB(personal, "('a'),('b')")
	assertCLI(t, binary, env, "add", "codex", "work")
	createDB(work, "('b'),('c')")

	setup := exec.Command(binary, "install", "--shell", "bash")
	setup.Env = env
	// PATH, adopt ~/.codex, share codex, then decline the dvsw and profile aliases.
	setup.Stdin = strings.NewReader("y\ny\ny\nn\nn\nn\n")
	if data, err := setup.CombinedOutput(); err != nil {
		t.Fatalf("setup: %v %s", err, data)
	}
	if got := threads(work); got != "a,b,c" {
		t.Fatalf("work threads after setup: %q", got)
	}
	sql(filepath.Join(work, "state_5.sqlite"), "INSERT INTO threads VALUES ('from-work')")
	if got := threads(personal); got != "a,b,c,from-work" {
		t.Fatalf("thread created in work is not visible in personal: %q", got)
	}
	if doctor := assertCLI(t, binary, env, "doctor"); !strings.Contains(doctor, "OK  codex sharing        personal -> work") {
		t.Fatalf("doctor does not report sharing: %s", doctor)
	}

	// A share made before the database was shareable leaves work with its own copy.
	if err := os.Remove(filepath.Join(work, "state_5.sqlite")); err != nil {
		t.Fatal(err)
	}
	createDB(work, "('d')")
	doctor := exec.Command(binary, "doctor")
	doctor.Env = env
	data, err := doctor.CombinedOutput()
	if err == nil || !strings.Contains(string(data), "unshared") || !strings.Contains(string(data), "devswitch doctor --fix") {
		t.Fatalf("doctor missed the unshared database: %v %s", err, data)
	}
	assertCLI(t, binary, env, "doctor", "--fix")
	if got := threads(personal); got != "a,b,c,d,from-work" {
		t.Fatalf("doctor --fix did not merge threads: %q", got)
	}
	assertCLI(t, binary, env, "doctor")
}

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
			// Ubuntu's /etc/zsh/zshrc runs its own compinit, which prompts on CI runners.
			run.Env = append(env, "CONFIG_FILE="+config, "skip_global_compinit=1")
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
