"""Build the core with embedded Web and assemble the full desktop distribution."""
import argparse
import os
from pathlib import Path
import sys

from release_config import ROOT, DEFAULT_CONFIG, read_config, run, distribution_presets, frontend_output
from project_config import read_project


def main():
    project = read_project()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--platform', required=True)
    parser.add_argument('--out', required=True)
    parser.add_argument('--work', default=str(ROOT / 'build/release'))
    parser.add_argument('--config', default=str(DEFAULT_CONFIG))
    parser.add_argument('--frontend')
    parser.add_argument('--distribution', action='append', choices=['full'], help='Full desktop distribution')
    parser.add_argument('--prebuilt-web', action='store_true', help='Use the configured, already built frontend directory')
    parser.add_argument('--prebuilt', action='append', default=[], help='Package ID already built by another job at the same source revision')
    parser.add_argument('--packages', default=str(ROOT / 'build/packages'))
    parser.add_argument('--development-key', action='store_true')
    parser.add_argument('--key')
    parser.add_argument('--key-id')
    parser.add_argument('--trust')
    args = parser.parse_args()
    version = project['components']['core']['version']
    config = read_config(args.config)
    presets = distribution_presets(config, args.distribution, args.frontend)
    if not args.prebuilt_web:
        run(['pnpm', 'install', '--frozen-lockfile'], cwd=ROOT / 'web')
        run(['pnpm', 'exec', 'vue-tsc', '--noEmit'], cwd=ROOT / 'web')
        for frontend in sorted({preset['frontend'] for preset in presets.values()}):
            run(['pnpm', 'exec', 'vite', 'build', '--mode', frontend], cwd=ROOT / 'web')
    packages = sorted({identifier for preset in presets.values() for identifier in preset['packages']})
    web = frontend_output(presets['full']['frontend'], config, project)
    if web.resolve() != (ROOT / 'web/build-web').resolve():
        raise ValueError('The release frontend must build into web/build-web for go:embed')
    for identifier in args.prebuilt:
        if identifier not in packages:
            parser.error(f'Unknown prebuilt package: {identifier}')
    selected = [p for p in packages if p not in args.prebuilt]
    if selected:
        command = [sys.executable, ROOT / 'scripts/build-packages.py', '--config', Path(args.config).resolve(), '--platform', args.platform, '--out', Path(args.packages).resolve()]
        for identifier in selected:
            command += ['--only', identifier]
        for name in ('key', 'key_id', 'trust'):
            if getattr(args, name):
                command += ['--' + name.replace('_', '-'), getattr(args, name)]
        if args.development_key:
            command += ['--development-key']
        run(command)
    target_os, target_arch = args.platform.split('/')
    work = Path(args.work).resolve() / f'{target_os}-{target_arch}'
    work.mkdir(parents=True, exist_ok=True)
    target = dict(os.environ, GOOS=target_os, GOARCH=target_arch, CGO_ENABLED='1', GOWORK='off')
    core = work / ('core.exe' if target_os == 'windows' else 'core')
    flags = '-s -w -X github.com/ShadowSmallBaby/ClawProxyHub/internal/version.UpdateRepository=' + project['updates']['repository']
    run(['go', 'build', '-trimpath', '-ldflags=' + flags, '-o', core, './cmd/cph'], cwd=ROOT, env=target)
    control = work / ('cli.exe' if target_os == 'windows' else 'cli')
    run(['go', 'build', '-trimpath', '-ldflags=-s -w', '-o', control, './cmd/cphctl'], cwd=ROOT, env=target)
    host = dict(os.environ)
    for name in ('GOOS', 'GOARCH', 'CC', 'CGO_ENABLED'):
        host.pop(name, None)
    assembler = work / ('cph-assemble.exe' if os.name == 'nt' else 'cph-assemble')
    run(['go', 'build', '-trimpath', '-o', assembler, './cmd/cph-assemble'], cwd=ROOT, env=host)
    command = [sys.executable, ROOT / 'scripts/assemble-release.py', '--config', Path(args.config).resolve(), '--core', core, '--cli', control, '--assembler', assembler, '--version', version, '--platform', args.platform, '--packages', Path(args.packages).resolve(), '--out', Path(args.out).resolve()]
    if args.frontend:
        command += ['--frontend', args.frontend]
    if args.trust:
        command += ['--trust', Path(args.trust).resolve()]
    for name in presets:
        command += ['--distribution', name]
    run(command)


if __name__ == '__main__':
    main()
