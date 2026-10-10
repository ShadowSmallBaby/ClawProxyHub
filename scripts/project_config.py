"""Read component versions and update sources; synchronize the legacy changelog document."""
import argparse
import copy
import json
import os
from pathlib import Path
import re
import tomllib
from urllib.parse import urlsplit

ROOT = Path(__file__).resolve().parents[1]
PROJECT_FILE = ROOT / 'project.toml'
VERSION_PATTERN = r'(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)'


def repository_url(value):
    value = value.rstrip('/')
    parsed = urlsplit(value)
    segments = parsed.path.split('/')[1:]
    if (parsed.scheme != 'https' or not parsed.hostname or parsed.username or parsed.password
            or parsed.query or parsed.fragment or len(segments) != 2
            or any(not re.fullmatch(r'[A-Za-z0-9_.-]+', p) or p in ('.', '..') for p in segments)):
        raise ValueError('Update repository must be an HTTPS GitHub repository URL')
    return value.removesuffix('.git')


def read_project(path=PROJECT_FILE, *, environment=True):
    with Path(path).open('rb') as stream:
        project = tomllib.load(stream)
    if project.get('format') != 1:
        raise ValueError('Unsupported project configuration format')
    components = project['components']
    for name in ('core', 'android', 'frontend', 'lua-runtime', 'lua-editor'):
        if name not in components:
            raise ValueError(f'Missing component version: {name}')
    for name, component in components.items():
        if not re.fullmatch(VERSION_PATTERN, component['version']):
            raise ValueError(f'Invalid {name} version')
    android = components['android']
    if (type(android['version_code']) is not int or not 1 <= android['version_code'] <= 2100000000
            or type(android['min_sdk']) is not int or android['min_sdk'] < 24
            or type(android['target_sdk']) is not int or android['target_sdk'] < android['min_sdk']
            or not re.fullmatch(r'[A-Za-z][A-Za-z0-9_]*(?:\.[A-Za-z][A-Za-z0-9_]*)+', android['application_id'])):
        raise ValueError('Invalid Android application configuration')
    override = os.environ.get('CPH_REPOSITORY_URL') if environment else None
    project['updates']['repository'] = repository_url(override or project['updates']['repository'])
    return project


def package_manifest(identifier, project=None, build=None):
    project = project or read_project()
    build = build or json.loads((ROOT / 'build-config.json').read_text(encoding='utf-8'))
    manifest = json.loads((ROOT / build['packages'][identifier]['manifest']).read_text(encoding='utf-8'))
    if manifest['id'] != identifier:
        raise ValueError(f'Package identity mismatch: {identifier}')
    component = build['packages'][identifier].get('component', identifier)
    manifest['version'] = project['components'][component]['version']
    return manifest


def compatibility_manifest(project, source):
    # 保留原始日志及未知字段，只同步由项目配置管理的元数据。
    result = copy.deepcopy(source)
    result.update(version=project['components']['core']['version'],
                  release_url=project['updates']['repository'] + '/releases/latest')
    return result


def apk_name(android, abi):
    if abi not in ('arm64-v8a', 'x86_64'):
        raise ValueError(f'Unsupported Android ABI: {abi}')
    return f"ClawProxyHub-{android['version']}-{android['version_code']}-android-{abi}.apk"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['export', 'get', 'manifest', 'sync', 'check', 'apk-name'])
    parser.add_argument('value', nargs='?')
    parser.add_argument('--project', default=str(PROJECT_FILE))
    parser.add_argument('--version-file', default=str(ROOT / 'version.json'))
    args = parser.parse_args()
    project = read_project(args.project, environment=args.command not in ('sync', 'check'))
    if args.command == 'export':
        print(json.dumps(project))
    elif args.command == 'get':
        value = project
        for part in (args.value or '').split('.'):
            value = value[part]
        print(value if isinstance(value, (str, int)) else json.dumps(value))
    elif args.command == 'manifest':
        print(json.dumps(package_manifest(args.value, project), ensure_ascii=False, indent=2))
    elif args.command == 'apk-name':
        print(apk_name(project['components']['android'], args.value))
    else:
        path = Path(args.version_file)
        source = json.loads(path.read_text(encoding='utf-8'))
        expected = compatibility_manifest(project, source)
        if args.command == 'check':
            if source != expected:
                parser.error('version.json metadata is stale; run python scripts/project_config.py sync')
        elif source != expected:
            path.write_text(json.dumps(expected, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')


if __name__ == '__main__':
    main()
