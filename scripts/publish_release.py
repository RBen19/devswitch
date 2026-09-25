#!/usr/bin/env python3
"""Finish a release idempotently; never replace published assets."""
import json
import pathlib
import re
import subprocess
import sys


def gh(*args):
    return subprocess.run(['gh', *args], check=True, capture_output=True, text=True).stdout


def publish(tag, directory):
    if not re.fullmatch(r'v\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?', tag):
        raise ValueError('Invalid version tag')
    # Listing avoids mistaking authentication/network failures for a missing tag.
    releases = json.loads(gh('api', '--paginate', '--slurp', 'repos/{owner}/{repo}/releases?per_page=100'))
    release = next((r for page in releases for r in page if r['tag_name'] == tag), None)
    if release is not None and not release['draft']:
        print(f'{tag} is already published; assets left unchanged')
        return
    if release is None:
        gh('release', 'create', tag, '--verify-tag', '--draft', '--generate-notes', '--title', f'devswitch {tag}')
    root = pathlib.Path(directory)
    assets = [root/'install.sh', root/'checksums.txt', *sorted(root.glob(f'devswitch_{tag[1:]}_*.tar.gz'))]
    if len(assets) != 6 or not all(p.is_file() for p in assets):
        raise ValueError('Expected installer, checksums, and four platform archives')
    gh('release', 'upload', tag, *map(str, assets), '--clobber')
    # Download and verify the draft before making it public. A failed upload or
    # verification leaves the draft resumable on the next workflow run.
    import tempfile
    with tempfile.TemporaryDirectory() as tmp:
        gh('release', 'download', tag, '--dir', tmp)
        subprocess.run([sys.executable, str(pathlib.Path(__file__).with_name('verify_release.py')), tmp, tag], check=True)
    # Ask GitHub to determine latest server-side by semantic version, avoiding a
    # race where an older CI job finishes after a newer release has published.
    release_id = json.loads(gh('api', 'repos/{owner}/{repo}/releases/tags/'+tag))['id']
    stable = re.fullmatch(r'v\d+\.\d+\.\d+', tag)
    gh('api', '--method', 'PATCH', f'repos/{{owner}}/{{repo}}/releases/{release_id}',
       '-F', 'draft=false', '-F', 'prerelease='+('false' if stable else 'true'),
       '-f', 'make_latest='+('legacy' if stable else 'false'))
    print(f'Published {tag}')


if __name__ == '__main__':
    publish(sys.argv[1], sys.argv[2])
