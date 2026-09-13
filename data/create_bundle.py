#!/usr/bin/env python3
import glob
import os
import re
import shutil
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from bundle_libs import bundle_lib, get_deps, verify_bundle
from bundle_paths import rewrite_binary
from theme_compat import install_adwaita_compat_aliases


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


# Paths
env_exec = os.environ.get("EXECUTABLE_PATH")
if env_exec and os.path.exists(env_exec):
    executable_path = env_exec
else:
    build_dir = os.path.join("cmd", "WiiUDownloader")
    build_env = os.environ.copy()
    build_env.setdefault("MACOSX_DEPLOYMENT_TARGET", MIN_MACOS_VERSION)
    try:
        subprocess.check_call(["go", "build", "-o", "main"], cwd=build_dir, env=build_env)
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

# 1. Recursive Bundle (Manual)
processed = set()
main_exe = os.path.join(macos_path, "WiiUDownloader")
for dep in get_deps(main_exe):
    bundle_lib(dep, lib_path, processed, search_paths)

# 2. Bundle Modules (GIO/Loaders)
loaders_dest = os.path.join(lib_path, "gdkpixbuf_loaders")
os.makedirs(loaders_dest, exist_ok=True)

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

# Ensure librsvg
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

cache_path = os.path.join(resources_path, "loaders.cache")
query_loaders = os.path.join(brew_prefix, "bin", "gdk-pixbuf-query-loaders")
bundled_loaders = sorted(glob.glob(os.path.join(loaders_dest, "*.so")))
if os.path.exists(query_loaders) and bundled_loaders:
    query_env = os.environ.copy()
    # Help the query tool find bundled dylibs while inspecting loaders
    dyld_paths = [loaders_dest, lib_path]
    existing_dyld = query_env.get("DYLD_LIBRARY_PATH", "")
    if existing_dyld:
        dyld_paths.insert(0, existing_dyld)
    query_env["DYLD_LIBRARY_PATH"] = ":".join(dyld_paths)
    res = subprocess.run([query_loaders] + bundled_loaders, capture_output=True, text=True, env=query_env)
    if res.returncode == 0 and res.stdout:
        cache_content = re.sub(
            r'"[^"]*?/gdkpixbuf_loaders/([^"]+\.so)"',
            r'"@executable_path/lib/gdkpixbuf_loaders/\1"',
            res.stdout,
        )
        with open(cache_path, "w") as f:
            f.write(cache_content)
        svg_present = "libpixbufloader_svg.so" in cache_content
        print(f"Created loaders.cache ({len(bundled_loaders)} loaders, svg={'OK' if svg_present else 'MISSING'}, paths=@executable_path)")
    else:
        open(cache_path, "w").close()
        print("Warning: gdk-pixbuf-query-loaders failed, created empty loaders.cache")
else:
    open(cache_path, "w").close()
    print("Warning: gdk-pixbuf-query-loaders not found or no loaders, created empty loaders.cache")

# GIO modules
gio_dest = os.path.join(lib_path, "gio-modules")
os.makedirs(gio_dest, exist_ok=True)
for mod in glob.glob(os.path.join(brew_prefix, "lib", "gio", "modules", "*.so")):
    shutil.copy2(os.path.realpath(mod), os.path.join(gio_dest, os.path.basename(mod)))
    bundle_lib(mod, lib_path, processed, search_paths)

# 3. Use dylibbundler to fix all paths and rpaths
print("=== Fixing dylib paths with dylibbundler ===")
dylibbundler_cmd = [
    "dylibbundler",
    "-d", lib_path,
    "-x", main_exe,
    "-n",
    "-o",
]
for sp in search_paths:
    dylibbundler_cmd.extend(["-s", sp])

print(f"Running: {' '.join(dylibbundler_cmd)}")
try:
    subprocess.run(dylibbundler_cmd, check=True, capture_output=True, text=True)
    print("dylibbundler completed successfully")
except subprocess.CalledProcessError as e:
    print("dylibbundler failed (continuing anyway; rewrite_binary() will fix paths):")
    print(f"  stdout: {e.stdout!r}")
    print(f"  stderr: {e.stderr!r}")

print("=== Rewriting load commands and rpaths ===")
rewrite_binary(main_exe, is_main_exe=True, run_fn=print)
for root, dirs, files in os.walk(macos_path):
    for f in files:
        if f.endswith(".dylib") or f.endswith(".so"):
            rewrite_binary(os.path.join(root, f), is_main_exe=False, run_fn=print)

# vtool only on main exe and dylibs (not .so which lack LC_BUILD_VERSION)
set_minimum_macos_version(main_exe)
for f in os.listdir(lib_path):
    if f.endswith(".dylib"):
        set_minimum_macos_version(os.path.join(lib_path, f))


verify_bundle(main_exe, macos_path)

# 5. Resources
share_src = os.path.join(brew_prefix, "share")
dest_share = os.path.join(resources_path, "share")
os.makedirs(dest_share, exist_ok=True)

dereference_items = ["icons/Adwaita", "icons/hicolor", "themes/Adwaita", "mime"]
no_dereference_items = ["glib-2.0/schemas"]

for item in dereference_items + no_dereference_items:
    src = os.path.join(share_src, item)
    if os.path.exists(src):
        src = os.path.realpath(src)
        dst = os.path.join(dest_share, item)
        os.makedirs(os.path.dirname(dst), exist_ok=True)
        print(f"Copying resource: {item}")
        if os.path.isdir(src):
            if os.path.exists(dst):
                shutil.rmtree(dst)
            deref = "--dereference" if item in dereference_items else ""
            run(f'tar {deref} -C "{os.path.dirname(src)}" -cf - "{os.path.basename(src)}" | tar -C "{os.path.dirname(dst)}" -xf -')
        else:
            shutil.copy2(src, dst)

# Fix icon theme
for icon_dir in glob.glob(os.path.join(dest_share, "icons", "*")):
    if os.path.isdir(icon_dir):
        index_theme = os.path.join(icon_dir, "index.theme")
        if os.path.exists(index_theme):
            with open(index_theme, "r") as f:
                content = f.read()
            content = content.replace("Hidden=true", "Hidden=false")
            with open(index_theme, "w") as f:
                f.write(content)
        install_adwaita_compat_aliases(icon_dir)
        cache_file = os.path.join(icon_dir, "icon-theme.cache")
        if os.path.exists(cache_file):
            os.remove(cache_file)
        update_cache = os.path.join(brew_prefix, "bin", "gtk-update-icon-cache")
        if os.path.exists(update_cache):
            run(f'"{update_cache}" -f "{icon_dir}"')
            print(f"Regenerated icon cache: {os.path.basename(icon_dir)}")

print("=== Bundle Complete ===")

# 6. Ad-hoc code signing (macOS SIP requires all dlopen'd code to be signed)
print("=== Code Signing ===")
# Sign libraries and loaders first (inside-out ordering)
for root, dirs, files in os.walk(macos_path):
    for f in sorted(files):
        if f.endswith(".so") or f.endswith(".dylib"):
            p = os.path.join(root, f)
            run(f'codesign --sign - --force --timestamp=none "{p}"')
# Sign the main executable
run(f'codesign --sign - --force --timestamp=none "{main_exe}"')
# Sign the entire bundle (catches any remaining unsigned code)
run(f'codesign --sign - --force --deep --timestamp=none "{app_bundle_path}"')
print("Code signing complete")
