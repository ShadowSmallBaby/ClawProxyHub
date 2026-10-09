import importlib.util
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch
import extension_build


spec = importlib.util.spec_from_file_location('extension_builder', Path(__file__).with_name('build-extension.py'))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class ExtensionBuilderTests(unittest.TestCase):
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
