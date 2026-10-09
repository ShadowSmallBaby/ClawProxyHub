"""Shared release configuration, component inputs and verified artifact cache."""
import hashlib
import copy
import json
import os
from pathlib import Path
import shutil
import subprocess
import zipfile

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_CONFIG = ROOT / 'build-config.json'


def read_config(path=DEFAULT_CONFIG):
    config = json.loads(Path(path).read_text(encoding='utf-8'))
    if config.get('format') != 1 or set(config['distributions']) != {'full'}:
        raise ValueError('Expected full release configuration format 1')
    for name, profile in config['frontends'].items():
        if name not in ('web-full', 'app-full') or profile['surface'] != name.split('-')[0]:
            raise ValueError(f'Invalid frontend profile: {name}')
        for package in profile['packages']:
            if package not in config['packages']:
                raise ValueError(f'Unknown package: {package}')
    distribution_presets(config)
    android = config['android']
    if config['frontends'][android['frontend']]['surface'] != 'app':
        raise ValueError('Android requires an App frontend')
    if any(identifier not in config['packages'] for identifier in android['packages']):
        raise ValueError('Unknown Android package')
    return config


def distribution_presets(config, selected=None, frontend=None):
    presets = copy.deepcopy(config['distributions'])
    if frontend:
        presets['full']['frontend'] = frontend
    result = {}
    for name in selected or presets:
        if name not in presets:
            raise ValueError(f'Unknown distribution: {name}')
        preset = presets[name]
        ui = config['frontends'].get(preset['frontend'])
        if not ui or ui['surface'] != 'web':
            raise ValueError(f'{name} requires a Web frontend')
        preset['packages'] = list(dict.fromkeys(preset['packages'] + ui['packages']))
        if any(identifier not in config['packages'] for identifier in preset['packages']):
            raise ValueError(f'{name} contains an unknown package')
        result[name] = preset
    return result


def frontend_output(name, config, project):
    preset = config['frontends'][name]
    source = ROOT / preset['out']
    report = json.loads((source / 'profile.json').read_text(encoding='utf-8'))
    if (not (source / 'index.html').is_file() or report['profile'] != name or report['packages'] != preset['packages']
            or report['version'] != project['components']['frontend']['version']):
        raise ValueError(f'{name}: rebuild the configured frontend')
    return source


def run(command, **kwargs):
    command = list(map(str, command))
    if os.name == 'nt' and command[0] in ('pnpm', 'npm'):
        command[0] = shutil.which(command[0] + '.cmd') or command[0]
    if kwargs.get('text'):
        kwargs.setdefault('encoding', 'utf-8')
    return subprocess.run(command, check=True, **kwargs)


def digest(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def content_key(files, properties, root=ROOT):
    value = hashlib.sha256(json.dumps(properties, sort_keys=True).encode())
    root = Path(root).resolve()
    for path in sorted(set(Path(p).resolve() for p in files)):
        name = path.relative_to(root).as_posix() if path.is_relative_to(root) else path.as_posix()
        value.update(name.encode())
        value.update(b'\0')
        value.update(bytes.fromhex(digest(path)))
    return value.hexdigest()


def json_stream(value):
    decoder = json.JSONDecoder()
    while value.strip():
        value = value.lstrip()
        item, offset = decoder.raw_decode(value)
        yield item
        value = value[offset:]


def go_inputs(module, target, env, tags=''):
    output = run(['go', 'list', '-deps', '-json', '-tags=' + tags, target], cwd=ROOT / module, env=env, capture_output=True, text=True).stdout
    files = [ROOT / 'go.mod', ROOT / 'go.sum', ROOT / module / 'go.mod', ROOT / module / 'go.sum']
    for package in json_stream(output):
        directory = Path(package['Dir']).resolve()
        if not directory.is_relative_to(ROOT):
            continue
        dependency = package.get('Module', {}).get('Dir')
        if dependency and Path(dependency).is_relative_to(ROOT):
            files.extend(p for p in (Path(dependency) / 'go.mod', Path(dependency) / 'go.sum') if p.is_file())
        for field in ('GoFiles', 'CgoFiles', 'CFiles', 'CXXFiles', 'HFiles', 'MFiles', 'FFiles', 'SFiles', 'SwigFiles', 'SwigCXXFiles', 'EmbedFiles', 'SysoFiles'):
            files.extend(directory / name for name in package.get(field, []))
    return sorted(set(files))


def frontend_inputs(directory):
    base = ROOT / directory
    excluded = {'node_modules', 'dist', 'package', 'test-results', 'playwright-report'}
    return [p for p in base.rglob('*') if p.is_file() and p.suffix != '.md'
            and not any(part in excluded or part.startswith(('.', 'dist-')) for part in p.relative_to(base).parts)]


def validate_environments(values):
    if values is None:
        return
    if (not isinstance(values, list) or not values
            or any(not isinstance(value, str) or value not in ('app', 'web-desktop', 'web-mobile') for value in values)
            or len(set(values)) != len(values)):
        raise ValueError('Invalid extension environments')


def package_metadata(path):
    path = Path(path)
    with zipfile.ZipFile(path) as archive:
        manifest = json.loads(archive.read('manifest.json'))
    validate_environments(manifest.get('environments'))
    result = {
        'id': manifest['id'], 'version': manifest['version'], 'name': manifest['name'],
        'label': manifest.get('label', {}), 'desc': manifest.get('desc', {}),
        'kind': manifest['kind'], 'path': path.name, 'sha256': digest(path), 'size': path.stat().st_size,
        'core': manifest['core'], 'platforms': manifest.get('platforms', []),
        'target': manifest['target'],
        'permissions': manifest['permissions'], 'dependencies': manifest.get('dependencies', {}),
    }
    if manifest.get('environments') is not None:
        result['environments'] = manifest['environments']
    return result


def cached_package(directory, key, name):
    path = Path(directory) / key / name
    metadata = path.with_suffix(path.suffix + '.json')
    if not path.is_file() or not metadata.is_file():
        return None
    try:
        expected = json.loads(metadata.read_text(encoding='utf-8'))
        if expected == {'input': key, 'sha256': digest(path)}:
            return path
    except (OSError, ValueError):
        pass
    return None


def cache_package(directory, key, path):
    destination = Path(directory) / key / Path(path).name
    destination.parent.mkdir(parents=True, exist_ok=True)
    temporary = destination.with_suffix(destination.suffix + '.tmp')
    shutil.copyfile(path, temporary)
    temporary.replace(destination)
    destination.with_suffix(destination.suffix + '.json').write_text(json.dumps({'input': key, 'sha256': digest(path)}), encoding='utf-8')


if __name__ == '__main__':
    import argparse
    parser = argparse.ArgumentParser(description='List actual local Go build inputs for Gradle caching')
    parser.add_argument('--module', required=True)
    parser.add_argument('--target', required=True)
    parser.add_argument('--tags', default='')
    args = parser.parse_args()
    print(json.dumps([str(p) for p in go_inputs(args.module, args.target, dict(os.environ), args.tags)]))
