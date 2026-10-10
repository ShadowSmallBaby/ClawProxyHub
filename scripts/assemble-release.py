"""Assemble the full distribution with embedded Web, CLI and signed packages."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import zipfile

from release_config import ROOT, DEFAULT_CONFIG, read_config, run, digest, package_metadata, distribution_presets, frontend_output
from project_config import read_project


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('core', 'cli', 'assembler', 'version', 'platform', 'out', 'packages'):
        parser.add_argument('--' + name, required=True)
    parser.add_argument('--config', default=str(DEFAULT_CONFIG))
    parser.add_argument('--frontend', help='Web frontend preset for the full distribution')
    parser.add_argument('--distribution', action='append', choices=['full'])
    parser.add_argument('--trust', help='Defaults to <packages>/trust.json')
    args = parser.parse_args()
    config = read_config(args.config)
    project = read_project()
    package_dir = Path(args.packages).resolve()
    presets = distribution_presets(config, args.distribution, args.frontend)
    required = {identifier for preset in presets.values() for identifier in preset['packages']}
    trust_path = Path(args.trust or package_dir / 'trust.json')
    trust = json.loads(trust_path.read_text(encoding='utf-8') if trust_path.exists() else os.environ.get('CPH_EXTENSION_TRUST_JSON', '{}'))
    packages = {}
    for path in sorted(package_dir.iterdir()) if package_dir.exists() else []:
        if path.suffix not in ('.cphext', '.cphhost'):
            continue
        item = package_metadata(path)
        if item['id'] not in required or (item['platforms'] and args.platform not in item['platforms']):
            continue
        expected = project['components'][item['id']]['version']
        if item['version'] != expected:
            continue
        if item['id'] in packages:
            raise ValueError(f'Duplicate package: {item["id"]}')
        packages[item['id']] = path
    if required != set(packages):
        raise ValueError(f'Missing configured packages: {sorted(required - set(packages))}')
    output = Path(args.out).resolve()
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='cph-release-') as temporary:
        stage = Path(temporary)
        for profile, settings in presets.items():
            web = frontend_output(settings['frontend'], config, project)
            identifiers = settings['packages']
            request = stage / (profile + '.json')
            request.write_text(json.dumps({
                'profile': profile, 'core': str(Path(args.core).resolve()),
                'cli': str(Path(args.cli).resolve()),
                'version': args.version, 'platform': args.platform, 'trust': trust,
                'frontend': {k: v for k, v in json.loads((web / 'profile.json').read_text(encoding='utf-8')).items() if k in ('profile', 'version', 'packages')},
                'readme': str(ROOT / 'internal/distribution/QUICKSTART.md'), 'license': str(ROOT / 'LICENSE'),
                'packages': [str(packages[p]) for p in identifiers],
            }), encoding='utf-8')
            target = stage / profile
            run([Path(args.assembler).resolve(), '--manifest', request, '--out', target], stdout=subprocess.DEVNULL)
            lock = json.loads((target / 'distribution.lock.json').read_text(encoding='utf-8'))
            archive = output / ('cph-' + args.platform.replace('/', '-') + '-' + profile + '.zip')
            with zipfile.ZipFile(archive, 'x', zipfile.ZIP_DEFLATED) as result:
                for path in sorted(target.rglob('*')):
                    if not path.is_file():
                        continue
                    name = path.relative_to(target).as_posix()
                    entry = zipfile.ZipInfo('ClawProxyHub/' + name, (2020, 1, 1, 0, 0, 0))
                    entry.compress_type = zipfile.ZIP_DEFLATED
                    entry.create_system = 3
                    entry.external_attr = (0o100755 if name in (lock['core'], lock['cli']) else 0o100644) << 16
                    result.writestr(entry, path.read_bytes())
            archive.with_suffix('.zip.sha256').write_text(digest(archive) + '  ' + archive.name + '\n', encoding='utf-8')
        # 独立附件与发行组合复用完全相同的签名包。
        for path in packages.values():
            shutil.copyfile(path, output / path.name)
        print('Distribution core SHA256:', lock['core_sha256'])


if __name__ == '__main__':
    main()
