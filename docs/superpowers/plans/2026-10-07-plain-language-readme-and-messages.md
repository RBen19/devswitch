# Plain-Language README and Messages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make devswitch understandable to non-technical users: a README that starts in plain language, and the errors people hit most often explain what to do next.

**Architecture:** Each provider definition gets a display name and official install link, so the "not installed" error can point to the installer. `Store.Find` lists the user's existing accounts when a name is wrong. The README is restructured into a beginner part (intro, quickstart, demo GIF, glossary, FAQ) followed by the existing reference under "For developers". The GIF is generated from a committed VHS tape so it can be re-recorded.

**Tech Stack:** Go 1.26 stdlib, cobra (existing), VHS (`github.com/charmbracelet/vhs`, dev-only, not a Go module dependency), Markdown.

**Spec:** `docs/superpowers/specs/2026-10-07-non-tech-reach-design.md`, section "A. README and messages".

## Global Constraints

- devswitch never reads, copies or stores credentials; README text must not suggest otherwise.
- No new Go module dependencies (`go.mod` unchanged).
- CLI behavior and user-facing messages stay in English.
- Supported platforms are still macOS and Linux only; README must not promise Windows yet.
- Every commit passes `go test ./...`, `go vet ./...`, `git diff --check`.
- Commit messages end with: `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`

## Review Focus

1. A typo in the provider name (`devswitch run claud work`) must say which values are accepted — already true; keep the e2e test green.
2. Running a provider that is not installed must name the product and give the install link, and exit nonzero.
3. A wrong profile name must list the existing accounts for that provider, or say there are none and show the `add` command.
4. A wrong profile name for a provider with zero profiles must not print an empty list (`Your Claude Code accounts: .`).
5. The README's sharing section must match the code: Codex `state_5.sqlite` is shared under `sessions`.

---

### Task 1: Install links on provider definitions

**Files:**
- Modify: `internal/provider/provider.go`
- Create: `internal/provider/provider_test.go`

