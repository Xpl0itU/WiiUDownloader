#!/usr/bin/env python3
"""
Nuclear option: Manual library bundling without relying on dylibbundler.
Uses otool and install_name_tool directly for complete control.
"""

import os
import shutil
import subprocess
import sys
import re


def run(cmd, check=True):
    print(f"$ {cmd}")
    result = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    if check and result.returncode != 0:
        print(f"STDOUT: {result.stdout}")
        print(f"STDERR: {result.stderr}")
        print(f"Command failed with exit code {result.returncode}")
        sys.exit(1)
    return result


def get_dependencies(binary_path):
    """Get list of dynamic library dependencies using otool."""
    result = run(f'otool -L "{binary_path}"', check=False)
    deps = []
    for line in result.stdout.split("\n")[1:]:  # Skip first line (binary name)
        line = line.strip()
        if (
            line
            and not line.startswith("@")
            and not line.startswith("/System")
            and not line.startswith("/usr/lib")
        ):
            # Extract path (before the parentheses)
            match = re.match(r"^(.+?)\s+\(", line)
            if match:
                deps.append(match.group(1))
    return deps


def copy_and_fix_lib(lib_path, lib_dir, processed, search_paths):
    """Copy a library and fix its dependencies recursively."""
    if lib_path in processed:
        return

    # Resolve the actual file (follow symlinks)
    if os.path.islink(lib_path):
        real_path = os.path.realpath(lib_path)
    else:
        real_path = lib_path

    if not os.path.exists(real_path):
        # Try to find it in search paths
        lib_name = os.path.basename(lib_path)
        for sp in search_paths:
            candidate = os.path.join(sp, lib_name)
            if os.path.exists(candidate):
                real_path = os.path.realpath(candidate)
                break
        else:
            print(f"Warning: Could not find {lib_path}")
            return

    lib_name = os.path.basename(lib_path)
    dest_path = os.path.join(lib_dir, lib_name)

    if os.path.exists(dest_path):
        processed.add(lib_path)
        return

    print(f"Copying: {lib_name}")
    shutil.copy2(real_path, dest_path)
    os.chmod(dest_path, 0o755)
    processed.add(lib_path)

    # Fix the library's own ID
    run(
        f'install_name_tool -id "@executable_path/lib/{lib_name}" "{dest_path}"',
        check=False,
    )

    # Get and process this library's dependencies
    deps = get_dependencies(dest_path)
    for dep in deps:
        dep_name = os.path.basename(dep)
        # Fix the reference in the copied library
        run(
            f'install_name_tool -change "{dep}" "@executable_path/lib/{dep_name}" "{dest_path}"',
            check=False,
        )
        # Recursively copy the dependency
        copy_and_fix_lib(dep, lib_dir, processed, search_paths)


def safe_copy_dir(src, dst):
    """Copy directory, ignoring broken symlinks."""
    if not os.path.exists(dst):
        os.makedirs(dst)
    for item in os.listdir(src):
        s = os.path.join(src, item)
        d = os.path.join(dst, item)
        try:
            if os.path.islink(s):
                real = os.path.realpath(s)
                if os.path.exists(real):
                    if os.path.isdir(real):
                        safe_copy_dir(s, d)
                    else:
                        shutil.copy2(real, d)
            elif os.path.isdir(s):
                safe_copy_dir(s, d)
            else:
                shutil.copy2(s, d)
        except Exception as e:
            print(f"Warning: {e}")


# === MAIN ===

# Paths
executable_path = "main"
info_plist_path = "data/Info.plist"
app_bundle_path = "out/WiiUDownloader.app"
contents_path = os.path.join(app_bundle_path, "Contents")
macos_path = os.path.join(contents_path, "MacOS")
resources_path = os.path.join(contents_path, "Resources")
lib_path = os.path.join(macos_path, "lib")

# Get Homebrew prefix
try:
    brew_prefix = subprocess.check_output(["brew", "--prefix"]).decode().strip()
except:
    brew_prefix = "/opt/homebrew"

# Build search paths
search_paths = [os.path.join(brew_prefix, "lib")]
opt_dir = os.path.join(brew_prefix, "opt")
if os.path.exists(opt_dir):
    for item in os.listdir(opt_dir):
        p = os.path.join(opt_dir, item, "lib")
        if os.path.isdir(p):
            search_paths.append(p)

print(f"Using {len(search_paths)} search paths")

# Create bundle structure
if os.path.exists(app_bundle_path):
    shutil.rmtree(app_bundle_path)

