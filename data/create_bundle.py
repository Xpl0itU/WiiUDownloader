import os
import shutil
import subprocess
import sys
import glob


def run_command(command):
    print(f"Running: {command}")
    ret = os.system(command)
    if ret != 0:
        print(f"Error: Command failed with exit code {ret}")
        sys.exit(1)


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
print(f"Homebrew lib: {brew_lib}")

# Create bundle structure
if os.path.exists(app_bundle_path):
    shutil.rmtree(app_bundle_path)

os.makedirs(macos_path)
os.makedirs(resources_path)
os.makedirs(lib_path)

shutil.copy(info_plist_path, os.path.join(contents_path, "Info.plist"))
shutil.copy(executable_path, os.path.join(macos_path, "WiiUDownloader"))
os.chmod(os.path.join(macos_path, "WiiUDownloader"), 0o755)

# Find ALL Homebrew library search paths (opt/*/lib directories)
search_paths = [brew_lib]
opt_dir = os.path.join(brew_prefix, "opt")
if os.path.exists(opt_dir):
    for item in os.listdir(opt_dir):
        lib_dir = os.path.join(opt_dir, item, "lib")
        if os.path.isdir(lib_dir):
            search_paths.append(lib_dir)

print(f"Found {len(search_paths)} library search paths")

# Build search path argument for dylibbundler
search_args = " ".join(
    [f"-s {p}" for p in search_paths[:20]]
)  # Limit to 20 to avoid command line length issues

# Run dylibbundler with ALL search paths
print("Running dylibbundler with comprehensive search paths...")
run_command(
    f"dylibbundler -od -b -x {os.path.abspath(os.path.join(macos_path, 'WiiUDownloader'))} "
    f"-d {os.path.abspath(lib_path)} -p @executable_path/lib {search_args}"
)

# Check what dylibbundler copied
print(f"\nAfter dylibbundler - lib contains {len(os.listdir(lib_path))} files")
for f in sorted(os.listdir(lib_path)):
    print(f"  {f}")

# Verify GTK is present
gtk_present = any(f.startswith("libgtk-3") for f in os.listdir(lib_path))
gdk_present = any(f.startswith("libgdk-3") for f in os.listdir(lib_path))

if not gtk_present or not gdk_present:
    print(f"\nGTK present: {gtk_present}, GDK present: {gdk_present}")
    print("dylibbundler missed GTK/GDK - attempting manual copy...")

    # Manually copy from all search paths
    for search_path in search_paths:
        for pattern in ["libgtk-3*.dylib", "libgdk-3*.dylib"]:
            for lib_file in glob.glob(os.path.join(search_path, pattern)):
                copy_lib(lib_file, lib_path)

    # Run dylibbundler again to fix dependencies of manually copied libs
    run_command(
        f"dylibbundler -od -b -x {os.path.abspath(os.path.join(macos_path, 'WiiUDownloader'))} "
        f"-d {os.path.abspath(lib_path)} -p @executable_path/lib {search_args}"
    )

# Final verification
print("\n=== FINAL VERIFICATION ===")
lib_files = os.listdir(lib_path)
print(f"Total files in lib: {len(lib_files)}")

gtk_files = [f for f in lib_files if "gtk" in f.lower()]
gdk_files = [f for f in lib_files if "gdk" in f.lower()]
print(f"GTK files: {gtk_files}")
print(f"GDK files: {gdk_files}")

if not any(f.startswith("libgtk-3") for f in lib_files):
    print("FATAL: libgtk-3 not in bundle!")
    sys.exit(1)

# Copy schemas (but NOT with recursive directory copy that might break things)
schemas_dst = os.path.join(resources_path, "share", "glib-2.0", "schemas")
os.makedirs(schemas_dst, exist_ok=True)

schemas_src = os.path.join(brew_prefix, "share", "glib-2.0", "schemas")
if os.path.exists(schemas_src):
    print("Copying GLib schemas...")
    for f in os.listdir(schemas_src):
        src = os.path.join(schemas_src, f)
        dst = os.path.join(schemas_dst, f)
        if os.path.isfile(src) and not os.path.islink(src):
            try:
                shutil.copy2(src, dst)
            except:
                pass
        elif os.path.islink(src):
            target = os.path.realpath(src)
            if os.path.exists(target):
                try:
                    shutil.copy2(target, dst)
                except:
                    pass

print("\n=== BUNDLE COMPLETE ===")
print(f"Final lib directory contents ({len(os.listdir(lib_path))} files):")
for f in sorted(os.listdir(lib_path)):
    print(f"  {f}")
