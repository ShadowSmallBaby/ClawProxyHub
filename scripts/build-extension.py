"""Build a frontend and desktop/Android Go backends into one signed .cphext."""
import argparse
import base64
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

from release_config import ROOT, run, package_metadata
from extension_build import inside, step, copy_frontend, backend_target, load_project, build_source, tool_environment


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--project', required=True, help='Extension build.json')
    parser.add_argument('--out', required=True, help='Output directory')
    parser.add_argument('--key', help='Ed25519 private key file')
    parser.add_argument('--key-id', default=os.environ.get('CPH_EXTENSION_KEY_ID'))
    parser.add_argument('--trust', help='Host trust JSON file')
    parser.add_argument('--development-key', action='store_true')
    parser.add_argument('--android-test-abi', choices=['x86_64'], help='Include an additional Android library for local emulator tests')
    args = parser.parse_args()
    project_file = Path(args.project).resolve()
    _, _, manifest = load_project(project_file)
    output = Path(args.out).resolve()
    output.mkdir(parents=True, exist_ok=True)
    if args.development_key and (args.key or args.trust):
        raise ValueError('Development signing cannot be combined with explicit keys or trust')
    with tempfile.TemporaryDirectory(prefix='cph-extension-build-') as temporary:
        stage = Path(temporary)
        tool = stage / ('cphext.exe' if os.name == 'nt' else 'cphext')
        run(['go', 'build', '-trimpath', '-o', tool, './cmd/cphext'], cwd=ROOT, env=tool_environment())
        if args.development_key:
            key_file = ROOT / '.cache/development-signing/extension.key'
            key_file.parent.mkdir(parents=True, exist_ok=True)
            if not key_file.exists():
                run([tool, 'keygen', '--key', key_file], stdout=subprocess.DEVNULL)
            args.key_id = 'local-development'
            public = base64.b64encode(base64.b64decode(key_file.read_text().strip())[-32:]).decode()
            trust = {args.key_id: {'public_key': public, 'publisher': 'Local development', 'ids': [manifest['id']], 'permissions': manifest['permissions'], 'native': True}}
        else:
            if not args.key_id:
                raise ValueError('--key-id or CPH_EXTENSION_KEY_ID is required')
            key_file = Path(args.key).resolve() if args.key else stage / 'signing.key'
            if not args.key:
                key_file.touch(mode=0o600)
                key_file.write_text(os.environ['CPH_EXTENSION_PRIVATE_KEY'], encoding='utf-8')
            trust = json.loads(Path(args.trust).read_text(encoding='utf-8') if args.trust else os.environ['CPH_EXTENSION_TRUST_JSON'])
        source = stage / 'package'
        manifest = build_source(project_file, source, manifest, args.android_test_abi)
        trust_file = stage / 'trust.json'
        trust_file.write_text(json.dumps(trust, indent=2), encoding='utf-8')
        package = stage / 'output.cphext'
        run([tool, 'pack', '--dir', source, '--key', key_file, '--key-id', args.key_id, '--out', package])
        run([tool, 'verify', '--package', package, '--trust', trust_file], stdout=subprocess.DEVNULL)
        destination = output / f"{manifest['id']}-{manifest['version']}.cphext"
        shutil.copyfile(package, destination)
        (output / 'trust.json').write_text(json.dumps(trust, indent=2) + '\n', encoding='utf-8')
        (output / 'index.json').write_text(json.dumps({'format': 1, 'packages': [package_metadata(destination)]}, indent=2) + '\n', encoding='utf-8')
        print(f'Packaged {destination}', flush=True)


if __name__ == '__main__':
    main()
