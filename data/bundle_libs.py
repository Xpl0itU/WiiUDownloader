"""Dylib collection and verification for the macOS app bundle.

Kept apart from create_bundle.py so the walking and the check can be exercised
without building a bundle.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
import sys
from typing import List, Set

HOMEBREW_PREFIXES = ("/opt/homebrew", "/usr/local", "/opt/local")


def load_commands(path: str) -> List[str]:
    """Every dependency path in path's load commands.

    otool prints one header block per architecture for a universal binary —
    "<path> (architecture arm64):" — so depend only on lines carrying the
    version annotation rather than matching any parenthesised line.
    """
    if not os.path.exists(path):
        print(f"Warning: path does not exist: {path}")
        return []
    res = subprocess.run(
        f'otool -L "{path}"', shell=True, capture_output=True, text=True
    )
    if res.returncode != 0:
        print(f"Warning: otool failed for {path}")
        return []
    deps: List[str] = []
    for line in res.stdout.split("\n"):
        line = line.strip()
        match = re.match(r"^(.+?)\s+\(compatibility version", line)
        if match:
            deps.append(match.group(1))
    return deps


def get_deps(path: str) -> List[str]:
    """Dependencies that have to be copied into the bundle.

    Homebrew records some transitive deps relative — libwebp references
    @rpath/libsharpyuv.0.dylib — so accepting only absolute Homebrew paths
    silently dropped them and the bundle shipped incomplete. System libraries
    are still left alone.
    """
    return [
        dep
        for dep in load_commands(path)
        if dep.startswith("@") or dep.startswith(HOMEBREW_PREFIXES)
    ]


def bundle_lib(src_path: str, dest_dir: str, processed: Set[str], search_paths: List[str]) -> None:
    """Copy src_path into dest_dir, then recurse into its dependencies."""
    if not src_path or src_path in processed:
        return
    real_src = os.path.realpath(src_path)
    if not os.path.exists(real_src):
        name = os.path.basename(src_path)
        for sp in search_paths:
            candidate = os.path.join(sp, name)
            if os.path.exists(candidate):
                real_src = os.path.realpath(candidate)
                break
        else:
            print(f"Warning: {name} needed by {src_path} is not in any search path; not bundled")
            return
    name = os.path.basename(src_path)
    dest_path = os.path.join(dest_dir, name)
    if not os.path.exists(dest_path):
        shutil.copy2(real_src, dest_path)
        os.chmod(dest_path, 0o755)
    processed.add(src_path)
    processed.add(real_src)
    for dep in get_deps(dest_path):
        bundle_lib(dep, dest_dir, processed, search_paths)


def verify_bundle(main_exe: str, macos_path: str) -> None:
    """Fail the build if anything in the bundle loads a dylib the bundle lacks.

    The copier used to drop @rpath deps, so an incomplete bundle shipped and only
    died at launch on the user's machine ("Library not loaded:
    @rpath/libsharpyuv.0.dylib"). This turns that into a build failure.
    """
    bundled: Set[str] = set()
    binaries = [main_exe]
    for root, _, files in os.walk(macos_path):
        for f in files:
            if f.endswith((".dylib", ".so")):
                bundled.add(f)
                binaries.append(os.path.join(root, f))

    missing = []
    for binary in binaries:
        for dep in load_commands(binary):
            if dep.startswith(("/usr/lib/", "/System/")):
                continue
            if os.path.basename(dep) not in bundled:
                missing.append(f"{os.path.basename(binary)} -> {dep}")

    if missing:
        print("=== Bundle verification FAILED: dylibs missing from the bundle ===", file=sys.stderr)
        for entry in sorted(set(missing)):
            print(f"  {entry}", file=sys.stderr)
        print("This bundle would abort at launch with 'Library not loaded'.", file=sys.stderr)
        sys.exit(1)
    print(
        f"Bundle verification passed ({len(bundled)} bundled dylibs, "
        "every load command resolves inside the bundle)"
    )
