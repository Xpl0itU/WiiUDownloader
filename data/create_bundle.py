import os
import shutil
import subprocess
import sys


# Helper to run system commands with check
def run_command(command):
    print(f"Running: {command}")
    ret = os.system(command)
    if ret != 0:
        print(f"Error: Command failed with exit code {ret}")
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
                # If absolute, check existence. If relative, resolve it (approximate)
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


# Set the paths to the executable and Info.plist
executable_path = "main"
info_plist_path = "data/Info.plist"

# Set the path to the .app bundle
app_bundle_path = "out/WiiUDownloader.app"
contents_path = os.path.join(app_bundle_path, "Contents")
macos_path = os.path.join(contents_path, "MacOS")
resources_path = os.path.join(contents_path, "Resources")
lib_path = os.path.join(macos_path, "lib")

# Get Homebrew prefix
try:
    brew_prefix = subprocess.check_output(["brew", "--prefix"]).decode("utf-8").strip()
except Exception as e:
    print("Error getting brew prefix:", e)
    brew_prefix = "/opt/homebrew"  # Fallback

# Create the .app bundle structure
if os.path.exists(app_bundle_path):
    shutil.rmtree(app_bundle_path)

os.makedirs(macos_path)
os.makedirs(resources_path)

shutil.copy(info_plist_path, os.path.join(contents_path, "Info.plist"))
shutil.copy(executable_path, os.path.join(macos_path, "WiiUDownloader"))

# Fix permissions for main executable
os.chmod(os.path.join(macos_path, "WiiUDownloader"), 0o755)

# Run dylibbundler on the main executable
# -s: add search path for libraries
run_command(
    f"dylibbundler -od -b -x {os.path.join(macos_path, 'WiiUDownloader')} -d {lib_path} -p @executable_path/lib -s {os.path.join(brew_prefix, 'lib')}"
)

# Verify critical libraries exist
critical_libs = [
    "libgtk-3.0.dylib",
    "libgdk-3.0.dylib",
    "libgobject-2.0.0.dylib",
    "libglib-2.0.0.dylib",
    "libintl.8.dylib",
]
for lib in critical_libs:
    bundled_lib_path = os.path.join(lib_path, lib)
    if not os.path.exists(bundled_lib_path):
        print(
            f"Warning: {lib} missing from bundle after dylibbundler. Attempting manual recovery..."
        )
        # Try to find it in Homebrew
        src_lib = os.path.join(brew_prefix, "lib", lib)
        if os.path.exists(src_lib):
            print(f"Copying {lib} from {src_lib}...")
            shutil.copy2(src_lib, bundled_lib_path)
            os.chmod(bundled_lib_path, 0o755)
            # Fix dependencies of this manually copied lib
            run_command(
                f"dylibbundler -od -b -x {bundled_lib_path} -d {lib_path} -p @executable_path/lib -s {os.path.join(brew_prefix, 'lib')}"
            )
        else:
            print(f"Error: Could not find {lib} in {src_lib} to recover.")
            sys.exit(1)

# Double check
if not os.path.exists(os.path.join(lib_path, "libgtk-3.0.dylib")):
    print("Error: libgtk-3.0.dylib is strictly missing. Build failed.")
    sys.exit(1)

# --- Bundle Resources ---

# 1. GdkPixbuf Loaders
gdk_pixbuf_lib = os.path.join(brew_prefix, "lib", "gdk-pixbuf-2.0")
dest_gdk_pixbuf = os.path.join(lib_path, "gdk-pixbuf-2.0")

if os.path.exists(gdk_pixbuf_lib):
    print(f"Copying GdkPixbuf loaders from {gdk_pixbuf_lib}...")
    safe_copy_directory(gdk_pixbuf_lib, dest_gdk_pixbuf)

    # Fix paths in loaders (.so files)
    print("Fixing GdkPixbuf loader paths...")
    for root, dirs, files in os.walk(dest_gdk_pixbuf):
        for file in files:
            if file.endswith(".so"):
                so_path = os.path.join(root, file)
                if os.path.exists(so_path):
                    # Ensure write permissions
                    os.chmod(so_path, 0o755)
                    run_command(
                        f"dylibbundler -od -b -x {so_path} -d {lib_path} -p @executable_path/lib"
                    )
else:
    print("Warning: GdkPixbuf lib directory not found.")

# 2. Icons / Share
share_src = os.path.join(brew_prefix, "share")
dest_share = os.path.join(resources_path, "share")
os.makedirs(dest_share, exist_ok=True)

# Icons
icons_src = os.path.join(share_src, "icons")
if os.path.exists(icons_src):
    print("Copying icons...")
    safe_copy_directory(icons_src, os.path.join(dest_share, "icons"))

# GLib Schemas
schemas_src = os.path.join(share_src, "glib-2.0", "schemas")
if os.path.exists(schemas_src):
    print("Copying GLib schemas...")
    safe_copy_directory(schemas_src, os.path.join(dest_share, "glib-2.0", "schemas"))
else:
    print("Warning: GLib schemas not found.")

# Adwaita Theme
adwaita_src = os.path.join(share_src, "themes", "Adwaita")
if os.path.exists(adwaita_src):
    print("Copying Adwaita theme...")
    safe_copy_directory(adwaita_src, os.path.join(dest_share, "themes", "Adwaita"))

print("Bundle creation complete.")
