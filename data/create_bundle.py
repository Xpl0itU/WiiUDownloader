import os
import shutil
import subprocess
import sys
import glob


def run_command(command, ignore_errors=False):
    print(f"Running: {command}")
    ret = os.system(command)
    if ret != 0:
        print(f"Error: Command failed with exit code {ret}")
        if not ignore_errors:
            sys.exit(1)
    return ret == 0


def copy_lib(src, lib_path):
    """Copy a library file, following symlinks."""
    lib_name = os.path.basename(src)
    dst = os.path.join(lib_path, lib_name)
    if os.path.exists(dst):
        return False
    real_src = os.path.realpath(src)
    if os.path.exists(real_src):
        print(f"  Copying {lib_name}...")
        shutil.copy2(real_src, dst)
        os.chmod(dst, 0o755)
        return True
    return False


def safe_copy_file(src, dst):
    """Copy a single file, following symlinks."""
    try:
        if os.path.islink(src):
            real = os.path.realpath(src)
            if os.path.exists(real):
                shutil.copy2(real, dst)
                return True
        elif os.path.isfile(src):
            shutil.copy2(src, dst)
            return True
    except Exception as e:
        print(f"Warning: Failed to copy {src}: {e}")
    return False


# Paths
executable_path = "main"
info_plist_path = "data/Info.plist"
app_bundle_path = "out/WiiUDownloader.app"
contents_path = os.path.abspath(os.path.join(app_bundle_path, "Contents"))
macos_path = os.path.join(contents_path, "MacOS")
resources_path = os.path.join(contents_path, "Resources")
lib_path = os.path.join(macos_path, "lib")

# Get Homebrew prefix
try:
    brew_prefix = subprocess.check_output(["brew", "--prefix"]).decode("utf-8").strip()
except:
    brew_prefix = "/opt/homebrew"

brew_lib = os.path.join(brew_prefix, "lib")

# Create bundle structure
if os.path.exists(app_bundle_path):
    shutil.rmtree(app_bundle_path)

os.makedirs(macos_path)
os.makedirs(resources_path)
os.makedirs(lib_path)

shutil.copy(info_plist_path, os.path.join(contents_path, "Info.plist"))
shutil.copy(executable_path, os.path.join(macos_path, "WiiUDownloader"))
os.chmod(os.path.join(macos_path, "WiiUDownloader"), 0o755)

# 1. Discover ALL search paths (including opt/*/lib)
search_paths = [brew_lib]
opt_dir = os.path.join(brew_prefix, "opt")
if os.path.exists(opt_dir):
    for item in os.listdir(opt_dir):
        p = os.path.join(opt_dir, item, "lib")
        if os.path.isdir(p):
            search_paths.append(p)

search_args = " ".join([f"-s {p}" for p in search_paths])

# 2. Run dylibbundler on main executable
print("Bundling libraries...")
run_command(
    f"dylibbundler -od -b -x {os.path.abspath(os.path.join(macos_path, 'WiiUDownloader'))} "
    f"-d {os.path.abspath(lib_path)} -p @executable_path/lib {search_args}"
)

# 3. Safety net: manually copy critical libs if missing
for lib_name in ["libgtk-3.0.dylib", "libgdk-3.0.dylib", "librsvg-2.2.dylib"]:
    if not any(f == lib_name for f in os.listdir(lib_path)):
        for p in search_paths:
            candidate = os.path.join(p, lib_name)
            if os.path.exists(candidate):
                copy_lib(candidate, lib_path)
                run_command(
                    f"dylibbundler -od -b -x {os.path.join(lib_path, lib_name)} -d {lib_path} -p @executable_path/lib {search_args}",
                    ignore_errors=True,
                )
                break

# 4. Bundle GdkPixbuf Loaders (only essential ones)
loaders_version_dir = None
for ver in ["2.10.0", "2.10.1", "2.10.2"]:
    candidate = os.path.join(brew_prefix, "lib", "gdk-pixbuf-2.0", ver, "loaders")
    if os.path.isdir(candidate):
        loaders_version_dir = os.path.join(brew_prefix, "lib", "gdk-pixbuf-2.0", ver)
        break

