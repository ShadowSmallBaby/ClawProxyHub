"""Shared frontend, desktop Go and Android JNI extension build rules."""
import json
import os
from pathlib import Path
import re
import shutil
import sys

from release_config import ROOT, run, go_inputs, frontend_inputs
from project_config import read_project

PLATFORMS = ('windows/amd64', 'linux/amd64', 'linux/arm64', 'darwin/amd64', 'darwin/arm64')
ANDROID_PLATFORMS = ('android/arm64',)
NDK_VERSION = '28.2.13676358'


def inside(root, value):
    if not isinstance(value, str) or not value:
        raise ValueError('A project-relative path is required')
    path = (root / value).resolve()
    if not path.is_relative_to(root.resolve()):
        raise ValueError(f'Path leaves the project: {value}')
    return path


def step(command, directory):
    if command is None:
        return
    if not isinstance(command, list) or not command or any(not isinstance(v, str) or not v for v in command):
        raise ValueError('Build steps must be non-empty program/argument arrays')
    run(command, cwd=directory)


def copy_frontend(source, destination):
    destination.mkdir(parents=True)
    for path in sorted(source.rglob('*')):
        if path.is_symlink():
            raise ValueError('Frontend output must not contain symbolic links')
        target = destination / path.relative_to(source)
        if path.is_dir():
            target.mkdir(exist_ok=True)
        elif path.is_file():
            shutil.copyfile(path, target)


def tool_environment():
    environment = dict(os.environ, CGO_ENABLED='0', GOWORK='off')
    for key in ('GOOS', 'GOARCH', 'CC', 'CGO_CFLAGS', 'CGO_LDFLAGS'):
        environment.pop(key, None)
    return environment


def load_project(project_file):
    project_file = Path(project_file).resolve()
    project = json.loads(project_file.read_text(encoding='utf-8'))
    if project.get('format') != 1 or set(project) - {'format', 'manifest', 'component', 'frontend', 'backend', 'assets'}:
        raise ValueError('Unsupported extension build definition')
    manifest = json.loads(inside(project_file.parent, project['manifest']).read_text(encoding='utf-8'))
    if project.get('component'):
        if project['component'] != manifest['id']:
            raise ValueError('Component differs from extension identity')
        manifest['version'] = read_project()['components'][project['component']]['version']
    return project_file.parent, project, manifest


def backend_target(root, backend, environment):
    if backend.get('language') != 'go' or backend.get('cgo', False) is not False or set(backend) - {'language', 'module', 'package', 'cgo', 'android'}:
        raise ValueError('Desktop Go backends require CGO disabled')
    module = inside(root, backend['module'])
    target = backend['package']
    if not isinstance(target, str) or (target != '.' and not target.startswith('./')):
        raise ValueError('Go package must be local to its module')
    inside(module, target)
    if not (module / 'go.mod').is_file():
        raise ValueError('Backend module requires a go.mod file')
    result = run(['go', 'list', '-json', target], cwd=module, env=environment, capture_output=True, text=True)
    package = json.loads(result.stdout)
    if package.get('Name') != 'main' or not Path(package['Dir']).resolve().is_relative_to(module):
        raise ValueError('Backend must select a single main package inside its module')
    return module, target


def android_environment(android, architecture, environment):
    if not isinstance(android, dict) or set(android) - {'package', 'min_sdk', 'ndk'}:
        raise ValueError('Unsupported Android backend definition')
    minimum = android.get('min_sdk', 24)
    if type(minimum) is not int or minimum < 24:
        raise ValueError('Android backends require min_sdk >= 24')
    version = android.get('ndk', NDK_VERSION)
    if not isinstance(version, str) or not re.fullmatch(r'\d+\.\d+\.\d+', version):
        raise ValueError('Android NDK version must be explicit')
    sdk = os.environ.get('ANDROID_HOME') or os.environ.get('ANDROID_SDK_ROOT')
    ndk_override = os.environ.get('ANDROID_NDK_HOME')
    if not sdk and not ndk_override:
        raise ValueError('Set ANDROID_HOME or ANDROID_NDK_HOME to build Android extension libraries')
    ndk = Path(ndk_override) if ndk_override else Path(sdk) / 'ndk' / version
    properties = (ndk / 'source.properties').read_text(encoding='utf-8')
    if not re.search(r'^Pkg\.Revision\s*=\s*' + re.escape(version) + r'\s*$', properties, re.M):
        raise ValueError('Android NDK version differs from the build definition')
    host = 'windows-x86_64' if os.name == 'nt' else 'darwin-x86_64' if sys.platform == 'darwin' else 'linux-x86_64'
    compiler = ndk / 'toolchains/llvm/prebuilt' / host / 'bin' / ('clang.exe' if os.name == 'nt' else 'clang')
    if not compiler.is_file():
        raise ValueError(f'Android NDK compiler missing: {compiler}')
    triple = {'arm64': 'aarch64', 'amd64': 'x86_64'}[architecture]
    return dict(environment, GOOS='android', GOARCH=architecture, CGO_ENABLED='1',
                CC=f'"{compiler.resolve().as_posix()}" --target={triple}-linux-android{minimum}',
                CGO_LDFLAGS='-Wl,-z,max-page-size=16384')


