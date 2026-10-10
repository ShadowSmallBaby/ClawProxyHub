"""Generate a shared desktop/Android update manifest from completed release artifacts."""
import argparse
import copy
import json
from pathlib import Path
import re
from urllib.parse import quote
import zipfile

from project_config import ROOT, VERSION_PATTERN, apk_name, compatibility_manifest, read_project
from release_config import digest, package_metadata, read_config, validate_environments
from distribution_archive import read_distribution


def artifact(path, repository, tag):
    return {'name': path.name, 'download_url': f'{repository}/releases/download/{quote(tag, safe="")}/{quote(path.name, safe="")}',
            'sha256': digest(path), 'size': path.stat().st_size}


def check_frontend(frontend, project, config, surface):
    profile = config['frontends'].get(frontend.get('profile'))
    if (not profile or profile['surface'] != surface or frontend.get('version') != project['components']['frontend']['version']
            or frontend.get('packages') != profile['packages']):
        raise ValueError('Frontend metadata does not match the project configuration')


def generate(directory, tag, project, source, config, *, components_only=False):
    if not re.fullmatch('v?' + VERSION_PATTERN, tag):
        raise ValueError('Expected a stable release tag')
    result = compatibility_manifest(project, source)
    repository = project['updates']['repository']
    result.update(schema_version=1, tag=tag, release_url=repository + '/releases/tag/' + tag,
                  artifacts={}, packages=[], frontends={})
    android = result.setdefault('platform', {}).setdefault('android', {})
    android.update(project['components']['android'])
    android.update(target='android-app', core_version=result['version'], artifacts={})
    directory = Path(directory)
    for path in sorted(directory.iterdir()):
        if not path.is_file() or path.name == 'update-manual.json':
            continue
        if path.suffix == '.sha256':
            target = path.with_suffix('')
            expected = digest(target) + '  ' + target.name
            if path.read_text(encoding='utf-8').strip() != expected:
                raise ValueError(f'Checksum mismatch: {path.name}')
            continue
        if components_only and path.suffix not in ('.cphext', '.cphhost', '.cphui'):
            raise ValueError(f'Component update cannot replace {path.name}')
        info = artifact(path, repository, tag)
        desktop = re.fullmatch(r'cph-(windows|linux|darwin)-(amd64|arm64)-(full)\.zip', path.name)
        if desktop:
            platform, profile = '/'.join(desktop.group(1, 2)), desktop[3]
            with zipfile.ZipFile(path) as archive:
                lock = read_distribution(archive, platform, result['version'])
                frontend = lock['frontend']
                check_frontend(frontend, project, config, 'web')
                if frontend['profile'] != config['distributions'][profile]['frontend']:
                    raise ValueError(f'Distribution frontend mismatch: {path.name}')
                info['frontend'] = {k: frontend[k] for k in ('profile', 'version')}
            result['artifacts'].setdefault(platform, {})[profile] = info
        elif path.suffix in ('.cphext', '.cphhost', '.cphui'):
            package = package_metadata(path)
            spec = config['packages'][package['id']]
            component = spec.get('component', package['id'])
            if package['version'] != project['components'][component]['version'] or path.suffix != spec['suffix']:
                raise ValueError(f'Package version mismatch: {path.name}')
            package.pop('path')
            package['display_name'] = package.pop('name')
            result['packages'].append(dict(package, **info))
            if path.suffix == '.cphui':
                with zipfile.ZipFile(path) as archive:
                    frontend = json.loads(archive.read('frontend/profile.json'))
                check_frontend(frontend, project, config, 'app')
                if package['id'] != 'frontend.app' or frontend['profile'] != spec['profile'] or package['target'] != 'client':
                    raise ValueError(f'App UI metadata mismatch: {path.name}')
                result['frontends']['app'] = dict(info, version=package['version'], profile=frontend['profile'])
        elif path.suffix == '.apk':
            with zipfile.ZipFile(path) as archive:
                built = json.loads(archive.read('assets/app-version.json'))
                frontend = built.get('frontend', {})
                profile = config['frontends'].get(frontend.get('profile'), {})
                check_frontend(dict(frontend, packages=profile.get('packages')), project, config, 'app')
                expected = dict(project['components']['android'], core_version=result['version'], frontend=frontend)
                if built != expected or 'AndroidManifest.xml' not in archive.namelist():
                    raise ValueError(f'APK version metadata mismatch: {path.name}')
                abis = [abi for abi in ('arm64-v8a', 'x86_64') if f'lib/{abi}/libcphcore.so' in archive.namelist()]
                if len(abis) != 1 or path.name != apk_name(android, abis[0]):
                    raise ValueError(f'APK ABI or filename mismatch: {path.name}')
            if abis[0] in android['artifacts']:
                raise ValueError(f'Duplicate APK ABI: {abis[0]}')
            android['artifacts'][abis[0]] = info
            android['frontend'] = frontend
        else:
            raise ValueError(f'Unknown release artifact: {path.name}')
    for platform, profiles in result['artifacts'].items():
        if set(profiles) != set(config['distributions']):
            raise ValueError(f'Incomplete desktop distributions for {platform}')
    if components_only and not result['packages']:
        raise ValueError('No component packages found')
    if not components_only and not result['artifacts'] and not android['artifacts']:
        raise ValueError('No desktop distributions or Android APKs found')
    return result