if loaders_version_dir:
    dest_loaders_version = os.path.join(
        lib_path, "gdk-pixbuf-2.0", os.path.basename(loaders_version_dir)
    )
    dest_loaders = os.path.join(dest_loaders_version, "loaders")
    os.makedirs(dest_loaders, exist_ok=True)

    # Copy ONLY essential loaders (avoid problematic ones)
    essential_loaders = [
        "libpixbufloader-png.so",
        "libpixbufloader-svg.so",
        "libpixbufloader-ico.so",
        "libpixbufloader-jpeg.so",
    ]
    src_loaders = os.path.join(loaders_version_dir, "loaders")

    for loader in essential_loaders:
        src = os.path.join(src_loaders, loader)
        dst = os.path.join(dest_loaders, loader)
        if os.path.exists(src):
            print(f"Copying loader: {loader}")
            safe_copy_file(src, dst)
            os.chmod(dst, 0o755)
            # Fix loader dependencies
            run_command(
                f"dylibbundler -od -b -x {dst} -d {lib_path} -p @executable_path/lib {search_args}",
                ignore_errors=True,
            )

    # CRITICAL: Generate a NEW loaders.cache for bundled loaders
    # This tells GdkPixbuf where to find them
    loaders_cache = os.path.join(dest_loaders_version, "loaders.cache")
    query_loaders = os.path.join(brew_prefix, "bin", "gdk-pixbuf-query-loaders")
    if os.path.exists(query_loaders):
        print("Generating loaders.cache...")
        # List the bundled loaders
        bundled_loaders = glob.glob(os.path.join(dest_loaders, "*.so"))
        if bundled_loaders:
            loader_list = " ".join(bundled_loaders)
            # Generate cache content
            result = subprocess.run(
                [query_loaders] + bundled_loaders, capture_output=True, text=True
            )
            if result.returncode == 0:
                # Write the cache, but replace absolute paths with relative ones that use @executable_path
                cache_content = result.stdout
                # The cache contains absolute paths to loaders. We need to adjust them.
                # Since we can't use @executable_path in the cache, we leave them as-is.
                # The GDK_PIXBUF_MODULE_DIR env var will point to the loaders dir at runtime.
                # But gdk-pixbuf-query-loaders outputs paths relative to where it was run.
                # We'll write the bundled loader paths.
                with open(loaders_cache, "w") as f:
                    f.write(cache_content)
                print(f"Created loaders.cache with {len(bundled_loaders)} loaders")
            else:
                print(f"Warning: gdk-pixbuf-query-loaders failed: {result.stderr}")

# 5. Copy Resources
share_src = os.path.join(brew_prefix, "share")
dest_share = os.path.join(resources_path, "share")
os.makedirs(dest_share, exist_ok=True)

# Schemas - copy files individually to avoid broken symlinks
schemas_src = os.path.join(share_src, "glib-2.0", "schemas")
schemas_dst = os.path.join(dest_share, "glib-2.0", "schemas")
if os.path.exists(schemas_src):
    os.makedirs(schemas_dst, exist_ok=True)
    for f in os.listdir(schemas_src):
        src = os.path.join(schemas_src, f)
        dst = os.path.join(schemas_dst, f)
        safe_copy_file(src, dst)

# Icons (Adwaita primarily)
icons_adwaita = os.path.join(share_src, "icons", "Adwaita")
if os.path.exists(icons_adwaita):
    print("Copying Adwaita icons...")
    dst_icons = os.path.join(dest_share, "icons", "Adwaita")
    shutil.copytree(
        icons_adwaita, dst_icons, symlinks=False, ignore_dangling_symlinks=True
    )

# Hicolor fallback
icons_hicolor = os.path.join(share_src, "icons", "hicolor")
if os.path.exists(icons_hicolor):
    print("Copying hicolor icons...")
    dst_icons = os.path.join(dest_share, "icons", "hicolor")
    shutil.copytree(
        icons_hicolor, dst_icons, symlinks=False, ignore_dangling_symlinks=True
    )

print("\n=== BUNDLE COMPLETE ===")
print(f"Lib directory: {len(os.listdir(lib_path))} items")
