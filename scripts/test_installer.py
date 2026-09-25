#!/usr/bin/env python3
"""Exercise the real installer offline, intercepting only HTTPS downloads."""
import hashlib
import io
import os
import pathlib
import shutil
import subprocess
import tarfile
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="devswitch installer ' $(nope) ")
        self.addCleanup(self.tmp.cleanup)
        self.root = pathlib.Path(self.tmp.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.downloads = self.root / 'downloads'
        self.downloads.mkdir()
        self.dest = self.root / 'installed bin'
        self.dest.mkdir()
        self.home = self.root / 'home'
        self.home.mkdir()
        self.fake_curl = self.bin / 'curl'
        self.fake_curl.write_text('''#!/usr/bin/env python3
import os,pathlib,shutil,sys
args=sys.argv[1:]
assert args[args.index('--proto')+1]=='=https'
if '-w' in args:
 print('https://github.com/RBen19/devswitch/releases/tag/v0.2.0',end='')
 sys.exit(0)
url=next(a for a in args if a.startswith('https://'))
assert '/download/v0.2.0/' in url
shutil.copyfile(pathlib.Path(os.environ['DOWNLOADS'])/url.rsplit('/',1)[1],args[args.index('-o')+1])
''')
        self.fake_curl.chmod(0o755)
        self.env = dict(os.environ, HOME=str(self.home), SHELL='/bin/bash',
                        PATH=str(self.bin)+os.pathsep+os.environ['PATH'], DOWNLOADS=str(self.downloads))
        self.asset = f"devswitch_0.2.0_{'darwin' if os.uname().sysname == 'Darwin' else 'linux'}_{'arm64' if os.uname().machine in ('arm64', 'aarch64') else 'amd64'}.tar.gz"
        self.package()

    def package(self, binary=b'#!/bin/sh\necho "devswitch version 0.2.0"\n', filename='devswitch', symlink=False):
        with tarfile.open(self.downloads / self.asset, 'w:gz') as archive:
            info = tarfile.TarInfo(filename)
            info.mode = 0o755
            if symlink:
                info.type, info.linkname = tarfile.SYMTYPE, '/bin/sh'
                archive.addfile(info)
            else:
                info.size = len(binary)
                archive.addfile(info, io.BytesIO(binary))
        checksum = hashlib.sha256((self.downloads / self.asset).read_bytes()).hexdigest()
        (self.downloads / 'checksums.txt').write_text(f'{checksum}  {self.asset}\n')

    def run_install(self, *args, ok=True, setup=False):
        result = subprocess.run(['bash', str(ROOT/'install.sh'), '--install-dir', str(self.dest), *([] if setup else ['--no-setup']), *args],
                                env=self.env, capture_output=True, text=True)
        self.assertEqual(result.returncode == 0, ok, result.stdout+result.stderr)
        return result

    def test_install_upgrade_and_pinned_version(self):
        self.run_install()
        self.assertTrue(os.access(self.dest/'devswitch', os.X_OK))
        (self.dest/'devswitch').write_text('old installation')
        self.run_install('--version', 'v0.2.0')
        self.assertIn('0.2.0', (self.dest/'devswitch').read_text())
        self.assertFalse(list(self.dest.glob('.devswitch-install.*')))

    def test_bad_checksum_preserves_existing_install(self):
        (self.dest/'devswitch').write_text('old')
        (self.downloads/'checksums.txt').write_text('0'*64+'  '+self.asset+'\n')
        result = self.run_install(ok=False)
        self.assertIn('Checksum mismatch', result.stderr)
        self.assertEqual((self.dest/'devswitch').read_text(), 'old')

    def test_missing_checksum(self):
        (self.downloads/'checksums.txt').write_text('')
        self.run_install(ok=False)
        self.assertFalse((self.dest/'devswitch').exists())

    def test_missing_asset(self):
        (self.downloads/self.asset).unlink()
        self.run_install(ok=False)
        self.assertFalse((self.dest/'devswitch').exists())

    def test_invalid_archive_and_symlink(self):
        self.package(filename='../devswitch')
        self.run_install(ok=False)
        self.package(symlink=True)
        self.run_install(ok=False)
        self.assertFalse((self.dest/'devswitch').exists())

    def test_binary_cannot_run(self):
        self.package(binary=b'#!/bin/sh\nexit 1\n')
        self.run_install(ok=False)
        self.assertFalse((self.dest/'devswitch').exists())

    def test_real_binary_onboarding(self):
        binary = self.root / 'devswitch-real'
        subprocess.run(['go', 'build', '-o', str(binary), './cmd/devswitch'], cwd=ROOT, check=True)
        self.package(binary=binary.read_bytes())
        (self.home / '.codex').mkdir()
        self.run_install('--shell', 'bash', setup=True)
        self.assertIn('# >>> devswitch >>>', (self.home / '.bashrc').read_text())
        self.assertIn('# >>> devswitch >>>', (self.home / '.bash_profile').read_text())
        self.assertTrue((self.home / '.devswitch/shell/completion.bash').exists())
        self.assertIn('personal', (self.home / '.devswitch/profiles.json').read_text())

    def test_arguments(self):
        self.run_install('--version', 'invalid', ok=False)
        self.run_install('--shell', 'invalid', ok=False)
        self.run_install('--install-dir', 'relative', ok=False)
        self.run_install('--unknown', ok=False)


if __name__ == '__main__':
    unittest.main()
