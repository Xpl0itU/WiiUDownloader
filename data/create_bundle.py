import os
import shutil
import subprocess
import sys


# Helper to run system commands with check
def run_command(command, ignore_errors=False):
    print(f"Running: {command}")
    ret = os.system(command)
    if ret != 0:
        print(f"Error: Command failed with exit code {ret}")
        if not ignore_errors:
            sys.exit(1)


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

# Create bundle structure
if os.path.exists(app_bundle_path):
    shutil.rmtree(app_bundle_path)

os.makedirs(macos_path)
os.makedirs(resources_path)
os.makedirs(lib_path)

shutil.copy(info_plist_path, os.path.join(contents_path, "Info.plist"))
shutil.copy(executable_path, os.path.join(macos_path, "WiiUDownloader"))
os.chmod(os.path.join(macos_path, "WiiUDownloader"), 0o755)

# CRITICAL: Manually copy ALL GTK/GDK libraries FIRST
# dylibbundler is unreliable for these, so we force-copy them
print("Manually copying critical GTK/GDK libraries...")
critical_libs = [
    "libgtk-3.0.dylib",
    "libgtk-3.dylib",
    "libgdk-3.0.dylib",
    "libgdk-3.dylib",
    "libgobject-2.0.0.dylib",
    "libglib-2.0.0.dylib",
    "libintl.8.dylib",
    "libgio-2.0.0.dylib",
    "libgmodule-2.0.0.dylib",
    "libgthread-2.0.0.dylib",
    "libcairo.2.dylib",
    "libcairo-gobject.2.dylib",
    "libpango-1.0.0.dylib",
    "libpangocairo-1.0.0.dylib",
    "libpangoft2-1.0.0.dylib",
    "libatk-1.0.0.dylib",
    "libgdk_pixbuf-2.0.0.dylib",
    "libharfbuzz.0.dylib",
    "libfontconfig.1.dylib",
    "libfreetype.6.dylib",
    "libpixman-1.0.dylib",
    "libpng16.16.dylib",
    "libjpeg.8.dylib",
    "libtiff.6.dylib",
    "libffi.8.dylib",
    "libpcre2-8.0.dylib",
]

for lib in critical_libs:
    src = os.path.join(brew_prefix, "lib", lib)
    dst = os.path.join(lib_path, lib)
    if os.path.exists(src) and not os.path.exists(dst):
        print(f"  Copying {lib}...")
        shutil.copy2(src, dst)
        os.chmod(dst, 0o755)

# Now run dylibbundler to fix references and pull in any missing deps
print("Running dylibbundler to fix library references...")
run_command(
    f"dylibbundler -od -b -x {os.path.abspath(os.path.join(macos_path, 'WiiUDownloader'))} "
    f"-d {os.path.abspath(lib_path)} -p @executable_path/lib "
    f"-s {os.path.abspath(os.path.join(brew_prefix, 'lib'))}"
)

# Verify GTK is present
if not os.path.exists(os.path.join(lib_path, "libgtk-3.0.dylib")):
    print("FATAL: libgtk-3.0.dylib STILL missing after manual copy + dylibbundler!")
    print(f"Contents of {lib_path}:")
    if os.path.exists(lib_path):
        print(os.listdir(lib_path))
    sys.exit(1)

# Bundle Resources
gdk_pixbuf_lib = os.path.join(brew_prefix, "lib", "gdk-pixbuf-2.0")
dest_gdk_pixbuf = os.path.join(lib_path, "gdk-pixbuf-2.0")

if os.path.exists(gdk_pixbuf_lib):
    print(f"Copying GdkPixbuf loaders from {gdk_pixbuf_lib}...")
    safe_copy_directory(gdk_pixbuf_lib, dest_gdk_pixbuf)

    # Remove loaders.cache
    for root, dirs, files in os.walk(dest_gdk_pixbuf):
        for file in files:
            if file == "loaders.cache":
                print(f"Removing build-time cache: {os.path.join(root, file)}")
                os.remove(os.path.join(root, file))

    # Fix loader paths (soft-fail)
    print("Fixing GdkPixbuf loader paths...")
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
                            f"-s {os.path.abspath(os.path.join(brew_prefix, 'lib'))}",
                            ignore_errors=True,
                        )
                    except Exception as e:
                        print(f"Warning: Failed to process loader {so_path}: {e}")

# Copy icons/schemas/themes
share_src = os.path.join(brew_prefix, "share")
dest_share = os.path.join(resources_path, "share")
os.makedirs(dest_share, exist_ok=True)

icons_src = os.path.join(share_src, "icons")
if os.path.exists(icons_src):
    print("Copying icons...")
    safe_copy_directory(icons_src, os.path.join(dest_share, "icons"))

schemas_src = os.path.join(share_src, "glib-2.0", "schemas")
if os.path.exists(schemas_src):
    print("Copying GLib schemas...")
    safe_copy_directory(schemas_src, os.path.join(dest_share, "glib-2.0", "schemas"))

adwaita_src = os.path.join(share_src, "themes", "Adwaita")
if os.path.exists(adwaita_src):
    print("Copying Adwaita theme...")
    safe_copy_directory(adwaita_src, os.path.join(dest_share, "themes", "Adwaita"))

print("Bundle creation complete.")
