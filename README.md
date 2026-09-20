# devswitch

> Switch cleanly between Claude Code and Codex profiles. · by RBen19

`devswitch` is a small local CLI that keeps provider logins isolated without copying or handling credentials.

## MVP

```bash
go run ./cmd/devswitch add claude perso
go run ./cmd/devswitch login claude perso
go run ./cmd/devswitch run claude perso

go run ./cmd/devswitch add codex travail
go run ./cmd/devswitch login codex travail
go run ./cmd/devswitch run codex travail
```

Use `--` to pass arguments to the underlying tool:

```bash
devswitch run codex travail -- --full-auto
```

Profiles live under `~/.devswitch/profiles` and metadata under `~/.devswitch/profiles.json`, with restrictive local permissions. Authentication is delegated to the official provider CLIs and their browser flows.

## Build and runtime requirements

Go is required only to build or install devswitch from source. It is not required to run a compiled devswitch binary.

Development commands:

```bash
go build -ldflags "-X github.com/RBen19/devswitch/internal/cli.version=0.1.0" -o devswitch ./cmd/devswitch
make build
make test
```

`make install` also requires Go and installs the binary into Go's user bin directory. After that, to use `devswitch` directly from any directory:

```bash
make install
export PATH="$(go env GOPATH)/bin:$PATH"
```

Add this line to `~/.zshrc` to keep it after restarting your terminal.

From the project directory, use `./devswitch` if the binary is not installed in `PATH` yet.

After installing or downloading a compiled binary, devswitch can configure `PATH` automatically without requiring Go:

```bash
./devswitch install
```

The command uses the directory containing the current executable, asks for confirmation, detects zsh or bash, avoids duplicates, and only changes the shell configuration file. Use `./devswitch install --yes` to skip confirmation.

The project never assumes a fixed machine path such as `/home/user/project`. User directories are resolved at runtime, and profile data is stored under the current user's `~/.devswitch` directory.

## Deliberately limited scope

- no credential scraping or token management;
- no account API calls;
- no background daemon;
- no automatic email/code handling.
