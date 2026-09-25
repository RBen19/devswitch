#!/usr/bin/env python3
"""Allocate immutable patch tags; retry races and reuse tags on workflow reruns."""
import argparse
import os
import re
import subprocess
import time

STABLE = re.compile(r'^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$')


def git(*args):
    return subprocess.check_output(['git', *args], text=True).strip()


def version_key(tag):
    return tuple(map(int, STABLE.fullmatch(tag).groups()))


def next_tag(tags, at_commit):
    stable = [tag for tag in tags if STABLE.fullmatch(tag)]
    existing = [tag for tag in at_commit if tag in stable]
    if existing:
        return max(existing, key=version_key)
    if not stable:
        return 'v0.2.0'
    major, minor, patch = version_key(max(stable, key=version_key))
    return f'v{major}.{minor}.{patch + 1}'


def remote_tag_targets(snapshot):
    direct, peeled = {}, {}
    for line in snapshot.splitlines():
        commit, ref = line.split('\t')
        name = ref.removeprefix('refs/tags/')
        if name.endswith('^{}'):
            peeled[name[:-3]] = commit
        else:
            direct[name] = commit
    # Annotated tags point to tag objects; their peeled refs identify the commit.
    direct.update(peeled)
    return direct


def allocate(commit, push):
    commit = git('rev-parse', '--verify', '--end-of-options', commit + '^{commit}')
    for attempt in range(10):
        # Read names and target commits in one snapshot. A fetch followed by a
        # separate ls-remote can miss a concurrently created tag's commit mapping.
        remote = git('ls-remote', '--tags', 'origin')
        targets = remote_tag_targets(remote)
        tags = list(targets)
        at_commit = [name for name, target in targets.items() if target == commit]
        tag = next_tag(tags, at_commit)
        if tag in tags or not push:
            return tag
        result = subprocess.run(['git', 'push', 'origin', f'{commit}:refs/tags/{tag}'], text=True)
        if result.returncode == 0:
            return tag
        # Another successful build may have allocated this version first.
        time.sleep(min(attempt + 1, 5))
    raise RuntimeError('Could not push a release tag after 10 attempts; check repository permissions and network access')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--commit', default='HEAD')
    parser.add_argument('--push', action='store_true', help='create the remote tag (otherwise only print the proposed tag)')
    args = parser.parse_args()
    tag = allocate(args.commit, args.push)
    print(tag)
    if os.environ.get('GITHUB_OUTPUT'):
        with open(os.environ['GITHUB_OUTPUT'], 'a') as output:
            output.write(f'tag={tag}\n')


if __name__ == '__main__':
    main()
