import os
import shutil
import subprocess

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

# Run dylibbundler on the main executable
os.system(
    f"dylibbundler -od -b -x {os.path.join(macos_path, 'WiiUDownloader')} -d {lib_path} -p @executable_path/lib"
)

# --- Bundle Resources ---

# 1. GdkPixbuf Loaders
gdk_pixbuf_lib = os.path.join(brew_prefix, "lib", "gdk-pixbuf-2.0")
dest_gdk_pixbuf = os.path.join(lib_path, "gdk-pixbuf-2.0")

if os.path.exists(gdk_pixbuf_lib):
    print(f"Copying GdkPixbuf loaders from {gdk_pixbuf_lib}...")
    shutil.copytree(gdk_pixbuf_lib, dest_gdk_pixbuf, symlinks=True)

    # Fix paths in loaders (.so files)
    # We need to run dylibbundler on each .so to ensure they reference the bundled libs
    print("Fixing GdkPixbuf loader paths...")
    for root, dirs, files in os.walk(dest_gdk_pixbuf):
        for file in files:
            if file.endswith(".so"):
                so_path = os.path.join(root, file)
                # We use the SAME lib dir so they share the already bundled dylibs
                os.system(
                    f"dylibbundler -od -b -x {so_path} -d {lib_path} -p @executable_path/lib"
                )
else:
    print("Warning: GdkPixbuf lib directory not found.")

# 2. Icons / Share
# Copy share/icons and share/glib-2.0/schemas
share_src = os.path.join(brew_prefix, "share")
dest_share = os.path.join(resources_path, "share")
os.makedirs(dest_share, exist_ok=True)

# Icons
icons_src = os.path.join(share_src, "icons")
if os.path.exists(icons_src):
    print("Copying icons...")
    shutil.copytree(icons_src, os.path.join(dest_share, "icons"), symlinks=True)

# GLib Schemas
schemas_src = os.path.join(share_src, "glib-2.0", "schemas")
if os.path.exists(schemas_src):
    print("Copying GLib schemas...")
    shutil.copytree(
        schemas_src, os.path.join(dest_share, "glib-2.0", "schemas"), symlinks=True
    )
else:
    print("Warning: GLib schemas not found.")

# Adwaita Theme (optional but good for minimizing warnings)
# /opt/homebrew/share/themes/Adwaita
adwaita_src = os.path.join(share_src, "themes", "Adwaita")
if os.path.exists(adwaita_src):
    print("Copying Adwaita theme...")
    shutil.copytree(
        adwaita_src, os.path.join(dest_share, "themes", "Adwaita"), symlinks=True
    )

print("Bundle creation complete.")
