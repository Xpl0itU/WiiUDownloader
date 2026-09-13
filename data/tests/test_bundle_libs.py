#!/usr/bin/env python3
"""Tests for data/bundle_libs.py.

Pins the two halves of the macOS launch bug where the bundle shipped libwebp
without libsharpyuv, so dyld aborted with "Library not loaded:
@rpath/libsharpyuv.0.dylib" before the app could start.
"""

import contextlib
import io
import os
import shutil
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from bundle_libs import bundle_lib, get_deps, verify_bundle


def homebrew_libwebp():
    """Homebrew's libwebp, the library that exposed the bug."""
    for prefix in ("/opt/homebrew", "/usr/local"):
        for candidate in (
            os.path.join(prefix, "opt", "webp", "lib", "libwebp.7.dylib"),
            os.path.join(prefix, "lib", "libwebp.7.dylib"),
        ):
            if os.path.exists(candidate):
                return os.path.realpath(candidate)
    return None


class DylibCollectionTest(unittest.TestCase):
    def setUp(self):
        fixture = homebrew_libwebp()
        if fixture is None:
            self.skipTest("homebrew libwebp is not installed")
        self.libwebp = fixture

    def test_relative_dependencies_are_collected(self):
        deps = get_deps(self.libwebp)
        self.assertIn("@rpath/libsharpyuv.0.dylib", deps)
        self.assertFalse([dep for dep in deps if dep.startswith("/usr/lib/")], deps)

    def test_copying_follows_relative_dependencies(self):
        with tempfile.TemporaryDirectory() as dest:
            bundle_lib(self.libwebp, dest, set(), [os.path.dirname(self.libwebp)])
            copied = sorted(os.listdir(dest))
            self.assertIn("libwebp.7.dylib", copied)
            self.assertIn("libsharpyuv.0.dylib", copied)


class VerifyBundleTest(unittest.TestCase):
    def setUp(self):
        fixture = homebrew_libwebp()
        if fixture is None:
            self.skipTest("homebrew libwebp is not installed")
        self.libwebp = fixture
        self.root = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.root, ignore_errors=True)
        self.lib_dir = os.path.join(self.root, "lib")
        os.makedirs(self.lib_dir)
        self.main_exe = os.path.join(self.root, "WiiUDownloader")
        shutil.copy2(self.libwebp, self.main_exe)
        shutil.copy2(self.libwebp, os.path.join(self.lib_dir, "libwebp.7.dylib"))

    def test_bundle_missing_a_dylib_fails_the_build(self):
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            with self.assertRaises(SystemExit) as caught:
                verify_bundle(self.main_exe, self.root)
        self.assertEqual(caught.exception.code, 1)
        self.assertIn("libwebp.7.dylib -> @rpath/libsharpyuv.0.dylib", err.getvalue())

    def test_complete_bundle_passes(self):
        shutil.copy2(
            os.path.join(os.path.dirname(self.libwebp), "libsharpyuv.0.dylib"),
            os.path.join(self.lib_dir, "libsharpyuv.0.dylib"),
        )
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            verify_bundle(self.main_exe, self.root)
        self.assertIn("verification passed", out.getvalue())


if __name__ == "__main__":
    unittest.main()
