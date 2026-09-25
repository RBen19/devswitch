#!/usr/bin/env python3
import importlib.util
import os
import pathlib
import subprocess
import tempfile
import unittest
from unittest import mock
import json

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('tag_release', ROOT/'scripts/tag-release.py')
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class ReleaseTests(unittest.TestCase):
    def test_version_selection(self):
        self.assertEqual(release.next_tag([], []), 'v0.2.0')
        self.assertEqual(release.next_tag(['v0.2.9', 'v0.2.10', 'v5.0.0-rc.1', 'other'], []), 'v0.2.11')
        self.assertEqual(release.next_tag(['v1.0.0', 'v0.2.0'], ['v0.2.0']), 'v0.2.0')
        self.assertEqual(release.next_tag(['v0.2.0'], ['v5.0.0']), 'v0.2.1')

    def test_remote_snapshot_resolves_annotated_and_lightweight_tags(self):
        snapshot = 'object\trefs/tags/v1.0.0\ncommit-a\trefs/tags/v1.0.0^{}\ncommit-b\trefs/tags/v1.0.1\n'
        targets = release.remote_tag_targets(snapshot)
        self.assertEqual(targets, {'v1.0.0': 'commit-a', 'v1.0.1': 'commit-b'})
        self.assertEqual(release.next_tag(list(targets), [tag for tag, commit in targets.items() if commit == 'commit-a']), 'v1.0.0')

    def test_remote_tags_rerun_and_concurrent_pushes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            remote = root/'remote.git'
            subprocess.run(['git', 'init', '--bare', str(remote)], check=True, capture_output=True)
            work = root/'work'
            subprocess.run(['git', 'clone', str(remote), str(work)], check=True, capture_output=True)
            def git(*args):
                return subprocess.check_output(['git', '-C', str(work), *args], text=True, stderr=subprocess.DEVNULL).strip()
            git('config', 'user.email', 'test@example.invalid')
            git('config', 'user.name', 'Test')
            git('commit', '--allow-empty', '-m', 'first')
            git('push', 'origin', 'HEAD')
            def tag(commit='HEAD'):
                return subprocess.check_output(['python3', str(ROOT/'scripts/tag-release.py'), '--push', '--commit', commit], cwd=work, text=True, stderr=subprocess.DEVNULL).strip()
            self.assertEqual(tag(), 'v0.2.0')
            self.assertEqual(tag(), 'v0.2.0')
            git('tag', 'v99.0.0')  # unpushed local tags are ignored
            git('commit', '--allow-empty', '-m', 'second')
            second = git('rev-parse', 'HEAD')
            git('commit', '--allow-empty', '-m', 'third')
            third = git('rev-parse', 'HEAD')
            git('push', 'origin', 'HEAD')
            # Independent clones simulate successful CI jobs finishing together.
            clone = root/'other'
            subprocess.run(['git', 'clone', str(remote), str(clone)], check=True, capture_output=True)
            jobs = [subprocess.Popen(['python3', str(ROOT/'scripts/tag-release.py'), '--push', '--commit', commit], cwd=folder, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                    for folder, commit in [(work, second), (clone, third)]]
            tags = set()
            for job in jobs:
                out, err = job.communicate(timeout=30)
                self.assertEqual(job.returncode, 0, err)
                tags.add(out.strip())
            self.assertEqual(tags, {'v0.2.1', 'v0.2.2'})
            self.assertIn(tag(second), tags)
            self.assertIn(tag(third), tags)
            # Simulate a release tag arriving remotely after this clone's last fetch.
            subprocess.run(['git', '-C', str(clone), 'config', 'user.email', 'test@example.invalid'], check=True)
            subprocess.run(['git', '-C', str(clone), 'config', 'user.name', 'Test'], check=True)
            subprocess.run(['git', '-C', str(clone), 'tag', '-a', 'v1.0.0', '-m', 'annotated release', third], check=True)
            subprocess.run(['git', '-C', str(clone), 'push', 'origin', 'refs/tags/v1.0.0'], check=True, capture_output=True)
            self.assertEqual(tag(third), 'v1.0.0')
            self.assertNotIn('v1.0.0', git('tag', '--list').splitlines())



class PublishTests(unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location('publish_release', ROOT/'scripts/publish_release.py')
        self.module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.module)

    def test_published_release_is_never_modified(self):
        with mock.patch.object(self.module, 'gh', return_value=json.dumps([[{'tag_name': 'v0.2.0', 'draft': False}]])) as gh:
            self.module.publish('v0.2.0', '/does-not-exist')
            self.assertEqual(gh.call_count, 1)
            self.assertEqual(gh.call_args.args[0], 'api')

    def test_draft_retry_and_failed_verification(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            for name in ['install.sh', 'checksums.txt', *[f'devswitch_0.2.0_{system}_{arch}.tar.gz' for system in ('linux', 'darwin') for arch in ('amd64', 'arm64')]]:
                (root/name).touch()
            calls = []
            def gh(*args):
                calls.append(args)
                if '--paginate' in args:
                    return json.dumps([[{'tag_name': 'v0.2.0', 'draft': True, 'id': 123}]])
                if args[:2] == ('api', 'repos/{owner}/{repo}/releases/tags/v0.2.0'):
                    return json.dumps({'id': 123})
                return ''
            with mock.patch.object(self.module, 'gh', side_effect=gh), mock.patch.object(self.module.subprocess, 'run', side_effect=subprocess.CalledProcessError(1, 'verify')):
                with self.assertRaises(subprocess.CalledProcessError):
                    self.module.publish('v0.2.0', root)
                self.assertFalse(any('PATCH' in args for args in calls))
            calls.clear()
            with mock.patch.object(self.module, 'gh', side_effect=gh), mock.patch.object(self.module.subprocess, 'run'):
                self.module.publish('v0.2.0', root)
                self.assertFalse(any(args[:2] == ('release', 'create') for args in calls))
                self.assertTrue(any('PATCH' in args and 'make_latest=legacy' in args for args in calls))


if __name__ == '__main__':
    unittest.main()
