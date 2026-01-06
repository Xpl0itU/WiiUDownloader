#!/usr/bin/env python3
import os
import shutil
import subprocess
import sys
import re
import glob


def run(cmd, check=True):
    print(f"$ {cmd}")
    result = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    if check and result.returncode != 0:
        print(f"STDOUT: {result.stdout}")
        print(f"STDERR: {result.stderr}")
        return None
    return result


def get_deps(path):
    """Get all non-system dependencies of a binary."""
    res = run(f'otool -L "{path}"', check=False)
    if not res:
        return []
    deps = []
    for line in res.stdout.split("\n")[1:]:
        line = line.strip()
        if not line:
            continue
        # Extract path
        match = re.match(r"^(.+?)\s+\(", line)
        if not match:
            continue
        dep_path = match.group(1)
        # Filter for non-system, non-embedded paths
        if (
            dep_path.startswith("/opt/homebrew")
            or dep_path.startswith("/usr/local")
            or dep_path.startswith("/opt/local")
        ):
            deps.append(dep_path)
    return deps


def fix_binary(path, lib_dir_rel_to_exe="@executable_path/lib"):
    """Fix all dependencies of a binary to point to the local lib dir."""
    deps = get_deps(path)
    for dep in deps:
        dep_name = os.path.basename(dep)
        new_path = f"{lib_dir_rel_to_exe}/{dep_name}"
        run(f'install_name_tool -change "{dep}" "{new_path}" "{path}"', check=False)
    # Also fix the ID of the binary if it's a dylib
    if path.endswith(".dylib") or ".dylib." in path or path.endswith(".so"):
        name = os.path.basename(path)
        run(
            f'install_name_tool -id "{lib_dir_rel_to_exe}/{name}" "{path}"', check=False
        )


def bundle_lib(src_path, dest_dir, processed, search_paths):
    """Recursively bundle a library and its dependencies."""
    if not src_path or src_path in processed:
        return

    # Resolve actual file
    real_src = os.path.realpath(src_path)
    if not os.path.exists(real_src):
        # Search for it
        name = os.path.basename(src_path)
        for sp in search_paths:
            candidate = os.path.join(sp, name)
            if os.path.exists(candidate):
                real_src = os.path.realpath(candidate)
                break
        else:
            print(f"!! Could not find {src_path}")
            return

    name = os.path.basename(src_path)
    dest_path = os.path.join(dest_dir, name)

    if not os.path.exists(dest_path):
        print(f"Bundling: {name}")
        shutil.copy2(real_src, dest_path)
        os.chmod(dest_path, 0o755)

    processed.add(src_path)
    processed.add(real_src)

    # Recurse
    for dep in get_deps(dest_path):
        bundle_lib(dep, dest_dir, processed, search_paths)


# Paths
executable_path = "main"
app_bundle_path = "out/WiiUDownloader.app"
contents_path = os.path.join(app_bundle_path, "Contents")
macos_path = os.path.join(contents_path, "MacOS")
resources_path = os.path.join(contents_path, "Resources")
lib_path = os.path.join(macos_path, "lib")

try:
    brew_prefix = subprocess.check_output(["brew", "--prefix"]).decode().strip()
except:
    brew_prefix = "/opt/homebrew"

search_paths = [os.path.join(brew_prefix, "lib")]
opt_dir = os.path.join(brew_prefix, "opt")
if os.path.exists(opt_dir):
    for item in os.listdir(opt_dir):
        p = os.path.join(opt_dir, item, "lib")
        if os.path.isdir(p):
            search_paths.append(p)

# Prep bundle
if os.path.exists(app_bundle_path):
    shutil.rmtree(app_bundle_path)
os.makedirs(macos_path)
os.makedirs(resources_path)
os.makedirs(lib_path)

shutil.copy("data/Info.plist", os.path.join(contents_path, "Info.plist"))
shutil.copy(executable_path, os.path.join(macos_path, "WiiUDownloader"))
os.chmod(os.path.join(macos_path, "WiiUDownloader"), 0o755)

# 1. Recursive Bundling
print("=== Recursive Bundling ===")
processed = set()
main_exe = os.path.join(macos_path, "WiiUDownloader")
for dep in get_deps(main_exe):
    bundle_lib(dep, lib_path, processed, search_paths)

# 2. Bundle GdkPixbuf Loaders & GIO Modules
print("=== Bundling Modules ===")
# Essential loaders
for pattern in [
    "libpixbufloader-png.so",
    "libpixbufloader-svg.so",
    "libpixbufloader-ico.so",
]:
    matches = glob.glob(
        os.path.join(brew_prefix, "lib", "gdk-pixbuf-2.0", "*", "loaders", pattern)
    )
    if matches:
        dest_mod = os.path.join(lib_path, "loaders")
        os.makedirs(dest_mod, exist_ok=True)
        shutil.copy2(os.path.realpath(matches[0]), os.path.join(dest_mod, pattern))
        bundle_lib(
            matches[0], lib_path, processed, search_paths
        )  # Bundle dependencies of the loader too!

# GIO modules
gio_modules = glob.glob(os.path.join(brew_prefix, "lib", "gio", "modules", "*.so"))
if gio_modules:
    dest_gio = os.path.join(lib_path, "gio-modules")
    os.makedirs(dest_gio, exist_ok=True)
    for gm in gio_modules:
        shutil.copy2(os.path.realpath(gm), os.path.join(dest_gio, os.path.basename(gm)))
        bundle_lib(gm, lib_path, processed, search_paths)

# 3. DEEP FIX PASS
print("=== Deep Fix Pass ===")
# Fix EVERY binary (.dylib or .so) in the entire MacOS folder
for root, dirs, files in os.walk(macos_path):
    for f in files:
        if (
            f.endswith(".dylib")
            or ".dylib." in f
            or f.endswith(".so")
            or f == "WiiUDownloader"
        ):
            p = os.path.join(root, f)
            # Determine relative path to 'lib' directory
            rel_to_macos = os.path.relpath(lib_path, os.path.dirname(p))
            prefix = (
                "@executable_path"
                if rel_to_macos == "lib"
                else f"@loader_path/{rel_to_macos}"
            )
            fix_binary(p, prefix)
            run(f'codesign --force --sign - "{p}"', check=False)

# 4. Resources
print("=== Resources ===")
share_src = os.path.join(brew_prefix, "share")
dest_share = os.path.join(resources_path, "share")
for item in ["glib-2.0/schemas", "icons/Adwaita", "icons/hicolor", "themes/Adwaita"]:
    src = os.path.join(share_src, item)
    if os.path.exists(src):
        dst = os.path.join(dest_share, item)
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        if os.path.isdir(src):
            shutil.copytree(
                os.path.realpath(src),
                dst,
                symlinks=False,
                ignore_dangling_symlinks=True,
            )
        else:
            shutil.copy2(os.path.realpath(src), dst)

print(f"=== Bundle Complete: {len(os.listdir(lib_path))} libs ===")
