#!/usr/bin/env python3
import os
import shutil
import subprocess
import sys
import re
import glob


def run(cmd):
    print(f"$ {cmd}")
    result = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"STDOUT: {result.stdout}")
        print(f"STDERR: {result.stderr}")
    return result


def get_deps(path):
    res = run(f'otool -L "{path}"')
    if not res:
        return []
    deps = []
    for line in res.stdout.split("\n")[1:]:
        line = line.strip()
        if not line:
            continue
        match = re.match(r"^(.+?)\s+\(", line)
        if not match:
            continue
        dep_path = match.group(1)
        if any(
            dep_path.startswith(p)
            for p in ["/opt/homebrew", "/usr/local", "/opt/local"]
        ):
            deps.append(dep_path)
    return deps


def bundle_lib(src_path, dest_dir, processed, search_paths):
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

if os.path.exists(app_bundle_path):
    shutil.rmtree(app_bundle_path)
os.makedirs(macos_path)
os.makedirs(resources_path)
os.makedirs(lib_path)

shutil.copy("data/Info.plist", os.path.join(contents_path, "Info.plist"))
shutil.copy(executable_path, os.path.join(macos_path, "WiiUDownloader"))
os.chmod(os.path.join(macos_path, "WiiUDownloader"), 0o755)

# Generate ICNS
print("=== Generating Icon ===")
icon_src = "data/WiiUDownloader.png"
if os.path.exists(icon_src):
    iconset = "WiiUDownloader.iconset"
    if os.path.exists(iconset):
        shutil.rmtree(iconset)
    os.makedirs(iconset)

    # Standard sizes
    sizes = [16, 32, 128, 256, 512]
    for s in sizes:
        subprocess.run(
            f"sips -z {s} {s} {icon_src} --out {iconset}/icon_{s}x{s}.png", shell=True
        )
        subprocess.run(
            f"sips -z {s*2} {s*2} {icon_src} --out {iconset}/icon_{s}x{s}@2x.png",
            shell=True,
        )

    subprocess.run(f"iconutil -c icns {iconset}", shell=True)
    if os.path.exists("WiiUDownloader.icns"):
        shutil.move(
            "WiiUDownloader.icns", os.path.join(resources_path, "WiiUDownloader.icns")
        )
        print("Icon created and installed.")
    else:
        print("Error: Failed to create icns file.")
    shutil.rmtree(iconset)
else:
    print(f"Warning: {icon_src} not found")

# 1. Recursive Bundle
processed = set()
main_exe = os.path.join(macos_path, "WiiUDownloader")
for dep in get_deps(main_exe):
    bundle_lib(dep, lib_path, processed, search_paths)

# 2. Bundle Modules (GIO/Loaders)
# GdkPixbuf loaders
loaders_dest = os.path.join(lib_path, "loaders")
os.makedirs(loaders_dest, exist_ok=True)
for pattern in [
    "libpixbufloader-png.so",
    "libpixbufloader-svg.so",
    "libpixbufloader-ico.so",
]:
    matches = glob.glob(
        os.path.join(brew_prefix, "lib", "gdk-pixbuf-2.0", "*", "loaders", pattern)
    )
    if matches:
        shutil.copy2(os.path.realpath(matches[0]), os.path.join(loaders_dest, pattern))
        bundle_lib(matches[0], lib_path, processed, search_paths)

# GIO modules
gio_dest = os.path.join(lib_path, "gio-modules")
os.makedirs(gio_dest, exist_ok=True)
for mod in glob.glob(os.path.join(brew_prefix, "lib", "gio", "modules", "*.so")):
    shutil.copy2(os.path.realpath(mod), os.path.join(gio_dest, os.path.basename(mod)))
    bundle_lib(mod, lib_path, processed, search_paths)

# 3. RPATH STRATEGY FAIL-SAFE
print("=== RPATH Deep Fix ===")
# Add search paths to the main executable
run(f'install_name_tool -add_rpath "@executable_path/lib" "{main_exe}"')

# Fix EVERY binary in the bundle
for root, dirs, files in os.walk(macos_path):
    for f in files:
        if (
            f.endswith(".dylib")
            or ".dylib." in f
            or f.endswith(".so")
            or f == "WiiUDownloader"
        ):
            p = os.path.join(root, f)
            # Ensure dylibs have @rpath ID
            if f.endswith(".dylib") or ".dylib." in f or f.endswith(".so"):
                run(f'install_name_tool -id "@rpath/{f}" "{p}"')
                # Add @loader_path to dylibs so they can find their neighbors
                run(f'install_name_tool -add_rpath "@loader_path" "{p}"')
                run(f'install_name_tool -add_rpath "@loader_path/.." "{p}"')

            # Change all dependencies to @rpath
            deps = run(f'otool -L "{p}"').stdout.split("\n")[1:]
            for line in deps:
                line = line.strip()
                if not line:
                    continue
                match = re.match(r"^(.+?)\s+\(", line)
                if not match:
                    continue
                old_path = match.group(1)
                if any(
                    old_path.startswith(prefix)
                    for prefix in ["/opt/homebrew", "/usr/local", "/opt/local"]
                ):
                    new_path = f"@rpath/{os.path.basename(old_path)}"
                    run(f'install_name_tool -change "{old_path}" "{new_path}" "{p}"')

            run(f'codesign --force --sign - "{p}"')

# 4. Resources
share_src = os.path.join(brew_prefix, "share")
dest_share = os.path.join(resources_path, "share")
for item in ["glib-2.0/schemas", "icons/Adwaita", "icons/hicolor", "themes/Adwaita"]:
    src = os.path.join(share_src, item)
    if os.path.exists(src):
        dst = os.path.join(dest_share, item)
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        (
            shutil.copytree(
                os.path.realpath(src),
                dst,
                symlinks=False,
                ignore_dangling_symlinks=True,
            )
            if os.path.isdir(src)
            else shutil.copy2(os.path.realpath(src), dst)
        )

# 5. GENERATE LOADERS CACHE
print("=== Generating Loaders Cache ===")
query_loaders = os.path.join(brew_prefix, "bin", "gdk-pixbuf-query-loaders")
if os.path.exists(query_loaders):
    bundled_loaders = glob.glob(os.path.join(loaders_dest, "*.so"))
    if bundled_loaders:
        res = subprocess.run(
            [query_loaders] + bundled_loaders, capture_output=True, text=True
        )
        if res.returncode == 0:
            # Place it in Resources instead of MacOS/lib to satisfy codesign
            with open(os.path.join(resources_path, "loaders.cache"), "w") as f:
                f.write(res.stdout)
            print("Created loaders.cache in Resources")
