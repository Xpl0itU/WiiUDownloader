import os
import shutil
import subprocess
import sys
import glob


# Helper to run system commands with check
def run_command(command, ignore_errors=False):
    print(f"Running: {command}")
    ret = os.system(command)
    if ret != 0:
        print(f"Error: Command failed with exit code {ret}")
        if not ignore_errors:
            sys.exit(1)
    return ret == 0


# Helper for robust copying
def safe_copy_directory(src, dst):
    """Recursively copies files from src to dst, ignoring broken symlinks."""
    if not os.path.exists(dst):
        os.makedirs(dst)

    for item in os.listdir(src):
        s = os.path.join(src, item)
        d = os.path.join(dst, item)

        try:
            if os.path.islink(s):
                link_target = os.readlink(s)
                if not os.path.isabs(link_target):
                    link_target = os.path.join(os.path.dirname(s), link_target)

                if os.path.exists(link_target):
                    if os.path.isdir(s):
                        safe_copy_directory(s, d)
                    else:
                        shutil.copy2(s, d)
                else:
                    print(f"Warning: Skipping broken symlink {s}")
            elif os.path.isdir(s):
                safe_copy_directory(s, d)
            else:
                shutil.copy2(s, d)
        except Exception as e:
            print(f"Warning: Failed to copy {s}: {e}")


def copy_lib(src, dst):
    """Copy a library file, following symlinks to get real content."""
    if os.path.exists(src):
        # Resolve symlinks to get the actual file
        real_src = os.path.realpath(src)
        if os.path.exists(real_src):
            print(f"  Copying {os.path.basename(src)} (from {real_src})...")
            shutil.copy2(real_src, dst)
            os.chmod(dst, 0o755)
            return True
    return False


# Set the paths
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
except Exception as e:
    print("Error getting brew prefix:", e)
    brew_prefix = "/opt/homebrew"

brew_lib = os.path.join(brew_prefix, "lib")
print(f"Using Homebrew lib path: {brew_lib}")

# Create bundle structure
if os.path.exists(app_bundle_path):
    shutil.rmtree(app_bundle_path)

os.makedirs(macos_path)
os.makedirs(resources_path)
os.makedirs(lib_path)

shutil.copy(info_plist_path, os.path.join(contents_path, "Info.plist"))
shutil.copy(executable_path, os.path.join(macos_path, "WiiUDownloader"))
os.chmod(os.path.join(macos_path, "WiiUDownloader"), 0o755)

# CRITICAL: Find and copy ALL GTK-related libraries by pattern
print("Finding and copying GTK/GDK libraries...")
lib_patterns = [
    "libgtk-3*.dylib",
    "libgdk-3*.dylib",
    "libgobject-2.0*.dylib",
    "libglib-2.0*.dylib",
    "libgio-2.0*.dylib",
    "libgmodule-2.0*.dylib",
    "libgthread-2.0*.dylib",
    "libintl*.dylib",
    "libcairo*.dylib",
    "libpango*.dylib",
    "libatk-1.0*.dylib",
    "libgdk_pixbuf-2.0*.dylib",
    "libharfbuzz*.dylib",
    "libfontconfig*.dylib",
    "libfreetype*.dylib",
    "libpixman-1*.dylib",
    "libpng*.dylib",
    "libjpeg*.dylib",
    "libtiff*.dylib",
    "libffi*.dylib",
    "libpcre2*.dylib",
    "libfribidi*.dylib",
    "libepoxy*.dylib",
    "libgraphite2*.dylib",
    "liblzma*.dylib",
    "libz*.dylib",
    "libbz2*.dylib",
    "libX*.dylib",
    "libxcb*.dylib",
]

copied_libs = set()
for pattern in lib_patterns:
    for lib_file in glob.glob(os.path.join(brew_lib, pattern)):
        lib_name = os.path.basename(lib_file)
        dst = os.path.join(lib_path, lib_name)
        if lib_name not in copied_libs and not os.path.exists(dst):
            if copy_lib(lib_file, dst):
                copied_libs.add(lib_name)

print(f"Copied {len(copied_libs)} libraries")

# Verify GTK exists - check for any libgtk file
gtk_files = glob.glob(os.path.join(lib_path, "libgtk-3*.dylib"))
print(f"GTK files in bundle: {gtk_files}")
if not gtk_files:
    print("FATAL: No libgtk-3 libraries found in bundle!")
    print(f"Available GTK in brew: {glob.glob(os.path.join(brew_lib, 'libgtk*'))}")
    sys.exit(1)

# Now run dylibbundler to fix references and pull in any missing deps
print("Running dylibbundler to fix library references...")
run_command(
    f"dylibbundler -od -b -x {os.path.abspath(os.path.join(macos_path, 'WiiUDownloader'))} "
    f"-d {os.path.abspath(lib_path)} -p @executable_path/lib "
    f"-s {os.path.abspath(brew_lib)}"
)

# Final verification
print("Final library check:")
for name in ["libgtk-3", "libgdk-3", "libglib-2.0", "libgio-2.0"]:
    found = glob.glob(os.path.join(lib_path, f"{name}*.dylib"))
    print(f"  {name}: {found}")

# Bundle GdkPixbuf loaders
gdk_pixbuf_lib = os.path.join(brew_prefix, "lib", "gdk-pixbuf-2.0")
dest_gdk_pixbuf = os.path.join(lib_path, "gdk-pixbuf-2.0")

if os.path.exists(gdk_pixbuf_lib):
    print(f"Copying GdkPixbuf loaders from {gdk_pixbuf_lib}...")
    safe_copy_directory(gdk_pixbuf_lib, dest_gdk_pixbuf)

    # Remove loaders.cache
    for root, dirs, files in os.walk(dest_gdk_pixbuf):
        for file in files:
            if file == "loaders.cache":
                os.remove(os.path.join(root, file))

    # Fix loader paths (soft-fail)
    for root, dirs, files in os.walk(dest_gdk_pixbuf):
        for file in files:
            if file.endswith(".so"):
                so_path = os.path.join(root, file)
                if os.path.exists(so_path) and not os.path.islink(so_path):
                    try:
                        os.chmod(so_path, 0o755)
                        run_command(
                            f"dylibbundler -od -b -x {os.path.abspath(so_path)} "
                            f"-d {os.path.abspath(lib_path)} -p @executable_path/lib "
                            f"-s {os.path.abspath(brew_lib)}",
                            ignore_errors=True,
                        )
                    except Exception as e:
                        print(f"Warning: {e}")

# Copy resources
share_src = os.path.join(brew_prefix, "share")
dest_share = os.path.join(resources_path, "share")
os.makedirs(dest_share, exist_ok=True)

for src_name, dst_name in [
    ("icons", "icons"),
    ("glib-2.0/schemas", "glib-2.0/schemas"),
    ("themes/Adwaita", "themes/Adwaita"),
]:
    src = os.path.join(share_src, src_name)
    dst = os.path.join(dest_share, dst_name)
    if os.path.exists(src):
        print(f"Copying {src_name}...")
        safe_copy_directory(src, dst)

print("Bundle creation complete.")
print(f"Library directory contents ({len(os.listdir(lib_path))} files):")
for f in sorted(os.listdir(lib_path))[:20]:
    print(f"  {f}")
if len(os.listdir(lib_path)) > 20:
    print(f"  ... and {len(os.listdir(lib_path)) - 20} more")
