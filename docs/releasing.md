# Automatic releases

A push to `master` runs the Linux/macOS test matrix, vulnerability scan, installer
and tagging tests, and all four cross-compilation builds. If these checks pass,
CI automatically tags **that exact commit** with the next stable patch version,
starting at `v0.2.0` when there are no stable tags. Pull requests and other branches
run checks but never create release tags.

The tag allocator reads remote tags, compares semantic versions numerically, and
uses a non-forced Git push. Simultaneous builds retry a tag collision; workflow
reruns reuse a tag already on that commit. No version-bump commit is created, so
there is no recursive push loop. Published tags must remain immutable.

CI calls the reusable Release workflow directly after tagging. This is essential:
GitHub does not launch tag-push workflows for tags pushed with `GITHUB_TOKEN`.
Manual `v*` tag pushes still invoke the same Release workflow, and its manual
`workflow_dispatch` input can retry an existing tag.

The Release workflow checks out the tag, repeats validation, packages macOS/Linux
amd64/arm64 archives and the installer, and checks SHA-256 hashes, archive paths,
architecture headers, and the native executable's version. Assets upload to a
temporary draft. The workflow downloads and verifies that draft before publishing
it automatically. A failed upload or verification leaves the draft available for
a retry. Published assets are never replaced. GitHub's server-side semantic-version
selection determines the latest stable release; prereleases do not become latest.

After publication, CI installs the pinned public release into a temporary home.
For the first release, also verify `releases/latest/download/install.sh` and guided
setup from a clean terminal before removing the README's pending-release notice.

## Local checks and packaging

Requires Go 1.26+, Python 3.9+, Bash, Git, and the shells exercised by the tests.

```bash
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
python3 scripts/test_installer.py
python3 scripts/test_release.py
git diff --check
scripts/release.sh v0.2.0
python3 scripts/verify_release.py dist v0.2.0
```

Packaging normalizes archive metadata. Use the same Go toolchain and commit to
reproduce artifacts. Checksums detect corruption but are distributed with the
release; binaries do not yet carry an independently verified signature or
provenance attestation.

## Repository configuration

Actions must be enabled with permission to request `contents: write`. The tag and
publish jobs request it explicitly; test jobs use read-only permissions. Protected
tag rules must permit the repository's Actions identity to create `v*` tags.
Third-party actions are pinned to immutable commits.

For a major/minor version change, create an explicit version tag on a reviewed
commit. Subsequent automatic tags increment the highest stable version's patch.
Never force-update a released tag or silently replace published assets; fix
problems in a new commit and let the next patch release run.
