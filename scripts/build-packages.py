"""Build signed components independently of the core and reuse verified cached packages."""
import argparse
import base64
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

from release_config import ROOT, DEFAULT_CONFIG, read_config, run, content_key, go_inputs, frontend_inputs, frontend_output, package_metadata, cached_package, cache_package
from project_config import read_project, package_manifest
from extension_build import build_inputs as extension_inputs, build_source as extension_source


def build_app_frontend(args, config, project, output):
    spec = config['packages']['frontend.app']
    if args.cache_key:
        raise ValueError('App UI packages use the Gradle build cache; run :app:packageAppFrontend --build-cache')
    if not args.prebuilt_web:
        run(['pnpm', 'install', '--frozen-lockfile'], cwd=ROOT / 'web')
        run(['pnpm', 'exec', 'vue-tsc', '--noEmit'], cwd=ROOT / 'web')
        run(['pnpm', 'exec', 'vite', 'build', '--mode', spec['profile']], cwd=ROOT / 'web')
    frontend_output(spec['profile'], config, project)
    gradle = [ROOT / 'android/gradlew.bat'] if os.name == 'nt' else ['sh', ROOT / 'android/gradlew']
    abi = {'android/arm64': 'arm64-v8a', 'android/amd64': 'x86_64'}.get(args.platform, 'arm64-v8a')
    run(gradle + ['-p', ROOT / 'android', '--build-cache', '-PcphABI=' + abi,
                 '-PcphReleaseSigning=' + str(not args.development_key).lower(),
                 '-PcphBuildConfig=' + str(Path(args.config).resolve()), ':app:packageAppFrontend'])
    name = spec['filename'].format(version=project['components']['frontend']['version'])
    destination = output / name
    shutil.copyfile(ROOT / 'build/android/frontend-packages' / name, destination)
    return package_metadata(destination)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config', default=str(DEFAULT_CONFIG))
    parser.add_argument('--platform', required=True, help='Go OS/architecture, for example linux/amd64')
    parser.add_argument('--out', required=True)
    parser.add_argument('--only', action='append', help='Package ID; repeat to build several')
    parser.add_argument('--frontend', help='Build only the optional packages configured for this frontend')
    parser.add_argument('--prebuilt-web', action='store_true', help='Package the already built App frontend')
    parser.add_argument('--cache', default=str(ROOT / '.cache/cph-packages'))
    parser.add_argument('--cache-key', action='store_true', help='Print the combined component input key without building packages')
    parser.add_argument('--key', help='Ed25519 private key file; otherwise use CPH_EXTENSION_PRIVATE_KEY')
    parser.add_argument('--key-id', default=os.environ.get('CPH_EXTENSION_KEY_ID'))
    parser.add_argument('--trust', help='Trust JSON file; otherwise use CPH_EXTENSION_TRUST_JSON')
    parser.add_argument('--cphext', help='Existing package tool; otherwise build it locally')
    parser.add_argument('--development-key', action='store_true', help='Use a local signing identity for development builds only')
    parser.add_argument('--android-test-abi', choices=['x86_64'], help='Include an additional Android library for local emulator tests')
    args = parser.parse_args()
    config = read_config(args.config)
    project = read_project()
    if args.frontend and (args.only or args.frontend not in config['frontends']):
        parser.error('--frontend must select one configured frontend and cannot be combined with --only')
    selected = config['frontends'][args.frontend]['packages'] if args.frontend else (args.only or [
        identifier for identifier, spec in config['packages'].items() if spec['builder'] != 'app'])
    for identifier in selected:
        if identifier not in config['packages']:
            parser.error(f'Unknown package: {identifier}')
    if not selected:
        print(content_key([], {}) if args.cache_key else f'{args.frontend}: no optional packages configured')
        return
    if args.development_key and (args.key or args.trust):
        parser.error('--development-key cannot be combined with --key or --trust')
    target_os, target_arch = args.platform.split('/')
    if target_os == 'android' and any(config['packages'][p]['builder'] == 'go' for p in selected):
        parser.error('Android native runtime packages use android/gradlew :app:packageLuaHost')
    output = Path(args.out).resolve()
    output.mkdir(parents=True, exist_ok=True)
    index = []
    if 'frontend.app' in selected:
        index.append(build_app_frontend(args, config, project, output))
        selected = [identifier for identifier in selected if identifier != 'frontend.app']
        if not selected:
            (output / 'index.json').write_text(json.dumps({'format': 1, 'packages': index}, indent=2) + '\n', encoding='utf-8')
            return
    if not args.key_id and not args.development_key:
        parser.error('--key-id or CPH_EXTENSION_KEY_ID is required for extensions and desktop runtimes')
    tool_env = dict(os.environ)
    for name in ('GOOS', 'GOARCH', 'CC', 'CGO_ENABLED'):
        tool_env.pop(name, None)
    target_env = dict(tool_env, GOOS=target_os, GOARCH=target_arch, CGO_ENABLED='0', GOWORK='off')
    with tempfile.TemporaryDirectory(prefix='cph-packages-') as temporary:
        stage = Path(temporary)
        tool = Path(args.cphext).resolve() if args.cphext else stage / ('cphext.exe' if os.name == 'nt' else 'cphext')
        if not args.cphext:
            run(['go', 'build', '-trimpath', '-o', tool, './cmd/cphext'], cwd=ROOT, env=tool_env)
        if args.development_key:
            args.key_id = 'local-development'
            key = ROOT / '.cache/development-signing/extension.key'
            key.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
            if not key.exists():
                run([tool, 'keygen', '--key', key], capture_output=True, text=True)
            public = base64.b64encode(base64.b64decode(key.read_text(encoding='utf-8').strip())[-32:]).decode()
            identities = [identifier for identifier, spec in config['packages'].items() if spec['builder'] != 'app']
            manifests = [package_manifest(identifier, project, config) for identifier in identities]
            trust = {args.key_id: {'public_key': public, 'publisher': 'Local development', 'ids': sorted(identities),
                                  'permissions': sorted({p for m in manifests for p in m['permissions']}), 'native': any(m['kind'] in ('runtime', 'service') for m in manifests)}}
        else:
            trust = json.loads(Path(args.trust).read_text(encoding='utf-8') if args.trust else os.environ['CPH_EXTENSION_TRUST_JSON'])
            key = Path(args.key).resolve() if args.key else stage / 'signing.key'
            if not args.key:
                key.touch(mode=0o600)
                key.write_text(os.environ['CPH_EXTENSION_PRIVATE_KEY'], encoding='utf-8')
        if args.key_id not in trust:
            parser.error('Signing identity is absent from the trust store')
        trust_file = stage / 'trust.json'
        trust_file.write_text(json.dumps(trust), encoding='utf-8')
        packaging_files = [ROOT / 'scripts/build-packages.py', ROOT / 'scripts/release_config.py', ROOT / 'scripts/project_config.py']
        packaging_files += go_inputs('.', './cmd/cphext', dict(tool_env, GOWORK='off'))
        packaging_files += [ROOT / 'LICENSE']
        toolchain = run(['go', 'version'], capture_output=True, text=True).stdout.strip()
        fingerprints = {}
        for identifier in selected:
            spec = config['packages'][identifier]
            manifest_path = ROOT / spec['manifest']
            manifest = package_manifest(identifier, project, config)
            version = manifest['version']
            properties = {'package': spec, 'manifest': manifest, 'signer': args.key_id, 'trust': trust[args.key_id], 'go': toolchain}
            files = packaging_files + [manifest_path] + [ROOT / path for path in spec.get('inputs', [])]
            if spec['builder'] == 'go':
                files += go_inputs(spec['module'], spec['target'], target_env)
                properties.update(platform=args.platform, flags=['-trimpath', '-ldflags=-s -w', 'CGO_ENABLED=0'])
                name = spec.get('filename', '{id}-{version}-{os}-{arch}{suffix}').format(
                    id=identifier, version=version, os=target_os, arch=target_arch, suffix=spec['suffix'])
            elif spec['builder'] == 'frontend':
                files += frontend_inputs(spec['directory'])
                properties.update(node=run(['node', '--version'], capture_output=True, text=True).stdout.strip(), pnpm=run(['pnpm', '--version'], capture_output=True, text=True).stdout.strip())
                name = f"{identifier}-{version}{spec['suffix']}"
            elif spec['builder'] == 'extension':
                sources, definition = extension_inputs(ROOT / spec['project'], args.android_test_abi)
                files += sources
                properties.update(definition=definition, node=run(['node', '--version'], capture_output=True, text=True).stdout.strip(),
                                  pnpm=run(['pnpm', '--version'], capture_output=True, text=True).stdout.strip())
                name = f"{identifier}-{version}{spec['suffix']}"
            else:
                raise ValueError(f'Unsupported package builder: {spec["builder"]}')
            fingerprint = content_key(files, properties)
            fingerprints[identifier] = fingerprint
            if args.cache_key:
                continue
            destination = output / name
            cached = cached_package(args.cache, fingerprint, name)
            if cached:
                try:
                    run([tool, 'verify', '--package', cached, '--trust', trust_file], stdout=subprocess.DEVNULL)
                    metadata = package_metadata(cached)
                    if metadata['id'] != identifier or metadata['version'] != version:
                        raise ValueError('Cached package identity mismatch')
                except Exception:
                    cached = None
            if cached:
                shutil.copyfile(cached, destination)
                print(f'Reused {name}')
            else:
                source = stage / identifier
                source.mkdir()
                if spec['builder'] == 'go':
                    entry = spec['entry'] + ('.exe' if target_os == 'windows' else '')
                    run(['go', 'build', '-trimpath', '-ldflags=-s -w', '-o', source / entry, spec['target']], cwd=ROOT / spec['module'], env=target_env)
                    manifest.update(execution='trusted-process', entry=entry, platforms=[args.platform])
                    (source / 'manifest.json').write_text(json.dumps(manifest), encoding='utf-8')
                elif spec['builder'] == 'frontend':
                    directory = ROOT / spec['directory']
                    run(['pnpm', 'install', '--frozen-lockfile'], cwd=directory)
                    run(['pnpm', 'run', 'build'], cwd=directory, env=dict(os.environ, CPH_PYTHON=sys.executable))
                    shutil.copytree(directory / spec['output'], source, dirs_exist_ok=True)
                    (source / 'manifest.json').write_text(json.dumps(manifest), encoding='utf-8')
                elif spec['builder'] == 'extension':
                    extension_source(ROOT / spec['project'], source, manifest, args.android_test_abi)
                package = stage / name
                run([tool, 'pack', '--dir', source, '--key', key, '--key-id', args.key_id, '--out', package])
                run([tool, 'verify', '--package', package, '--trust', trust_file], stdout=subprocess.DEVNULL)
                shutil.copyfile(package, destination)
                cache_package(args.cache, fingerprint, package)
                print(f'Built {name}')
            index.append(package_metadata(destination))
        if args.cache_key:
            print(content_key([], fingerprints))
            return
        (output / 'index.json').write_text(json.dumps({'format': 1, 'packages': index}, indent=2) + '\n', encoding='utf-8')
        (output / 'trust.json').write_text(json.dumps(trust, indent=2) + '\n', encoding='utf-8')


if __name__ == '__main__':
    main()
