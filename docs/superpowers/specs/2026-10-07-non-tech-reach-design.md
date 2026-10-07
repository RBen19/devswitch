# devswitch for non-technical users — design

Date: 2026-10-07 · Status: approved

## Goal

Make devswitch usable by people who use Claude Code, Codex or Gemini CLI but
are not developers (freelancers, agencies, students, people who build apps by
prompting an AI). Success means a non-technical user can:

1. install devswitch and see which AIs and accounts they already have, without typing commands;
2. add a Claude, Codex or Gemini account by clicking a button;
3. open any account in one click;
4. stop in one AI and continue the same work in another ("Continue in Codex").

Non-goals: devswitch does not become a chat client, does not call provider
APIs, never reads or stores credentials, and does not install the AIs for the user
(it links to the official installers).

## Delivery pieces (each gets its own implementation plan)

| # | Piece | Stack | Depends on |
|---|---|---|---|
| A | Plain-language README + error messages | Markdown; demo GIF recorded with [VHS](https://github.com/charmbracelet/vhs) from a committed `.tape` file; Go for messages | — |
| B | Automatic detection | Go stdlib (`os/exec`, `os`, process list) | — |
| C | Hand-off ("Continue in…") | Go stdlib (`bufio`, `encoding/json`), new `internal/handoff` package | B |
| D | Terminal menu (`devswitch` with no arguments) | Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea), Bubbles, Lip Gloss | B, C |
| E | Desktop app | [Wails v2](https://wails.io) (Go backend reusing `internal/*`) + TypeScript/Svelte frontend built with Vite; embedded terminal with xterm.js + `creack/pty` | B, C |
| F | More AIs through a settings file | Go stdlib; `~/.devswitch/providers.json` | — |
| G | Windows | Go cross-compile; PowerShell installer (`install.ps1`), later winget; directory junctions instead of symlinks | E for the GUI installer |

Order: A → B → C → D → E → F → G. A, B and F are independent and can run in parallel.

### A. README and messages

- README opens with one plain sentence, a 3-step quickstart (install, open devswitch, click/choose an account),
  a demo GIF, a small glossary (profile, terminal, PATH), and an FAQ
  ("Will I lose my login?", "Is my password stored?", "Can I undo it?").
  The existing command reference moves below a "For developers" heading.
- Fix outdated text: Codex `state_5.sqlite` is now shared under `sessions`.
- Error messages say what to do next, e.g. `Claude Code isn't installed yet. Install it: <official link>`.
  Install links live next to each provider definition in `internal/provider`.
- Testing: existing e2e tests that assert message text are updated; `git diff --check`.

### B. Automatic detection

The existing `detectProviders` (PATH lookup + default home) is extended. Three levels:

1. **Installed** — `exec.LookPath`, plus well-known install folders not always on PATH
   (`~/.local/bin`, npm global bin, Homebrew, `~/.claude/local`).
2. **Logged in** — existence check only, never read: `~/.claude/.credentials.json` (Linux) or
   the macOS Keychain entry existence, `~/.codex/auth.json`, Gemini's equivalent. Result: `logged in / not logged in / unknown`.
3. **Running now** — process list (`ps` on Unix) to show which accounts are open, so the UI can say
   "Claude – work is open" and warn before sharing.

Output feeds the CLI (`discover --json`), the menu (D) and the desktop app (E).
Showing the account email (stored in `~/.claude.json`) is **opt-in** because it means reading
account metadata; off by default to keep the "never touch account data" promise.
Testing: table tests with fake homes and a fake PATH.

### C. Hand-off ("Continue in…")

Live shared memory between different AIs is not feasible: each stores conversations in its own
private, changing format. Instead, devswitch writes a hand-off note and starts the next AI with it.

Flow for `devswitch continue <from-provider> <from-profile> <to-provider> <to-profile>` (run inside the project folder):

1. Find the latest conversation for the current folder:
   - Claude: `<home>/projects/<folder-path-with-dashes>/<session>.jsonl`, newest by modification time.
   - Codex: `<home>/sessions/YYYY/MM/DD/rollout-*.jsonl` whose `session_meta.payload.cwd` matches.
   - Gemini: same idea once its format is confirmed (spike task inside this plan).
2. Extract, without any AI call: the user's messages, the assistant's final replies, and every file
   path read or edited by tools. Write `~/.devswitch/handoffs/<timestamp>.md`.
3. Start the target AI in the same folder with the note path as the first prompt
   (`claude "<prompt>"`, `codex "<prompt>"`), e.g.
   *"You are continuing work started in another AI. Read <note path>, summarize where we are, then continue."*
   The target AI does the summarizing, so devswitch needs no API key.

Cheap bonus, opt-in: one shared instructions file that every AI reads (link Codex `AGENTS.md`
and Claude `CLAUDE.md` to one file). This deliberately relaxes today's "same provider only" sharing rule,
only for that file.

Errors: no conversation found for this folder → clear message listing folders that have conversations.
Note files can contain sensitive conversation text: created with `0600` permissions, listed and deletable from the UI.
Testing: fixture `.jsonl` files for each provider; golden-file test of the generated note.

### D. Terminal menu

`devswitch` with no arguments, in an interactive terminal, opens a menu; with arguments or without a terminal,
behavior is unchanged (scripts keep working).

Screens: account list grouped by AI with status from B → actions *Open*, *Continue in…*, *Add account*,
*Share data*, *Settings*. Adding an account asks only "Which AI?" and "Name it (work, personal…)",
then offers to log in right away.
Testing: Bubble Tea model unit tests (send key messages, assert state) — no screen snapshots.

### E. Desktop app

A window with one card per AI (Claude, Codex, Gemini, + Add AI). Each card lists accounts with
an *Open* button, a status dot from B, and *Continue in…*. A first-run wizard runs detection and offers
to adopt existing logins.

Main decision: **where the AI runs when you click Open**.

- **Recommended: inside the app** — an embedded terminal tab (xterm.js + pty). The user never sees a
  separate terminal window; it feels like one app. More work, but it is what makes this usable for non-technical people.
- Alternative (faster first version): open the system terminal (macOS Terminal, the Linux default terminal, Windows Terminal).

Why Wails: the backend is Go, so it calls `internal/profile`, `internal/provider` and `internal/handoff`
directly; small binaries; native window. Tauri would need a Rust layer around the Go code; Electron is ~100 MB per install.

The app only runs while its window is open, which keeps the README's "no background daemon" promise.
Packaging: Linux AppImage + `.deb`, macOS `.dmg`. Signed and notarized macOS builds need an Apple Developer account ($99/year);
without one, users must right-click → Open the first time.
Testing: Go backend methods unit-tested; one Playwright smoke test of the frontend against the dev server.

### F. More AIs

`~/.devswitch/providers.json` adds entries with `id`, `binary`, `homeEnvVar`, `defaultHome`, `loginArgs`,
`installUrl`. Built-ins stay in code and cannot be overridden by name. The desktop app's "+ Add AI"
writes this file. Sharing and hand-off are available only for providers with a known layout.
Testing: load/validate tests, including rejection of relative paths and duplicate IDs.

### G. Windows

- `fileutil.WithLock` on Windows (`LockFileEx`); currently it returns an error there.
- Sharing: directory junctions instead of symlinks (no admin rights needed); shared single files are copied, with a warning.
- Shell integration: PowerShell profile block; `install.ps1`; the desktop app ships an NSIS installer.
- CI: add `windows-latest` to the test matrix and Windows assets to releases.

## Decisions (approved 2026-10-07)

1. Desktop app runs the AI in an embedded terminal inside the app window.
2. Account emails are opt-in, off by default.
3. The shared instructions file across different AIs is part of C.
