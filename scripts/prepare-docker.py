"""Prepare a Docker context from the verified Linux full release artifacts."""
import argparse
import json
from pathlib import Path
import tempfile
import zipfile

from project_config import read_project
from release_config import digest
from distribution_archive import PREFIX, read_distribution


def extract(archive_path, destination, platform, version):
    with zipfile.ZipFile(archive_path) as archive:
        lock = read_distribution(archive, platform, version)
        for member in archive.infolist():
            name = member.filename.removeprefix(PREFIX)
            data = archive.read(member)
            target = destination / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(data)
            target.chmod(0o755 if name in (lock['core'], lock['cli']) else 0o644)


def prepare(assets, output, project):
    assets, output = Path(assets).resolve(), Path(output).resolve()
    if output.exists():
        raise ValueError('Docker context output must not exist')
    manifest = json.loads((assets / 'update-manual.json').read_text(encoding='utf-8'))
    version = project['components']['core']['version']
    if manifest.get('schema_version') != 1 or manifest.get('version') != version:
        raise ValueError('Update manifest version mismatch')
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='cph-docker-', dir=output.parent) as temporary:
        stage = Path(temporary) / 'context'
        stage.mkdir()
        for arch in ('amd64', 'arm64'):
            platform = 'linux/' + arch
            item = manifest.get('artifacts', {}).get(platform, {}).get('full', {})
            name = f'cph-linux-{arch}-full.zip'
            if item.get('name') != name:
                raise ValueError(f'Missing Linux full artifact: {platform}')
            archive = assets / name
            if archive.stat().st_size != item.get('size') or digest(archive) != item.get('sha256'):
                raise ValueError(f'Release artifact checksum mismatch: {name}')
            extract(archive, stage / arch, platform, version)
        stage.rename(output)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--assets', required=True, help='Linux release ZIPs and update-manual.json from the same workflow run')
    parser.add_argument('--out', required=True, help='New directory for the multi-platform Docker context')
    args = parser.parse_args()
    prepare(args.assets, args.out, read_project())


if __name__ == '__main__':
    main()
