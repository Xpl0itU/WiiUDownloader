import os
import shutil

# Set the paths to the executable and Info.plist
executable_path = 'build/WiiUDownloader'
info_plist_path = 'bundling/macOS/Info.plist'

# Set the path to the .app bundle
app_bundle_path = 'out/WiiUDownloader.app'

# Create the .app bundle
os.makedirs(os.path.join(app_bundle_path, 'Contents', 'MacOS'))
shutil.copy(info_plist_path, os.path.join(app_bundle_path, 'Contents', 'Info.plist'))
shutil.copy(executable_path, os.path.join(app_bundle_path, 'Contents', 'MacOS', 'WiiUDownloader'))

# Run dylibbundler
os.system(f"dylibbundler -od -b -x {os.path.join(app_bundle_path, 'Contents', 'MacOS', 'WiiUDownloader')} -d {os.path.join(app_bundle_path, 'Contents', 'MacOS', 'lib')} -p @executable_path/lib")