def stable_version(value):
    if not isinstance(value, str) or not re.fullmatch(VERSION_PATTERN, value):
        raise ValueError('Invalid component or core version')
    return tuple(map(int, value.split('.')))


def package_identity(item):
    platforms = item.get('platforms', [])
    if not isinstance(platforms, list) or any(not isinstance(value, str) for value in platforms):
        raise ValueError('Invalid package platforms')
    return item['id'], item['target'], tuple(sorted(platforms))


def validate_index_package(item, repository, tag):
    stable_version(item.get('version'))
    name = item.get('name', '')
    if (not isinstance(name, str) or not re.fullmatch(r'[A-Za-z0-9._-]+\.(?:cphext|cphhost|cphui)', name)
            or item.get('download_url') != f'{repository}/releases/download/{quote(tag, safe="")}/{quote(name, safe="")}'
            or not re.fullmatch(r'[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*', item.get('id', ''))
            or not re.fullmatch(r'[0-9a-f]{64}', item.get('sha256', ''))
            or type(item.get('size')) is not int or not 0 < item['size'] <= 128 << 20
            or item.get('target') not in ('backend', 'client')):
        raise ValueError('Invalid release package metadata')
    package_identity(item)
    validate_environments(item.get('environments'))


def accepts_core(requirement, version):
    current = stable_version(version)
    return (not requirement.get('min') or current >= stable_version(requirement['min'])) and (
        not requirement.get('max_exclusive') or current < stable_version(requirement['max_exclusive']))


def merge_components(previous, generated, repository):
    tag = generated['tag']
    if (previous.get('schema_version') != 1 or previous.get('tag') != tag
            or previous.get('release_url') != repository + '/releases/tag/' + tag):
        raise ValueError('Target Release manifest does not match the repository and tag')
    stable_version(previous.get('version'))
    if not previous.get('artifacts') and not previous.get('platform', {}).get('android', {}).get('artifacts'):
        raise ValueError('Target Release needs a completed core or Android release first')
    existing = {}
    for item in previous.get('packages', []):
        validate_index_package(item, repository, tag)
        identity = package_identity(item)
        if identity in existing:
            raise ValueError('Duplicate component in target Release')
        existing[identity] = item
    for item in generated['packages']:
        validate_index_package(item, repository, tag)
        identity = package_identity(item)
        versions = {previous['version']}
        android = previous.get('platform', {}).get('android', {})
        if android.get('core_version') and (not item['platforms'] or any(p.startswith('android/') for p in item['platforms'])):
            versions.add(android['core_version'])
        if any(not accepts_core(item['core'], version) for version in versions):
            raise ValueError(f'{item["id"]} requires a newer core than the target Release')
        old = existing.get(identity)
        if old:
            comparison = (stable_version(item['version']) > stable_version(old['version'])) - (stable_version(item['version']) < stable_version(old['version']))
            if comparison < 0:
                raise ValueError(f'Component downgrade refused: {item["id"]}')
            if comparison == 0 and item['sha256'] != old['sha256']:
                raise ValueError(f'Published component version is immutable; increase {item["id"]} version')
        for other_identity, other in existing.items():
            if other_identity == identity or other_identity[:2] != identity[:2]:
                continue
            if not other['platforms'] or not item['platforms'] or set(other['platforms']).intersection(item['platforms']):
                raise ValueError(f'Overlapping component platforms: {item["id"]}')
        existing[identity] = item
    result = copy.deepcopy(previous)
    result['packages'] = [existing[key] for key in sorted(existing)]
    result.setdefault('frontends', {}).update(generated['frontends'])
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--assets', required=True)
    parser.add_argument('--out', help='Defaults to <assets>/update-manual.json')
    parser.add_argument('--base', help='Existing target Release manifest; update only component entries')
    args = parser.parse_args()
    project = read_project()
    source = json.loads((ROOT / 'version.json').read_text(encoding='utf-8'))
    result = generate(args.assets, args.tag, project, source, read_config(), components_only=bool(args.base))
    if args.base:
        previous = json.loads(Path(args.base).read_text(encoding='utf-8'))
        result = merge_components(previous, result, project['updates']['repository'])
    output = Path(args.out or Path(args.assets) / 'update-manual.json')
    output.parent.mkdir(parents=True, exist_ok=True)
    temporary = output.with_suffix('.tmp')
    temporary.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    temporary.replace(output)


if __name__ == '__main__':
    main()
