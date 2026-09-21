# devswitch

> Switch cleanly between Claude Code and Codex profiles. · by RBen19

`devswitch` is a small local CLI that keeps Claude Code and Codex logins isolated. It delegates authentication to the official provider CLIs and never reads, copies, or manages credentials.

## Why devswitch exists

Developers often use more than one account with the same coding assistant: a personal account, a work account, or separate accounts for different clients and organizations. Most provider CLIs keep one active login and one shared configuration directory by default. Switching accounts manually can log out the previous account, mix sessions and settings, or require repetitive environment-variable commands.

devswitch solves this by giving every provider account a named local profile. Each profile gets its own configuration directory, authentication state, sessions, and provider-specific settings. You can then launch the exact account you need with one predictable command:

```bash
devswitch run claude work
devswitch run codex personal
```

The project is intentionally a lightweight profile manager, not a replacement for Claude Code or Codex. It does not automate provider login pages or handle credentials; it prepares the correct environment and lets the official CLI complete authentication securely.

## Requirements

For building from source:

- Git
- Go 1.26 or newer

For using a provider profile, install the provider CLI separately:

- Claude Code: the `claude` command must be available in `PATH`.
- Codex: the `codex` command must be available in `PATH`.

Go is required to build devswitch from source, but it is not required to run a compiled devswitch binary.

### macOS

macOS is supported on both Apple Silicon (`arm64`) and Intel (`amd64`). The default macOS shell is usually zsh, so `devswitch install` updates `~/.zshrc`. Bash is supported as well and uses `~/.bashrc`.

Build the binary for the Mac you are using:

```bash
# Apple Silicon: M1, M2, M3, M4, ...
GOOS=darwin GOARCH=arm64 go build -o devswitch ./cmd/devswitch

# Intel Mac
GOOS=darwin GOARCH=amd64 go build -o devswitch ./cmd/devswitch
```

The downloaded or built binary must match the Mac architecture. A Linux or Windows binary cannot run natively on macOS.

## Quick start from a clone

```bash
git clone <repository-url>
cd devswitch
make build
./devswitch install
```

`devswitch install` asks for confirmation, detects zsh or bash, adds the directory containing the current executable to the appropriate shell configuration file, and scans for existing Claude Code and Codex installations. Reload the file it reports, or open a new terminal:

```bash
source ~/.zshrc   # zsh
source ~/.bashrc  # bash
```

You can skip the confirmation with:

```bash
./devswitch install --yes
```

To remove the shell integration later:

```bash
devswitch uninstall
```

This removes only the PATH entry and preserves your profiles. To permanently delete all devswitch profiles and settings, use the explicit purge option:

```bash
devswitch uninstall --purge
```

The binary itself is not deleted automatically because it may have been installed by Go, a package manager, or a manual copy. The command prints its exact location so it can be removed safely.

Then check the installation:

```bash
devswitch --help
devswitch list
```

## Create and use profiles

### Adopt an existing configuration

If Claude Code or Codex was already installed and logged in before devswitch, discover it first:

```bash
devswitch discover
```

If an existing configuration is found, assign it a profile name that describes the account or workspace:

```bash
devswitch adopt claude personal
devswitch adopt codex personal
```

Adoption reuses the existing configuration in place. It does not copy or move files, and it does not require logging in again. Choose `work`, `client-a`, or another name instead of `personal` when that better describes the account.

Confirm the result and launch the adopted profiles:

```bash
devswitch list
devswitch run claude personal
devswitch run codex personal
```

### Create a fresh isolated profile

Create one profile per account or workspace:

```bash
devswitch add claude personal
devswitch add claude work
devswitch add codex personal
devswitch add codex work
```

`add` creates a new isolated configuration directory. It is the right command when you want a new account/workspace profile rather than adopting the provider's existing default configuration.

Authenticate a profile through the provider's official flow:

```bash
devswitch login claude personal
```

For Claude Code, type `/login` in the launched session. For Codex, the official browser login flow starts with `codex login`.

Run a provider with a selected profile:

```bash
devswitch run claude work
devswitch run codex personal
```

You can run different profiles at the same time in separate terminals. Pass arguments to the underlying CLI after `--`:

```bash
devswitch run codex work -- --full-auto
```

Shortcuts are available after `devswitch install` and a shell reload:

```bash
dvsw cl p       # devswitch run claude personal
dvsw cl w       # devswitch run claude work
dvsw cx p       # devswitch run codex personal
dvsw cx w       # devswitch run codex work
```

Use `devswitch discover` to detect existing default provider configurations. Adopt one as a named profile without copying or moving it:

```bash
devswitch discover
devswitch adopt claude personal
devswitch adopt codex work
```

## Storage and isolation

Profiles are stored under the current user's home directory:

```text
~/.devswitch/profiles/claude/personal
~/.devswitch/profiles/claude/work
~/.devswitch/profiles/codex/personal
~/.devswitch/profiles/codex/work
```

The registry is stored in `~/.devswitch/profiles.json`. The paths are resolved at runtime; devswitch does not assume a specific username, home directory, or project location.

Existing Claude Code and Codex installations remain unchanged. If a provider CLI is missing, devswitch reports that it must be installed first. Existing profiles are never overwritten by `devswitch add`.

## Development commands

```bash
make build
make test
go vet ./...
```

`make install` uses Go to install the binary into Go's user bin directory. For a source installation, you may then add that directory to `PATH` with:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

## Deliberately limited scope

- no credential scraping or token management;
- no provider account API calls;
- no background daemon;
- no automatic email or verification-code handling;
- no automatic installation of Claude Code or Codex.
