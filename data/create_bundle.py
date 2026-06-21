#!/usr/bin/env python3
import glob
import os
import re
import shutil
import subprocess
import sys


MIN_MACOS_VERSION = os.environ.get("MACOSX_DEPLOYMENT_TARGET", "11.0")


def run(cmd):
    print(f"$ {cmd}")
    result = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"STDOUT: {result.stdout}")
        print(f"STDERR: {result.stderr}")
    return result


def set_minimum_macos_version(path):
    parts = MIN_MACOS_VERSION.split(".")
    minos = ".".join((parts + ["0", "0"])[:2])
    tmp_path = f"{path}.vtool.tmp"
    vtool_cmd = ["vtool", "-set-build-version", "macos", minos, minos, "-replace", "-output", tmp_path, path]
    try:
        print(f"Running: {' '.join(vtool_cmd)}")
        res = subprocess.run(vtool_cmd, capture_output=True, text=True, timeout=15)
    except subprocess.TimeoutExpired:
        print(f"Warning: vtool timed out while processing {path}; skipping version bump")
        return
    if res.returncode != 0:
        print(f"vtool failed for {path}. STDOUT:\n{res.stdout}\nSTDERR:\n{res.stderr}")
        print(f"Warning: could not set minimum macOS version for {path}")
        return
    try:
        os.replace(tmp_path, path)
    except Exception as e:
        print(f"Warning: failed to move vtool output for {path}: {e}")
        try:
            if os.path.exists(tmp_path):
                os.remove(tmp_path)
        except:
            pass


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
env_exec = os.environ.get("EXECUTABLE_PATH")
if env_exec and os.path.exists(env_exec):
    executable_path = env_exec
else:
    candidates = [
        "main",
        os.path.join("cmd", "WiiUDownloader", "main"),
    ]
    executable_path = None
    for c in candidates:
        if os.path.exists(c):
            executable_path = c
            break
    if not executable_path:
        try:
            build_dir = os.path.join("cmd", "WiiUDownloader")
            build_env = os.environ.copy()
            build_env.setdefault("MACOSX_DEPLOYMENT_TARGET", MIN_MACOS_VERSION)
            subprocess.check_call(
                ["go", "build", "-o", "main"], cwd=build_dir, env=build_env
            )
            executable_path = os.path.join(build_dir, "main")
        except subprocess.CalledProcessError as e:
            print(f"Error building executable: {e}")
            sys.exit(1)
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
# GdkPixbuf loaders — copy ALL .so files from the loaders directory
# Use flat dir without dots/version to avoid codesign bundle detection
loaders_dest = os.path.join(lib_path, "gdkpixbuf_loaders")
os.makedirs(loaders_dest, exist_ok=True)

# Find the gdk-pixbuf loaders directory (may be versioned path under brew lib)
loaders_src_dir = None
for candidate in glob.glob(os.path.join(brew_prefix, "lib", "gdk-pixbuf-2.0", "*", "loaders")):
    if os.path.isdir(candidate):
        loaders_src_dir = candidate
        break

if loaders_src_dir:
    print(f"Copying pixbuf loaders from {loaders_src_dir}")
    for so_file in glob.glob(os.path.join(loaders_src_dir, "*.so")):
        real = os.path.realpath(so_file)
        dest = os.path.join(loaders_dest, os.path.basename(so_file))
        try:
            shutil.copy2(real, dest)
            print(f"  Copied: {os.path.basename(so_file)}")
            bundle_lib(so_file, lib_path, processed, search_paths)
        except Exception as e:
            print(f"  Warning: failed to copy {os.path.basename(so_file)}: {e}")
else:
    print("Warning: gdk-pixbuf loaders directory not found!")

# Ensure librsvg is bundled for SVG loader (version-agnostic discovery)
# Homebrew lib files are often symlinks — use realpath to follow them
rsvg_lib = None
for candidate in glob.glob(os.path.join(brew_prefix, "lib", "librsvg-*.dylib")):
    rsvg_real = os.path.realpath(candidate)
    if os.path.exists(rsvg_real):
        rsvg_lib = rsvg_real
        break
if not rsvg_lib:
    for candidate in glob.glob(os.path.join(brew_prefix, "opt", "librsvg", "lib", "librsvg-*.dylib")):
        rsvg_real = os.path.realpath(candidate)
        if os.path.exists(rsvg_real):
            rsvg_lib = rsvg_real
            break
if rsvg_lib:
    bundle_lib(rsvg_lib, lib_path, processed, search_paths)
    shutil.copy2(os.path.realpath(rsvg_lib), os.path.join(loaders_dest, os.path.basename(rsvg_lib)))
    print(f"Copied {os.path.basename(rsvg_lib)} into gdkpixbuf_loaders")
else:
    print("Warning: librsvg not found for SVG loader")

# Generate loaders.cache
# At runtime, main.go patches the paths to the actual bundle location
cache_path = os.path.join(resources_path, "loaders.cache")
query_loaders = os.path.join(brew_prefix, "bin", "gdk-pixbuf-query-loaders")
if os.path.exists(query_loaders):
    bundled_loaders = glob.glob(os.path.join(loaders_dest, "*.so"))
    if bundled_loaders:
        res = subprocess.run([query_loaders] + bundled_loaders, capture_output=True, text=True)
        if res.returncode == 0 and res.stdout:
            with open(cache_path, "w") as f:
                f.write(res.stdout)
            print(f"Created loaders.cache ({len(bundled_loaders)} entries)")
        else:
            open(cache_path, "w").close()
            print("Empty loaders.cache (query failed, runtime will populate)")
else:
    open(cache_path, "w").close()
    print("gdk-pixbuf-query-loaders not found, empty cache")

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

            set_minimum_macos_version(p)

# 4. Resources
share_src = os.path.join(brew_prefix, "share")
dest_share = os.path.join(resources_path, "share")
os.makedirs(dest_share, exist_ok=True)

# List of essential resources to copy from Homebrew's share
for item in ["glib-2.0/schemas", "icons/Adwaita", "icons/hicolor", "themes/Adwaita", "mime"]:
    src = os.path.join(share_src, item)
    if os.path.exists(src):
        src = os.path.realpath(src)
        dst = os.path.join(dest_share, item)
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        print(f"Copying resource: {item}")
        # Use a more robust copy approach to follow symlinks to their REAL source
        # and skip broken links that homebrew sometimes leaves behind.
        if os.path.isdir(src):
            if os.path.exists(dst):
                shutil.rmtree(dst)
            # Use tar to copy — handles broken symlinks, preserves structure
            run(f'tar -C "{os.path.dirname(src)}" -cf - "{os.path.basename(src)}" | tar -C "{os.path.dirname(dst)}" -xf -')
        else:
            shutil.copy2(src, dst)
