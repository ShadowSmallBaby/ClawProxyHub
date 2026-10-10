"""在明确指定的测试设备导入原生插件包，验证主应用私有进程加载与握手。"""
import argparse
import json
from pathlib import Path
import re
import subprocess
import sys
import zipfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('packages', type=Path)
    parser.add_argument('--adb', required=True)
    parser.add_argument('--serial', required=True)
    parser.add_argument('--abi', required=True, choices=['arm64-v8a', 'x86_64'])
    parser.add_argument('--output', type=Path)
    args = parser.parse_args()
    host = 'github.shadowbaby.clawproxyhub.core'

    def adb(*command):
        result = subprocess.run([args.adb, '-s', args.serial, *command], capture_output=True, text=True, encoding='utf-8', timeout=120)
        if result.returncode:
            raise RuntimeError(result.stdout + result.stderr)
        return result.stdout

    reports = []
    try:
        for path in sorted(args.packages.glob('*-android-*.cphplugin')):
            with zipfile.ZipFile(path) as package:
                manifest = json.loads(package.read('manifest.json'))
            if manifest['android']['abi'] != args.abi:
                continue
            name = manifest['name']
            if manifest['android']['format'] != 'cph-native-v1' or not re.fullmatch(r'[a-z][a-z0-9_]*', name):
                raise ValueError('invalid native plugin identity')
            adb('push', str(path), '/data/local/tmp/cph-native-plugin.cphplugin')
            adb('shell', 'run-as', host, 'cp', '/data/local/tmp/cph-native-plugin.cphplugin', 'cache/plugin.cphplugin')
            output = adb('shell', 'am', 'instrument', '-w', '-e', 'native', 'install', '-e', 'plugin', name, f'{host}.test/{host}.CoreInstrumentation')
            report = next((json.loads(line.partition('=')[2]) for line in output.splitlines() if line.startswith('INSTRUMENTATION_RESULT: report=')), None)
            if not report or not report.get('ok') or (report['plugin'], report['version'], report['protocol']) != (name, manifest['version'], manifest['protocol_version']):
                raise RuntimeError(f'{name}: {output}')
            reports.append(report)
            print(f"PASS {name} {report['version']}", flush=True)
    finally:
        for command in [('shell', 'rm', '-f', '/data/local/tmp/cph-native-plugin.cphplugin'),
                        ('shell', 'run-as', host, 'rm', '-f', 'cache/plugin.cphplugin')]:
            try:
                adb(*command)
            except Exception as error:
                print(f'Cleanup could not finish: {error}', file=sys.stderr)
        if args.output:
            args.output.write_text(json.dumps(reports, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    if not reports:
        parser.error('no packages matched the requested ABI')
    print(f'Verified {len(reports)} plugins on {args.serial} ({args.abi}).')


if __name__ == '__main__':
    main()
