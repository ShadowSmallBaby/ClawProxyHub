"""使用核心验包器验证 Android Lua Host 的清单、发行证书及全部文件摘要。"""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--package', required=True, type=Path)
    parser.add_argument('--certificate-sha256', required=True)
    args = parser.parse_args()
    with zipfile.ZipFile(args.package) as archive:
        manifest = json.loads(archive.read('manifest.json'))
        signature = json.loads(archive.read('signature.json'))
    if manifest.get('id') != 'lua-runtime' or manifest.get('kind') != 'runtime':
        parser.error('Expected the Lua Host runtime package')
    certificate = signature.get('certificate', '')
    digest = hashlib.sha256(base64.b64decode(certificate, validate=True)).hexdigest()
    if digest != args.certificate_sha256:
        parser.error('Lua Host signer differs from the APP signing certificate')
    trust = {'android-app:' + digest: {'certificate': certificate, 'publisher': 'Android APP',
             'ids': ['lua-runtime'], 'permissions': ['runtime.execute'], 'native': True}}
    with tempfile.TemporaryDirectory(prefix='cph-host-verify-') as directory:
        path = Path(directory) / 'trust.json'
        path.write_text(json.dumps(trust), encoding='utf-8')
        subprocess.run(['go', 'run', './cmd/cphext', 'verify', '--package', str(args.package.resolve()), '--trust', str(path)],
                       cwd=Path(__file__).resolve().parents[1], env=dict(os.environ, GOWORK='off'), check=True, stdout=subprocess.DEVNULL)
    print('Lua Host package verified')


if __name__ == '__main__':
    main()
