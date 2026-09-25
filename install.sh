#!/usr/bin/env bash
# Install official devswitch release binaries without Go or administrator access.
# Usage: curl -fsSL <release-install-url> | bash
set -euo pipefail

main() {
  local version="${DEVSWITCH_VERSION:-latest}"
  local install_dir="${DEVSWITCH_INSTALL_DIR:-${HOME:?HOME is required}/.local/bin}"
  local setup=1 selected_shell="" arg
  while [ "$#" -gt 0 ]; do
    arg="$1"; shift
    case "$arg" in
      --version) [ "$#" -gt 0 ] || fail '--version requires a value'; version="$1"; shift ;;
      --install-dir) [ "$#" -gt 0 ] || fail '--install-dir requires a value'; install_dir="$1"; shift ;;
      --shell) [ "$#" -gt 0 ] || fail '--shell requires a value'; selected_shell="$1"; shift ;;
      --no-setup) setup=0 ;;
      --help|-h) printf '%s\n' 'Usage: install.sh [--version vX.Y.Z] [--install-dir DIR] [--shell bash|zsh|fish] [--no-setup]'; return ;;
      *) fail "Unknown option: $arg" ;;
    esac
  done
  case "$install_dir" in /*) ;; *) fail 'Installation directory must be an absolute path' ;; esac
  case "$version" in latest) ;; v[0-9]*) [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]] || fail 'Use a version such as v0.2.0' ;; *) fail 'Use latest or a version such as v0.2.0' ;; esac
  case "$selected_shell" in ''|bash|zsh|fish) ;; *) fail 'Supported shells: bash, zsh, fish' ;; esac
  local platform arch
  case "$(uname -s)" in Darwin) platform=darwin ;; Linux) platform=linux ;; *) fail 'Supported platforms: macOS and Linux' ;; esac
  case "$(uname -m)" in arm64|aarch64) arch=arm64 ;; x86_64|amd64) arch=amd64 ;; *) fail 'Supported architectures: arm64 and amd64' ;; esac
  command -v curl >/dev/null || fail 'curl is required'
  command -v tar >/dev/null || fail 'tar is required'
  local checksum_tool
  if command -v sha256sum >/dev/null; then checksum_tool=sha256sum
  elif command -v shasum >/dev/null; then checksum_tool=shasum
  else fail 'sha256sum or shasum is required'; fi

  local repo='https://github.com/RBen19/devswitch/releases'
  local base
  if [ "$version" = latest ]; then
    # Resolve the redirect once so checksum and archive always use the same release.
    local release_url
    release_url="$(curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --retry 3 --connect-timeout 15 --max-time 120 -o /dev/null -w '%{url_effective}' "$repo/latest")" || fail 'No release could be resolved; check network access and published releases'
    version="${release_url##*/}"
    [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]] || fail "Unexpected release URL: $release_url"
  fi
  base="$repo/download/$version"
  local asset="devswitch_${version#v}_${platform}_${arch}.tar.gz"
  installer_stage=""
  installer_temp="$(mktemp -d "${TMPDIR:-/tmp}/devswitch-install.XXXXXXXX")"
  trap 'rm -rf -- "$installer_temp"; if [ -n "$installer_stage" ]; then rm -f -- "$installer_stage"; fi' EXIT
  printf 'Downloading devswitch %s for %s/%s...\n' "$version" "$platform" "$arch"
  curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --retry 3 --connect-timeout 15 --max-time 300 "$base/$asset" -o "$installer_temp/$asset" || fail 'Binary download failed; your existing installation is unchanged'
  curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --retry 3 --connect-timeout 15 --max-time 120 "$base/checksums.txt" -o "$installer_temp/checksums.txt" || fail 'Checksum download failed; your existing installation is unchanged'
  local expected actual
  expected="$(awk -v file="$asset" '$2 == file { print $1 }' "$installer_temp/checksums.txt")"
  [[ "$expected" =~ ^[a-fA-F0-9]{64}$ ]] || fail 'Release checksum is missing or invalid'
  if [ "$checksum_tool" = sha256sum ]; then actual="$(sha256sum "$installer_temp/$asset")"; else actual="$(shasum -a 256 "$installer_temp/$asset")"; fi
  actual="${actual%% *}"
  [ "$actual" = "$expected" ] || fail 'Checksum mismatch; your existing installation is unchanged'
  # Require exactly one member before extraction, including rejecting duplicates.
  [ "$(tar -tzf "$installer_temp/$asset")" = devswitch ] || fail 'Unexpected archive contents'
  # Extract only the expected binary, never arbitrary archive paths.
  tar -xzf "$installer_temp/$asset" -C "$installer_temp" devswitch
  [ -f "$installer_temp/devswitch" ] && [ ! -L "$installer_temp/devswitch" ] || fail 'Archive does not contain a regular devswitch binary'
  chmod 755 "$installer_temp/devswitch"
  "$installer_temp/devswitch" --version || fail 'Downloaded binary cannot run on this system'
  mkdir -p -- "$install_dir"
  [ ! -d "$install_dir/devswitch" ] || fail 'Install target is a directory'
  installer_stage="$(mktemp "$install_dir/.devswitch-install.XXXXXXXX")"
  cp "$installer_temp/devswitch" "$installer_stage"
  chmod 755 "$installer_stage"
  mv -f -- "$installer_stage" "$install_dir/devswitch"
  installer_stage=""
  printf 'Installed %s\n' "$install_dir/devswitch"
  if [ "$setup" -eq 1 ]; then
    local setup_args=(install)
    if [ -n "$selected_shell" ]; then setup_args+=(--shell "$selected_shell"); fi
    if [ -t 1 ] && ( : </dev/tty ) 2>/dev/null; then
      "$install_dir/devswitch" "${setup_args[@]}" </dev/tty || fail "Binary installed. Finish setup with: $install_dir/devswitch install"
    else
      "$install_dir/devswitch" "${setup_args[@]}" --yes || fail "Binary installed. Finish setup with: $install_dir/devswitch install --shell bash --yes"
    fi
  else
    printf 'Shell setup skipped. Run: %s install\n' "$install_dir/devswitch"
  fi
}
fail() { printf 'devswitch installer: %s\n' "$*" >&2; exit 1; }
main "$@"
