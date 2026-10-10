"""Validate the common desktop ZIP layout before publishing or preparing Docker."""
import hashlib
import json
from pathlib import PurePosixPath
import re
import stat

PREFIX = 'ClawProxyHub/'


def read_distribution(archive, platform, version):
    try:
        lock = json.loads(archive.read(PREFIX + 'distribution.lock.json'))
    except (KeyError, ValueError) as error:
        raise ValueError('Missing or invalid distribution metadata') from error
    suffix = '.exe' if platform.startswith('windows/') else ''
    if (lock.get('format') != 1 or lock.get('profile') != 'full' or lock.get('platform') != platform
            or lock.get('version') != version or lock.get('core') != 'cph' + suffix or lock.get('cli') != 'cli' + suffix):
        raise ValueError('Distribution metadata mismatch')
    files = dict(lock['files'])
    if (files.get(lock['core']) != lock.get('core_sha256') or lock['cli'] not in files
            or not isinstance(lock.get('frontend'), dict)):
        raise ValueError('Incomplete full distribution')
    for package in lock['packages']:
        name = package['path']
        if name in files or not name.startswith('data/packages/'):
            raise ValueError('Invalid distribution package path')
        files[name] = package['sha256']
    expected = {PREFIX + name for name in files} | {PREFIX + 'distribution.lock.json'}
    members = archive.infolist()
    if len(members) != len(expected) or {item.filename for item in members} != expected:
        raise ValueError('Distribution file list mismatch')
    for member in members:
        name = member.filename.removeprefix(PREFIX)
        relative = PurePosixPath(name)
        mode = stat.S_IFMT(member.external_attr >> 16)
        if (relative.is_absolute() or '..' in relative.parts or name != relative.as_posix()
                or '\\' in name or ':' in name or member.is_dir() or mode not in (0, stat.S_IFREG)
                or member.file_size > 256 << 20):
            raise ValueError(f'Unsafe distribution path: {name}')
        if name in files:
            if not re.fullmatch('[0-9a-f]{64}', files[name]):
                raise ValueError(f'Invalid distribution checksum: {name}')
            with archive.open(member) as stream:
                actual = hashlib.file_digest(stream, 'sha256').hexdigest()
            if actual != files[name]:
                raise ValueError(f'Distribution file checksum mismatch: {name}')
    return lock