**Interfaces:**
- Produces: `Provider.Name string` (display name, e.g. `"Claude Code"`), `Provider.InstallURL string`; `Provider.Available()` error text: `<Name> isn't installed yet (the "<binary>" command was not found). Install it from <InstallURL>, then try again.`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/provider/ -run TestMissingProviderExplainsHowToInstall -v`
Expected: FAIL to compile — `p.Name undefined`.

- [ ] **Step 3: Implement**

In `internal/provider/provider.go`, add the fields and fill them in `Parse`:

```go
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
```

- [ ] **Step 4: Run tests**

Run: `go test ./... && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/provider/provider.go internal/provider/provider_test.go
git commit -m "feat: tell users where to install a missing provider"
```

---

### Task 2: Helpful "profile not found"

**Files:**
- Modify: `internal/profile/store.go:170-177` (`Find`)
- Test: `internal/profile/store_test.go`

**Interfaces:**
- Consumes: `provider.Parse(id).Name` from Task 1.
- Produces: `Store.Find` error text, unchanged signature `Find(p provider.ID, name string) (Profile, error)`:
  - with accounts: `no Claude Code account named "wrk". Your Claude Code accounts: personal, work`
  - without: `no Claude Code account named "wrk" yet. Create it with: devswitch add claude wrk`

- [ ] **Step 1: Write the failing test** (append to `internal/profile/store_test.go`; add `"strings"` to its imports if missing)

```go
func TestFindExplainsMissingProfile(t *testing.T) {
	store, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claude, _ := provider.Parse("claude")

	_, err = store.Find(provider.Claude, "wrk")
	if err == nil || err.Error() != `no Claude Code account named "wrk" yet. Create it with: devswitch add claude wrk` {
		t.Fatalf("empty store: %v", err)
	}

	for _, name := range []string{"work", "personal"} {
		if _, err := store.Add(claude, name); err != nil {
			t.Fatal(err)
		}
	}
	_, err = store.Find(provider.Claude, "wrk")
	if err == nil || err.Error() != `no Claude Code account named "wrk". Your Claude Code accounts: personal, work` {
		t.Fatalf("populated store: %v", err)
	}
	if _, err := store.Find(provider.Codex, "work"); err == nil || !strings.Contains(err.Error(), "devswitch add codex work") {
		t.Fatalf("other provider's profiles must not be listed: %v", err)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/profile/ -run TestFindExplainsMissingProfile -v`
Expected: FAIL — `empty store: profile not found: claude/wrk`.

- [ ] **Step 3: Implement** (replace `Find`; `Store.Profiles` is already kept sorted by `create`)

```go
func (s *Store) Find(p provider.ID, name string) (Profile, error) {
	var names []string
	for _, item := range s.Profiles {
		if item.Provider != p {
			continue
		}
		if item.Name == name {
			return item, nil
		}
		names = append(names, item.Name)
	}
	label := string(p)
	if def, err := provider.Parse(string(p)); err == nil {
		label = def.Name
	}
	if len(names) == 0 {
		return Profile{}, fmt.Errorf("no %s account named %q yet. Create it with: devswitch add %s %s", label, name, p, name)
	}
	return Profile{}, fmt.Errorf("no %s account named %q. Your %s accounts: %s", label, name, label, strings.Join(names, ", "))
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./... && go vet ./...`
Expected: PASS. If any test asserted `profile not found`, update it to the new text.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/store.go internal/profile/store_test.go
git commit -m "feat: list existing accounts when a profile name is wrong"
```

---

### Task 3: End-to-end check of both messages

**Files:**
- Test: `internal/e2e/cli_e2e_test.go`

**Interfaces:**
- Consumes: message texts from Tasks 1 and 2; existing helpers `projectRoot`, `writeFakeProvider`.

- [ ] **Step 1: Write the test** (append; it builds the binary like `TestProfileLifecycleWithCompiledBinary`)

```go
func TestFriendlyErrorsWithCompiledBinary(t *testing.T) {
	temp := t.TempDir()
	home := filepath.Join(temp, "home")
	emptyBin := filepath.Join(temp, "empty-bin")
	fakeBin := filepath.Join(temp, "fake-bin")
	for _, dir := range []string{home, emptyBin, fakeBin} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writeFakeProvider(t, filepath.Join(fakeBin, "claude"))
	devswitch := filepath.Join(temp, "devswitch")
	build := exec.Command("go", "build", "-o", devswitch, "./cmd/devswitch")
	build.Dir = projectRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, output)
	}
	run := func(path string, args ...string) (string, error) {
		command := exec.Command(devswitch, args...)
		command.Env = append(os.Environ(), "HOME="+home, "PATH="+path, "DEVSWITCH_E2E_OUTPUT="+filepath.Join(temp, "out"))
		output, err := command.CombinedOutput()
		return string(output), err
	}

	output, err := run(emptyBin, "run", "claude", "work")
	if err == nil || !strings.Contains(output, "Claude Code isn't installed yet") || !strings.Contains(output, "https://code.claude.com/docs/en/setup") {
		t.Fatalf("missing provider: err=%v\n%s", err, output)
	}

	if _, err := run(fakeBin, "add", "claude", "work"); err != nil {
		t.Fatal(err)
	}
	output, err = run(fakeBin, "run", "claude", "wrk")
	if err == nil || !strings.Contains(output, "Your Claude Code accounts: work") {
		t.Fatalf("wrong profile name: err=%v\n%s", err, output)
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/e2e/ -run TestFriendlyErrorsWithCompiledBinary -v`
Expected: PASS (Tasks 1–2 already implemented the behavior). Temporarily revert the `Available()` message to confirm the test fails, then restore.

- [ ] **Step 3: Commit**

```bash
git add internal/e2e/cli_e2e_test.go
git commit -m "test: cover friendly provider and profile errors end to end"
```

---

### Task 4: Reproducible demo GIF

**Files:**
- Create: `docs/demo.tape`
- Create: `docs/demo.gif` (generated)
- Modify: `Makefile`

**Interfaces:**
- Produces: `docs/demo.gif`, referenced by the README in Task 5; `make demo` regenerates it.

- [ ] **Step 1: Install VHS (dev machine only)**

Run: `go install github.com/charmbracelet/vhs@latest` and install its runtime tools `ttyd` and `ffmpeg` (Ubuntu: `sudo apt install ffmpeg` and ttyd from its GitHub releases; macOS: `brew install ttyd ffmpeg`).
Expected: `vhs --version` prints a version.

- [ ] **Step 2: Write `docs/demo.tape`**

The tape uses a throwaway HOME and fake `claude`/`codex` commands so it never touches real accounts.

```
Output docs/demo.gif
Set FontSize 18
Set Width 1000
Set Height 560
Set TypingSpeed 60ms

Hide
Type "export HOME=$(mktemp -d) && mkdir -p $HOME/bin && printf '#!/bin/sh\necho Welcome to the assistant' > $HOME/bin/claude && cp $HOME/bin/claude $HOME/bin/codex && chmod +x $HOME/bin/* && export PATH=$PWD:$HOME/bin:$PATH && clear"
Enter
Show

Type "devswitch add claude work"
Enter
Sleep 1s
Type "devswitch add claude personal"
Enter
Sleep 1s
Type "devswitch list"
Enter
Sleep 2s
Type "devswitch run claude work"
Enter
Sleep 2s
```

- [ ] **Step 3: Add a Makefile target**

Add `demo` to the `.PHONY` line and append:

```make
demo: build
	vhs docs/demo.tape
```

- [ ] **Step 4: Generate and inspect**

Run: `make demo`
Expected: `docs/demo.gif` exists, under 2 MB, and shows the four commands with readable output. Open it and check it visually.

- [ ] **Step 5: Commit**

```bash
git add docs/demo.tape docs/demo.gif Makefile
git commit -m "docs: add a reproducible demo GIF"
```

---

### Task 5: Rewrite the README

**Files:**
- Modify: `README.md`

**Interfaces:**
- Consumes: `docs/demo.gif` (Task 4); message texts (Tasks 1–2) for the FAQ.

- [ ] **Step 1: Replace everything above `## Install` with the beginner section**

```markdown
# devswitch

> Use your work and personal AI accounts on the same computer, without logging out. · by RBen19

devswitch lets you keep several accounts for **Claude Code**, **Codex** and **Gemini CLI** side by side —
for example a personal account and a work account — and open the one you need with a single command.
Each account keeps its own login and its own conversations.

![devswitch demo](docs/demo.gif)

## Get started in 3 steps

1. **Install** — open the Terminal app, paste this line and press Enter:

   ```bash
   curl -fsSL https://github.com/RBen19/devswitch/releases/latest/download/install.sh | bash
   ```

   It asks a few yes/no questions. Pressing Enter accepts the safe default.
2. **Add an account** — give it any name you like:

   ```bash
   devswitch add claude work
   devswitch login claude work
   ```

   The official Claude, Codex or Gemini login opens. devswitch never sees your password.
3. **Use it** — open that account whenever you need it:

   ```bash
   devswitch run claude work
   ```

   Open a second Terminal window to use another account at the same time.

Already logged in before installing devswitch? Setup offers to keep that login as your `personal` account.

## Words you will see

| Word | Meaning |
| --- | --- |
| Terminal | The app where you type commands (Terminal on macOS; Terminal or Console on Linux). |
| Profile / account | One login for one AI, with a name you choose, like `work` or `personal`. |
| Provider | The AI tool: `claude` (Claude Code), `codex` (Codex) or `gemini` (Gemini CLI). |
| PATH | The list of places your computer looks for commands. Setup handles it for you. |

## Questions

**Will I lose my current login?** No. Setup can keep your existing login as an account; nothing is logged out.

**Does devswitch see or store my password?** No. Logging in happens in the official Claude, Codex or Gemini window. devswitch only tells each tool which folder to use.

**It says the AI "isn't installed yet".** devswitch opens these tools but does not install them. Follow the link in the message, then try again.

**It says there is "no account named …".** Check the spelling; the message lists the accounts you have. `devswitch list` shows them all.

**Can I undo everything?** Yes: `devswitch uninstall` removes the shell setup; add `--purge` to also delete the accounts devswitch created.

**Does it work on Windows?** Not yet. macOS and Linux are supported today.

---

# For developers

## Why devswitch exists
```

Keep the existing "Why devswitch exists" paragraphs after the new heading, then the rest of the file unchanged except for Step 2.

- [ ] **Step 2: Fix the outdated sharing text**

In the sharing table, change the Codex `sessions` cell to:
`` `sessions/`, `archived_sessions/`, `history.jsonl`, `session_index.jsonl`, `state_5.sqlite` ``

Replace the sentence starting `Credentials, provider settings` with:

```markdown
Credentials, provider settings (`config.toml`, `settings.json`), plugins and caches remain per-profile. SQLite databases also stay per-profile, except Codex's thread list `state_5.sqlite`, which is shared with `sessions` so shared conversations appear in Codex's picker; its threads are merged with the target's when sharing starts (requires the `sqlite3` command).
```

Delete the now-wrong sentence `Provider versions may use local database indexes for session lists; linking transcript files does not synchronize those indexes or guarantee every session appears in a provider's picker.` (Codex now shares its thread database; leave no claim about Claude's picker that hasn't been verified).

- [ ] **Step 3: Check**

Run: `git diff --check && grep -n "state_5.sqlite" README.md && grep -c "docs/demo.gif" README.md`
Expected: no whitespace errors; `state_5.sqlite` appears in the table and the paragraph; the GIF is referenced once. Preview the README (e.g. on GitHub or a Markdown viewer) and confirm the GIF renders and the table aligns.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: plain-language README for non-technical users"
```
