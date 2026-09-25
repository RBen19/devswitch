#!/usr/bin/env python3
"""Validate checksums, archive structure, platform headers, and the native binary."""
import hashlib
import os
import pathlib
import platform
import re
import subprocess
import sys
import tarfile
import tempfile


def verify(directory, tag):
    if not re.fullmatch(r'v\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?', tag):
        raise ValueError('Invalid version tag')
    root = pathlib.Path(directory)
    checksums = {}
    for line in (root/'checksums.txt').read_text().splitlines():
        checksum, name = line.split()
        if name in checksums or pathlib.Path(name).name != name:
            raise ValueError('Duplicate or unsafe checksum path')
        checksums[name] = checksum
    expected = {'install.sh'} | {f'devswitch_{tag[1:]}_{system}_{arch}.tar.gz' for system in ('linux', 'darwin') for arch in ('arm64', 'amd64')}
    if set(checksums) != expected:
        raise ValueError('Missing or extra release checksums')
    for name, checksum in checksums.items():
        if hashlib.sha256((root/name).read_bytes()).hexdigest() != checksum:
            raise ValueError(f'Checksum mismatch: {name}')
        if not name.endswith('.tar.gz'):
            continue
        with tarfile.open(root/name) as archive:
            members = archive.getmembers()
            if len(members) != 1 or members[0].name != 'devswitch' or not members[0].isfile() or members[0].mode != 0o755:
                raise ValueError(f'Invalid archive: {name}')
            data = archive.extractfile(members[0]).read()
        system, arch = name.removesuffix('.tar.gz').rsplit('_', 2)[1:]
        if system == 'linux':
            machine = int.from_bytes(data[18:20], 'little')
            if data[:4] != b'\x7fELF' or data[4:6] != b'\x02\x01' or machine != {'amd64': 62, 'arm64': 183}[arch]:
                raise ValueError(f'Invalid Linux architecture: {name}')
        else:
            cpu = int.from_bytes(data[4:8], 'little')
            if data[:4] != b'\xcf\xfa\xed\xfe' or cpu != {'amd64': 0x1000007, 'arm64': 0x100000c}[arch]:
                raise ValueError(f'Invalid macOS architecture: {name}')
        native_arch = 'arm64' if platform.machine() in ('arm64', 'aarch64') else 'amd64'
        if system == platform.system().lower() and arch == native_arch:
            with tempfile.TemporaryDirectory() as tmp:
                binary = pathlib.Path(tmp)/'devswitch'
                binary.write_bytes(data)
                binary.chmod(0o755)
                output = subprocess.check_output([str(binary), '--version'], text=True).strip()
                if output != 'devswitch version '+tag[1:]:
                    raise ValueError(f'Wrong embedded version: {output}')
    print(f'Verified {tag}: checksums, four architectures, archive safety, native executable')


if __name__ == '__main__':
    verify(sys.argv[1], sys.argv[2])
