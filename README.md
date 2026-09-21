# devswitch

> Switch cleanly between Claude Code and Codex profiles. · by RBen19

`devswitch` is a small local CLI that keeps Claude Code and Codex logins isolated. It delegates authentication to the official provider CLIs and never reads, copies, or manages credentials.

## Requirements

For building from source:

- Git
- Go 1.26 or newer

For using a provider profile, install the provider CLI separately:

- Claude Code: the `claude` command must be available in `PATH`.
- Codex: the `codex` command must be available in `PATH`.

Go is required to build devswitch from source, but it is not required to run a compiled devswitch binary.

## Quick start from a clone

```bash
git clone <repository-url>
cd devswitch
make build
./devswitch install
```

`devswitch install` asks for confirmation, detects zsh or bash, and adds the directory containing the current executable to the appropriate shell configuration file. Reload the file it reports, or open a new terminal:

```bash
source ~/.zshrc   # zsh
source ~/.bashrc  # bash
```

You can skip the confirmation with:

```bash
./devswitch install --yes
```

Then check the installation:

```bash
devswitch --help
devswitch list
```

## Create and use profiles

Create one profile per account or workspace:

```bash
devswitch add claude personal
devswitch add claude work
devswitch add codex personal
devswitch add codex work
```

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
