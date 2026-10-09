from pathlib import Path
import tempfile
import unittest

from release_config import cached_package, cache_package, content_key, distribution_presets, read_config, validate_environments


class PackageCacheTests(unittest.TestCase):
    def test_interface_scope_is_optional_but_explicit_values_are_validated(self):
        for values in (None, ['app'], ['web-desktop'], ['web-mobile'], ['app', 'web-desktop', 'web-mobile']):
            validate_environments(values)
        for values in ([], 'app', ['desktop'], ['app', 'app'], [1], [{}]):
            with self.subTest(values=values), self.assertRaisesRegex(ValueError, 'environments'):
                validate_environments(values)

    def test_full_distribution_selects_packages_for_each_surface(self):
        config = read_config()
        profiles = distribution_presets(config)
        self.assertEqual(set(profiles), {'full'})
        self.assertEqual(set(profiles['full']['packages']), {'lua-runtime', 'lua-editor'})
        self.assertEqual(config['frontends']['app-full']['packages'], [])
        self.assertEqual(set(config['frontends']), {'web-full', 'app-full'})

    def test_only_declared_inputs_affect_component_identity(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            runtime, core = root / 'runtime.go', root / 'core.go'
            runtime.write_text('runtime v1')
            core.write_text('core v1')
            inputs = {'platform': 'linux/arm64', 'signer': 'public-key-1', 'go': 'go1.26'}
            original = content_key([runtime], inputs, root)
            core.write_text('core v2')
            self.assertEqual(original, content_key([runtime], inputs, root))
            for field in inputs:
                self.assertNotEqual(original, content_key([runtime], dict(inputs, **{field: 'changed'}), root))
            runtime.write_text('runtime v2')
            self.assertNotEqual(original, content_key([runtime], inputs, root))

    def test_corrupt_or_incomplete_cache_is_a_miss(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            package = root / 'test.cphext'
            package.write_bytes(b'package')
            cache = root / 'cache'
            cache_package(cache, 'key', package)
            cached = cached_package(cache, 'key', package.name)
            self.assertIsNotNone(cached)
            cached.write_bytes(b'corrupt')
            self.assertIsNone(cached_package(cache, 'key', package.name))
            self.assertIsNone(cached_package(cache, 'another-input', package.name))

if __name__ == '__main__':
    unittest.main()
