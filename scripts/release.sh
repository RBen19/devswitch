#!/usr/bin/env bash
# Build deterministic archives for every supported platform.
set -euo pipefail
version="${1:?Usage: scripts/release.sh vX.Y.Z [output-directory]}"
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]] || { echo 'Invalid release version' >&2; exit 1; }
output="${2:-dist}"
mkdir -p "$output"
output="$(cd "$output" && pwd)"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
for platform in darwin linux; do
  for arch in amd64 arm64; do
    CGO_ENABLED=0 GOOS="$platform" GOARCH="$arch" go build -trimpath -ldflags "-s -w -buildid= -X github.com/RBen19/devswitch/internal/cli.version=${version#v}" -o "$stage/devswitch" ./cmd/devswitch
    # Use Python's standard library for deterministic tar metadata and gzip headers.
    python3 - "$stage/devswitch" "$output/devswitch_${version#v}_${platform}_${arch}.tar.gz" <<'PY'
import gzip, io, pathlib, sys, tarfile
binary, output = map(pathlib.Path, sys.argv[1:])
with output.open('wb') as stream, gzip.GzipFile(filename='', mode='wb', fileobj=stream, mtime=0) as compressed:
    with tarfile.open(fileobj=compressed, mode='w') as archive:
        info = tarfile.TarInfo('devswitch')
        data = binary.read_bytes()
        info.size, info.mode, info.mtime = len(data), 0o755, 0
        archive.addfile(info, io.BytesIO(data))
PY
  done
done
cp install.sh "$output/install.sh"
python3 - "$output" "${version#v}" <<'PY'
import hashlib, pathlib, sys
root, version = pathlib.Path(sys.argv[1]), sys.argv[2]
files = sorted(root.glob(f'devswitch_{version}_*.tar.gz')) + [root / 'install.sh']
(root / 'checksums.txt').write_text(''.join(f'{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n' for p in files))
PY
printf 'Release artifacts: %s\n' "$output"
