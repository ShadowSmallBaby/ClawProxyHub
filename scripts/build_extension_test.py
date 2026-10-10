import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch
import extension_build
from release_config import content_key


spec = importlib.util.spec_from_file_location('extension_builder', Path(__file__).with_name('build-extension.py'))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class ExtensionBuilderTests(unittest.TestCase):
    def test_cache_tracks_assets_bridge_and_producers_but_not_outputs(self):
        with tempfile.TemporaryDirectory() as directory, \
                patch.object(extension_build, 'targets', return_value=[]), \
                patch.object(extension_build, 'ROOT', Path(directory).resolve()):
            root = Path(directory).resolve()
            bridge = root / 'sdk/extension/bridge.js'
            bridge.parent.mkdir(parents=True)
            bridge.write_text('bridge')
            (root / 'ui').mkdir()
            (root / 'manifest.json').write_text('{"id":"test"}')
            (root / 'icon.svg').write_text('first')
            (root / 'prepare.py').write_text('producer')
            project = root / 'build.json'
            project.write_text(json.dumps({
                'format': 1, 'manifest': 'manifest.json', 'backend': {},
                'frontend': {'directory': 'ui', 'output': 'out', 'sdk': 'bridge.js'},
                'assets': {'icon.svg': 'icon.svg', 'generated': {'source': 'ui/generated', 'inputs': ['prepare.py']}}
            }))
            files, properties = extension_build.build_inputs(project)
            cold_key = content_key(files, properties)
            self.assertIn(root / 'icon.svg', files)
            self.assertIn(root / 'prepare.py', files)
            self.assertIn(extension_build.ROOT / 'sdk/extension/bridge.js', files)
            (root / 'ui/generated').write_text('generated')
            (root / 'ui/out').mkdir()
            (root / 'ui/out/index.html').write_text('built')
            warm, _ = extension_build.build_inputs(project)
            self.assertEqual(set(files), set(warm))
            self.assertEqual(cold_key, content_key(warm, properties))
            for path in (root / 'icon.svg', root / 'prepare.py', bridge):
                previous = content_key(files, properties)
                path.write_text(path.read_text() + 'changed')
                self.assertNotEqual(previous, content_key(files, properties))

    def test_android_release_only_includes_arm64_and_test_build_adds_x86(self):
        config = {'language': 'go', 'module': 'backend', 'package': './cmd', 'android': {'package': './android'}}
        with tempfile.TemporaryDirectory() as directory, \
                patch.object(extension_build, 'backend_target', return_value=(Path(directory), './cmd')), \
                patch.object(extension_build, 'android_environment', side_effect=lambda spec, arch, env: dict(env, GOOS='android', GOARCH=arch, CGO_ENABLED='1')):
            release = extension_build.targets(Path(directory), config)
            self.assertEqual([item[0] for item in release], list(extension_build.PLATFORMS) + ['android/arm64'])
            local = extension_build.targets(Path(directory), config, 'x86_64')
            self.assertEqual([item[0] for item in local], list(extension_build.PLATFORMS) + ['android/arm64', 'android/amd64'])
            self.assertTrue(local[-1][-1].endswith('.so'))
            self.assertEqual(local[-1][3]['CGO_ENABLED'], '1')
            with self.assertRaises(ValueError):
                extension_build.targets(Path(directory), config, 'armeabi-v7a')

    def test_android_compiler_uses_pinned_ndk_and_minimum_sdk(self):
        with tempfile.TemporaryDirectory(prefix='cph ndk ') as directory:
            ndk = Path(directory)
            (ndk / 'source.properties').write_text('Pkg.Revision = 28.2.13676358\n')
            host = 'windows-x86_64' if os.name == 'nt' else 'darwin-x86_64' if sys.platform == 'darwin' else 'linux-x86_64'
            compiler = ndk / 'toolchains/llvm/prebuilt' / host / 'bin' / ('clang.exe' if os.name == 'nt' else 'clang')
            compiler.parent.mkdir(parents=True)
            compiler.touch()
            with patch.dict(os.environ, {'ANDROID_NDK_HOME': str(ndk)}):
                environment = extension_build.android_environment({'package': './android', 'min_sdk': 26}, 'arm64', {})
                self.assertIn('"' + compiler.resolve().as_posix() + '"', environment['CC'])
                self.assertIn('--target=aarch64-linux-android26', environment['CC'])
                self.assertIn('max-page-size=16384', environment['CGO_LDFLAGS'])
                with self.assertRaises(ValueError):
                    extension_build.android_environment({'min_sdk': 23}, 'arm64', {})
                with self.assertRaises(ValueError):
                    extension_build.android_environment({'ndk': '27.0.0'}, 'arm64', {})

    def test_build_step_preserves_arguments_and_static_assets(self):
        with tempfile.TemporaryDirectory(prefix='cph frontend ') as directory:
            root = Path(directory)
            source, copied = root / 'source', root / 'copied'
            source.mkdir()
            marker = 'spaces and & shell $ characters'
            builder.step([sys.executable, '-c', 'from pathlib import Path; import sys; Path("index.html").write_text(sys.argv[1])', marker], source)
            builder.copy_frontend(source, copied)
            self.assertEqual((copied / 'index.html').read_text(), marker)

    def test_go_library_cannot_be_packaged_as_backend(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            module = root / 'backend'
            module.mkdir()
            (module / 'go.mod').write_text('module example.test/extension\n\ngo 1.23\n')
            source = module / 'main.go'
            source.write_text('package library\nfunc Run() {}\n')
            config = {'language': 'go', 'module': 'backend', 'package': '.', 'cgo': False}
            environment = dict(os.environ, GOWORK='off', CGO_ENABLED='0')
            with self.assertRaisesRegex(ValueError, 'main package'):
                builder.backend_target(root, config, environment)
            source.write_text('package main\nfunc main() {}\n')
            self.assertEqual(builder.backend_target(root, config, environment), (module.resolve(), '.'))
            for target in ('../outside', '-o', 'example.test/other'):
                with self.assertRaises(ValueError):
                    builder.backend_target(root, dict(config, package=target), environment)
            with self.assertRaises(ValueError):
                builder.backend_target(root, dict(config, cgo=True), environment)

    def test_output_paths_stay_inside_the_project(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            self.assertEqual(builder.inside(root, 'frontend/dist/index.html'), root / 'frontend/dist/index.html')
            for path in ('../outside', str(root.parent / 'outside')):
                with self.assertRaises(ValueError):
                    builder.inside(root, path)


if __name__ == '__main__':
    unittest.main()