os.makedirs(macos_path)
os.makedirs(resources_path)
os.makedirs(lib_path)

# Copy executable and Info.plist
shutil.copy(info_plist_path, os.path.join(contents_path, "Info.plist"))
shutil.copy(executable_path, os.path.join(macos_path, "WiiUDownloader"))
os.chmod(os.path.join(macos_path, "WiiUDownloader"), 0o755)

main_exe = os.path.join(macos_path, "WiiUDownloader")

# Get direct dependencies of main executable
print("\n=== Getting main executable dependencies ===")
deps = get_dependencies(main_exe)
print(f"Found {len(deps)} direct dependencies")
for d in deps:
    print(f"  {d}")

# Copy all libraries recursively
print("\n=== Copying libraries ===")
processed = set()
for dep in deps:
    copy_and_fix_lib(dep, lib_path, processed, search_paths)

# Fix main executable references
print("\n=== Fixing main executable references ===")
for dep in deps:
    dep_name = os.path.basename(dep)
    run(
        f'install_name_tool -change "{dep}" "@executable_path/lib/{dep_name}" "{main_exe}"',
        check=False,
    )

# Re-sign everything (required after modifying binaries)
print("\n=== Re-signing binaries ===")
for f in os.listdir(lib_path):
    if f.endswith(".dylib"):
        run(f'codesign --force --sign - "{os.path.join(lib_path, f)}"', check=False)
run(f'codesign --force --sign - "{main_exe}"', check=False)

# Verify
print("\n=== Verification ===")
lib_files = os.listdir(lib_path)
print(f"Library count: {len(lib_files)}")
gtk_found = any("libgtk-3" in f for f in lib_files)
gdk_found = any("libgdk-3" in f for f in lib_files)
print(f"GTK found: {gtk_found}")
print(f"GDK found: {gdk_found}")

if not gtk_found:
    print("FATAL: libgtk-3 not bundled!")
    sys.exit(1)

# === RESOURCES ===
print("\n=== Bundling resources ===")

# GdkPixbuf loaders (minimal - just PNG and SVG)
loaders_src = None
for ver in ["2.10.0", "2.10.1", "2.10.2"]:
    candidate = os.path.join(brew_prefix, "lib", "gdk-pixbuf-2.0", ver, "loaders")
    if os.path.isdir(candidate):
        loaders_src = candidate
        loaders_ver = ver
        break

if loaders_src:
    loaders_dst = os.path.join(lib_path, "gdk-pixbuf-2.0", loaders_ver, "loaders")
    os.makedirs(loaders_dst, exist_ok=True)

    for loader in ["libpixbufloader-png.so", "libpixbufloader-svg.so"]:
        src = os.path.join(loaders_src, loader)
        dst = os.path.join(loaders_dst, loader)
        if os.path.exists(src):
            print(f"Copying loader: {loader}")
            shutil.copy2(os.path.realpath(src), dst)
            os.chmod(dst, 0o755)
            # Fix loader dependencies
            loader_deps = get_dependencies(dst)
            for dep in loader_deps:
                dep_name = os.path.basename(dep)
                run(
                    f'install_name_tool -change "{dep}" "@executable_path/lib/{dep_name}" "{dst}"',
                    check=False,
                )
                copy_and_fix_lib(dep, lib_path, processed, search_paths)
            run(f'codesign --force --sign - "{dst}"', check=False)

# Schemas
schemas_src = os.path.join(brew_prefix, "share", "glib-2.0", "schemas")
schemas_dst = os.path.join(resources_path, "share", "glib-2.0", "schemas")
if os.path.exists(schemas_src):
    os.makedirs(schemas_dst, exist_ok=True)
    for f in os.listdir(schemas_src):
        src = os.path.join(schemas_src, f)
        if os.path.isfile(src) or (
            os.path.islink(src) and os.path.exists(os.path.realpath(src))
        ):
            try:
                shutil.copy2(
                    os.path.realpath(src) if os.path.islink(src) else src,
                    os.path.join(schemas_dst, f),
                )
            except:
                pass

# Icons (Adwaita)
icons_src = os.path.join(brew_prefix, "share", "icons", "Adwaita")
if os.path.exists(icons_src):
    print("Copying Adwaita icons...")
    icons_dst = os.path.join(resources_path, "share", "icons", "Adwaita")
    safe_copy_dir(icons_src, icons_dst)

print("\n=== BUNDLE COMPLETE ===")
print(f"Total libraries: {len(os.listdir(lib_path))}")