def targets(root, backend, android_test_abi=None):
    environment = tool_environment()
    module, target = backend_target(root, backend, environment)
    result = []
    for platform in PLATFORMS:
        system, architecture = platform.split('/')
        result.append((platform, module, target, dict(environment, GOOS=system, GOARCH=architecture),
                       f'backend/{system}-{architecture}/service' + ('.exe' if system == 'windows' else '')))
    android = backend.get('android')
    if android is not None:
        if android_test_abi not in (None, 'x86_64'):
            raise ValueError('Only x86_64 is supported as an additional Android test ABI')
        for platform in ANDROID_PLATFORMS + (('android/amd64',) if android_test_abi else ()):
            architecture = platform.split('/')[1]
            android_env = android_environment(android, architecture, environment)
            _, package = backend_target(root, dict(backend, package=android['package']), android_env)
            result.append((platform, module, package, android_env, f'backend/android-{architecture}/libservice.so'))
    return result


def build_inputs(project_file, android_test_abi=None):
    root, project, manifest = load_project(project_file)
    files = [Path(__file__), Path(project_file), inside(root, project['manifest'])]
    if project.get('frontend'):
        frontend = project['frontend']
        directory = inside(root, frontend['directory'])
        output = inside(directory, frontend['output'])
        files += [path for path in frontend_inputs(directory) if not path.is_relative_to(output)]
        if frontend.get('sdk'):
            files.append(ROOT / 'sdk/extension/bridge.js')
    for asset in project.get('assets', {}).values():
        if isinstance(asset, str):
            files.append(inside(root, asset))
        else:
            asset_source(root, asset)
            files += [(root / value).resolve() for value in asset['inputs']]
    for _, module, target, environment, _ in targets(root, project['backend'], android_test_abi):
        files += go_inputs(module, target, environment)
    generated = {asset_source(root, asset) for asset in project.get('assets', {}).values() if isinstance(asset, dict)}
    files = [path for path in files if path.resolve() not in generated]
    return files, {'definition': project, 'manifest': manifest, 'compiler_host': sys.platform, 'android_test_abi': android_test_abi}


def asset_source(root, asset):
    if isinstance(asset, str):
        return inside(root, asset)
    if (not isinstance(asset, dict) or set(asset) != {'source', 'inputs'}
            or not isinstance(asset['inputs'], list) or not asset['inputs']
            or any(not isinstance(value, str) or not value for value in asset['inputs'])):
        raise ValueError('Generated assets require source and non-empty producer inputs')
    return inside(root, asset['source'])


def build_source(project_file, destination, manifest=None, android_test_abi=None):
    root, project, declared = load_project(project_file)
    manifest = dict(declared if manifest is None else manifest)
    builds = targets(root, project['backend'], android_test_abi)
    destination = Path(destination)
    destination.mkdir(parents=True, exist_ok=True)
    frontend = project.get('frontend')
    if frontend:
        if set(frontend) - {'directory', 'install', 'build', 'output', 'entry', 'sdk'}:
            raise ValueError('Unsupported frontend build fields')
        directory = inside(root, frontend['directory'])
        step(frontend.get('install'), directory)
        step(frontend.get('build'), directory)
        built = inside(directory, frontend['output'])
        entry = inside(built, frontend['entry'])
        if not entry.is_file():
            raise ValueError('Frontend entry is missing after build')
        copy_frontend(built, destination / 'frontend')
        if frontend.get('sdk'):
            bridge = inside(destination / 'frontend', frontend['sdk'])
            if bridge.exists():
                raise ValueError('SDK destination collides with a frontend resource')
            bridge.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / 'sdk/extension/bridge.js', bridge)
        if not manifest.get('pages'):
            manifest['pages'] = [{'id': 'main', 'title': manifest['name'], 'entry': 'frontend/' + entry.relative_to(built).as_posix()}]
    for name, source in project.get('assets', {}).items():
        target = inside(destination, name)
        if name in ('manifest.json', 'signature.json') or target.exists():
            raise ValueError('Asset collides with a generated package file')
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(asset_source(root, source), target)
    entries = {}
    for platform, module, target, environment, entry in builds:
        output = destination / entry
        output.parent.mkdir(parents=True, exist_ok=True)
        command = ['go', 'build', '-trimpath', '-ldflags=-s -w']
        if platform.startswith('android/'):
            command.append('-buildmode=c-shared')
        run(command + ['-o', output, target], cwd=module, env=environment)
        if output.suffix == '.so':
            output.with_suffix('.h').unlink(missing_ok=True)
        entries[platform] = entry
        print(f'Built {platform}', flush=True)
    backend = {'language': 'go', 'protocol': 1, 'entries': entries,
               'background_permissions': manifest.get('backend', {}).get('background_permissions', [])}
    if project['backend'].get('android') is not None:
        backend['android'] = {'protocol': 1, 'min_sdk': project['backend']['android'].get('min_sdk', 24)}
    manifest.update(kind='service', execution='trusted-process', target='backend', activation='hot',
                    platforms=list(entries), backend=backend, files={})
    manifest.pop('entry', None)
    (destination / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    return manifest
