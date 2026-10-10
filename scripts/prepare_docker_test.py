import hashlib
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
import zipfile

from project_config import ROOT, read_project
from release_config import digest

spec = importlib.util.spec_from_file_location('prepare_docker', ROOT / 'scripts/prepare-docker.py')
docker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(docker)


class DockerContextTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.assets = self.root / 'assets'
        self.assets.mkdir()
        self.output = self.root / 'context'
        self.project = read_project(environment=False)
        self.manifest = {'schema_version': 1, 'version': self.project['components']['core']['version'], 'artifacts': {}}
        for arch in ('amd64', 'arm64'):
            self.archive(arch)

    def archive(self, arch, platform=None, extra=None):
        files = {'cph': ('core-with-web-' + arch).encode(), 'cli': b'cli',
                 'data/packages/lua-runtime.cphhost': b'signed runtime', 'data/packages/lua-editor.cphext': b'signed editor'}
        files.update(extra or {})
        lock = {'format': 1, 'profile': 'full', 'platform': platform or 'linux/' + arch,
                'version': self.manifest['version'], 'core': 'cph', 'cli': 'cli', 'frontend': {'profile': 'web-full'},
                'packages': [{'path': name, 'sha256': hashlib.sha256(data).hexdigest()} for name, data in files.items() if name.startswith('data/packages/')],
                'files': {name: hashlib.sha256(data).hexdigest() for name, data in files.items() if not name.startswith('data/packages/')}}
        lock['core_sha256'] = lock['files']['cph']
        files['distribution.lock.json'] = json.dumps(lock).encode()
        path = self.assets / f'cph-linux-{arch}-full.zip'
        with zipfile.ZipFile(path, 'w') as archive:
            for name, data in files.items():
                archive.writestr('ClawProxyHub/' + name, data)
        self.manifest['artifacts']['linux/' + arch] = {'full': {'name': path.name, 'size': path.stat().st_size, 'sha256': digest(path)}}

    def prepare(self):
        (self.assets / 'update-manual.json').write_text(json.dumps(self.manifest), encoding='utf-8')
        docker.prepare(self.assets, self.output, self.project)

    def test_preserves_release_payload_and_executable_permissions(self):
        self.prepare()
        for arch in ('amd64', 'arm64'):
            root = self.output / arch
            self.assertEqual((root / 'cph').read_bytes(), ('core-with-web-' + arch).encode())
            self.assertEqual((root / 'data/packages/lua-runtime.cphhost').read_bytes(), b'signed runtime')
            self.assertEqual((root / 'data/packages/lua-editor.cphext').read_bytes(), b'signed editor')
            self.assertEqual((root / 'cli').read_bytes(), b'cli')
            self.assertFalse((root / 'web').exists())
            if os.name != 'nt':
                self.assertEqual((root / 'cph').stat().st_mode & 0o777, 0o755)
                self.assertEqual((root / 'cli').stat().st_mode & 0o777, 0o755)
        with self.assertRaisesRegex(ValueError, 'must not exist'):
            self.prepare()

    def test_rejects_tampered_artifact_without_leaving_a_partial_context(self):
        with (self.assets / 'cph-linux-arm64-full.zip').open('ab') as archive:
            archive.write(b'tampered')
        with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
            self.prepare()
        self.assertFalse(self.output.exists())

    def test_rejects_wrong_platform_and_missing_architecture(self):
        self.archive('arm64', platform='linux/amd64')
        with self.assertRaisesRegex(ValueError, 'metadata mismatch'):
            self.prepare()
        del self.manifest['artifacts']['linux/arm64']
        with self.assertRaisesRegex(ValueError, 'Missing Linux full artifact'):
            self.prepare()

    def test_rejects_archive_path_escape(self):
        for name in ('../../escaped', '/absolute', 'C:/absolute', 'packages/../escaped', 'packages\\escaped'):
            with self.subTest(name=name):
                self.archive('arm64', extra={name: b'bad'})
                with self.assertRaises(ValueError):
                    self.prepare()
                self.assertFalse(self.output.exists())
        self.assertFalse((self.root / 'escaped').exists())


if __name__ == '__main__':
    unittest.main()
