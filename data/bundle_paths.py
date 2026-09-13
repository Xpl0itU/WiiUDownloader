from __future__ import annotations

import os
import re
import subprocess
from typing import Callable, List, Optional

ABSOLUTE_PATH_PREFIXES = ("/opt/homebrew", "/usr/local", "/opt/local")


def _run(cmd: str, run_fn: Optional[Callable[[str], object]] = None) -> subprocess.CompletedProcess:
    if run_fn is not None:
        run_fn(cmd)
    return subprocess.run(cmd, shell=True, capture_output=True, text=True)


def get_absolute_deps(binary_path: str, run_fn: Optional[Callable[[str], object]] = None) -> List[str]:
    res = _run(f'otool -L "{binary_path}"', run_fn)
    if res.returncode != 0:
        return []
    deps: List[str] = []
    for line in res.stdout.split("\n"):
        line = line.strip()
        # otool prints a "<path> (architecture arch):" header per slice for a
        # universal binary; only real deps carry the compatibility version.
        match = re.match(r"^(.+?)\s+\(compatibility version", line)
        if not match:
            continue
        dep_path = match.group(1).strip()
        if any(dep_path.startswith(p) for p in ABSOLUTE_PATH_PREFIXES):
            deps.append(dep_path)
    return deps


def get_existing_rpaths(
    binary_path: str,
    run_fn: Optional[Callable[[str], object]] = None,
) -> List[str]:
    cmd = f'otool -l "{binary_path}"'
    if run_fn is not None:
        run_fn(cmd)
    res = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    rpaths: List[str] = []
    if res.returncode != 0:
        return rpaths
    lines = res.stdout.split("\n")
    for i, line in enumerate(lines):
        if "cmd LC_RPATH" in line:
            for j in range(1, 5):
                if i + j < len(lines):
                    match = re.search(r"^\s*path\s+(\S+)", lines[i + j])
                    if match:
                        rpaths.append(match.group(1))
                        break
    return rpaths


def add_rpath_if_missing(
    binary_path: str,
    new_rpath: str,
    run_fn: Optional[Callable[[str], object]] = None,
) -> bool:
    if new_rpath in get_existing_rpaths(binary_path, run_fn):
        return False
    _run(f'install_name_tool -add_rpath "{new_rpath}" "{binary_path}"', run_fn)
    return True


def rewrite_binary(
    binary_path: str,
    is_main_exe: bool = False,
    run_fn: Optional[Callable[[str], object]] = None,
) -> None:
    for dep in get_absolute_deps(binary_path, run_fn):
        basename = os.path.basename(dep)
        _run(
            f'install_name_tool -change "{dep}" "@rpath/{basename}" "{binary_path}"',
            run_fn,
        )

    if not is_main_exe:
        basename = os.path.basename(binary_path)
        _run(f'install_name_tool -id "@rpath/{basename}" "{binary_path}"', run_fn)

    if is_main_exe:
        add_rpath_if_missing(binary_path, "@executable_path/lib", run_fn)
    else:
        add_rpath_if_missing(binary_path, "@loader_path", run_fn)
        add_rpath_if_missing(binary_path, "@loader_path/..", run_fn)
