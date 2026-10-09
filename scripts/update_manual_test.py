import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import zipfile

from project_config import ROOT, apk_name, compatibility_manifest, package_manifest, read_project
from release_config import content_key, read_config

spec = importlib.util.spec_from_file_location('update_manual', ROOT / 'scripts/update-manual.py')
updates = importlib.util.module_from_spec(spec)
spec.loader.exec_module(updates)


class ProjectMetadataTests(unittest.TestCase):
    def test_compatibility_sync_preserves_bilingual_logs_and_unknown_fields(self):
        project = read_project(environment=False)
        original = {'version': '0.0.0', 'custom': True, 'changelog': {'items': [{'zh': '桌面', 'en': 'Desktop'}]},
                    'platform': {'android': {'changelog': {'items': [{'zh': '安卓', 'en': 'Android'}]}}}}
        actual = compatibility_manifest(project, original)
        self.assertEqual(actual['version'], project['components']['core']['version'])
        self.assertEqual(actual['changelog'], original['changelog'])
        self.assertEqual(actual['platform'], original['platform'])
        self.assertEqual(set(actual['platform']['android']), {'changelog'})
        self.assertTrue(actual['custom'])
        self.assertEqual(original['version'], '0.0.0')

    def test_core_version_and_repository_do_not_invalidate_unchanged_packages(self):
        project = read_project(environment=False)
        original = package_manifest('lua-runtime', project)
        project['components']['core']['version'] = '99.0.0'
        project['updates']['repository'] = 'https://github.com/example/fork'
        self.assertEqual(content_key([], original), content_key([], package_manifest('lua-runtime', project)))
        major, minor, patch_version = map(int, original['version'].split('.'))
        project['components']['lua-runtime']['version'] = f'{major}.{minor}.{patch_version + 1}'
        self.assertNotEqual(content_key([], original), content_key([], package_manifest('lua-runtime', project)))

    def test_repository_override_is_shared_and_checked(self):
        with patch.dict('os.environ', {'CPH_REPOSITORY_URL': 'https://github.com/example/fork/'}):
            self.assertEqual(read_project()['updates']['repository'], 'https://github.com/example/fork')
            self.assertNotEqual(read_project(environment=False)['updates']['repository'], 'https://github.com/example/fork')
        for url in ['http://github.com/example/fork', 'https://user:secret@github.com/example/fork',
                    'https://github.com/example/fork?x=1', 'https://github.com/../fork']:
            with patch.dict('os.environ', {'CPH_REPOSITORY_URL': url}), self.assertRaises(ValueError):
                read_project()


class ReleaseManifestTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.project = read_project(environment=False)
        self.project['updates']['repository'] = 'https://github.com/example/fork'
        # 固定升级样例的起始版本，避免项目发版改变升级与降级场景。
        self.project['components']['lua-editor']['version'] = '0.1.0'
        self.config = read_config()
        self.source = {'version': '0.0.0', 'changelog': {'items': [{'en': 'Desktop change'}]},
                       'platform': {'android': {'changelog': {'items': [{'en': 'Android change'}]}}}}
        for profile in ('full',):
            files = {'cph': b'core', 'cli': b'cli'}
            lock = {'format': 1, 'profile': profile, 'platform': 'linux/amd64', 'core': 'cph', 'cli': 'cli', 'packages': [],
                    'version': self.project['components']['core']['version'], 'core_sha256': hashlib.sha256(b'core').hexdigest(),
                    'files': {name: hashlib.sha256(data).hexdigest() for name, data in files.items()}}
            frontend = self.config['distributions'][profile]['frontend']
            lock['frontend'] = {'profile': frontend, 'version': self.project['components']['frontend']['version'], 'packages': self.config['frontends'][frontend]['packages']}
            files['distribution.lock.json'] = json.dumps(lock)
            self.archive(f'cph-linux-amd64-{profile}.zip', {'ClawProxyHub/' + name: data for name, data in files.items()})
        android = dict(self.project['components']['android'], core_version=self.project['components']['core']['version'],
                       frontend={'profile': 'app-full', 'version': self.project['components']['frontend']['version']})
        self.apk = apk_name(android, 'arm64-v8a')
        self.archive(self.apk, {'assets/app-version.json': json.dumps(android), 'AndroidManifest.xml': b'manifest', 'lib/arm64-v8a/libcphcore.so': b'core'})
        package = package_manifest('lua-editor', self.project)
        self.archive(f"lua-editor-{package['version']}.cphext", {'manifest.json': json.dumps(package)})
        app = package_manifest('frontend.app', self.project)
        self.archive(f"app-{app['version']}.cphui", {'manifest.json': json.dumps(app), 'frontend/profile.json': json.dumps({
            'profile': 'app-full', 'version': app['version'], 'packages': [],
        })})

    def archive(self, name, files):
        with zipfile.ZipFile(self.root / name, 'w') as archive:
            for path, value in files.items():
                archive.writestr(path, value)

    def generate(self):
        return updates.generate(self.root, 'v2.0.0', self.project, self.source, self.config)

    def test_manifest_uses_actual_artifacts_and_independent_versions(self):
        result = self.generate()
        self.assertEqual(result['version'], self.project['components']['core']['version'])
        self.assertEqual(result['changelog']['items'][0]['en'], 'Desktop change')
        self.assertEqual(result['platform']['android']['changelog']['items'][0]['en'], 'Android change')
        asset = result['platform']['android']['artifacts']['arm64-v8a']
        self.assertEqual(asset['name'], self.apk)
        self.assertNotIn('v2.0.0', self.apk)
        self.assertEqual(asset['sha256'], hashlib.sha256((self.root / self.apk).read_bytes()).hexdigest())
        self.assertEqual(asset['size'], (self.root / self.apk).stat().st_size)
        self.assertTrue(asset['download_url'].startswith('https://github.com/example/fork/releases/download/v2.0.0/'))
        self.assertEqual(set(result['artifacts']['linux/amd64']), {'full'})
        editor = next(item for item in result['packages'] if item['id'] == 'lua-editor')
        self.assertEqual(editor['platforms'], [])
        self.assertEqual(editor['environments'], ['app', 'web-desktop', 'web-mobile'])
        self.assertEqual(set(result['frontends']), {'app'})
        self.assertTrue(result['frontends']['app']['name'].endswith('.cphui'))
        self.assertNotIn('tools', result)

    def test_cached_artifacts_with_old_versions_are_rejected(self):
        for component in ('core', 'android', 'lua-editor', 'frontend'):
            with self.subTest(component=component):
                project = copy.deepcopy(self.project)
                project['components'][component]['version'] = '99.0.0'
                with self.assertRaises(ValueError):
                    updates.generate(self.root, 'v2.0.0', project, self.source, self.config)

    def test_invalid_layout_or_corrupt_checksum_is_rejected(self):
        sidecar = self.root / 'cph-linux-amd64-full.zip.sha256'
        sidecar.write_text('incorrect checksum', encoding='utf-8')
        with self.assertRaisesRegex(ValueError, 'Checksum mismatch'):
            self.generate()
        sidecar.unlink()
        path = self.root / 'cph-linux-amd64-full.zip'
        with zipfile.ZipFile(path) as archive:
            files = {name.removeprefix('ClawProxyHub/'): archive.read(name) for name in archive.namelist()}
        self.archive(path.name, files)
        with self.assertRaisesRegex(ValueError, 'Missing or invalid distribution'):
            self.generate()

    def test_component_update_preserves_core_apk_and_logs(self):
        previous = self.generate()
        previous['custom'] = {'preserve': True}
        generated = self.component_update('0.1.1')
        result = updates.merge_components(previous, generated, self.project['updates']['repository'])
        for key in ('version', 'changelog', 'artifacts', 'platform', 'custom', 'frontends'):
            self.assertEqual(result[key], previous[key])
        self.assertEqual(next(item for item in result['packages'] if item['id'] == 'lua-editor')['version'], '0.1.1')
        self.assertEqual(next(item for item in previous['packages'] if item['id'] == 'lua-editor')['version'], '0.1.0')

    def test_component_merge_validates_and_preserves_interface_scope(self):
        previous = self.generate()
        generated = self.component_update('0.1.1')
        package = next(item for item in generated['packages'] if item['id'] == 'lua-editor')
        for value in ([], ['web'], ['app', 'app']):
            package['environments'] = value
            with self.subTest(value=value), self.assertRaisesRegex(ValueError, 'environments'):
                updates.merge_components(previous, generated, self.project['updates']['repository'])
        package['environments'] = ['web-desktop']
        result = updates.merge_components(previous, generated, self.project['updates']['repository'])
        self.assertEqual(next(item for item in result['packages'] if item['id'] == 'lua-editor')['environments'], ['web-desktop'])

    def component_update(self, version, core='1.5.2'):
        directory = self.root / ('components-' + version)
        directory.mkdir(exist_ok=True)
        project = copy.deepcopy(self.project)
        project['components']['core']['version'] = '99.0.0'
        project['components']['lua-editor']['version'] = version
        package = package_manifest('lua-editor', project)
        package['core']['min'] = core
        with zipfile.ZipFile(directory / f'lua-editor-{version}.cphext', 'w') as archive:
            archive.writestr('manifest.json', json.dumps(package))
            archive.writestr('new-content.txt', b'new content')
        return updates.generate(directory, 'v2.0.0', project, self.source, self.config, components_only=True)

    def test_component_updates_reject_downgrade_same_version_and_incompatible_core(self):
        previous = self.generate()
        for version, core, message in [('0.1.0', '1.5.2', 'immutable'), ('0.0.9', '1.5.2', 'downgrade'), ('0.1.1', '99.0.0', 'newer core')]:
            with self.subTest(version=version), self.assertRaisesRegex(ValueError, message):
                updates.merge_components(previous, self.component_update(version, core), self.project['updates']['repository'])
        with self.assertRaisesRegex(ValueError, 'cannot replace'):
            updates.generate(self.root, 'v2.0.0', self.project, self.source, self.config, components_only=True)
        with self.assertRaisesRegex(ValueError, 'repository and tag'):
            updates.merge_components(previous, self.component_update('0.1.2'), 'https://github.com/another/repository')


if __name__ == '__main__':
    unittest.main()
