import os
import shutil
import subprocess

# Set the paths to the executable and Info.plist
executable_path = 'build/WiiUDownloader'
info_plist_path = 'bundling/macOS/Info.plist'

# Set the path to the .app bundle
app_bundle_path = 'out/WiiUDownloader.app'

# Create the .app bundle
os.makedirs(os.path.join(app_bundle_path, 'Contents', 'MacOS'))
shutil.copy(info_plist_path, os.path.join(app_bundle_path, 'Contents', 'Info.plist'))
shutil.copy(executable_path, os.path.join(app_bundle_path, 'Contents', 'MacOS', 'WiiUDownloader'))

# Create a set to store the processed dependencies
processed_dependencies = set()

# Create a queue to store the dependencies to process
dependencies_queue = [executable_path]

# Process the dependencies
while dependencies_queue:
    # Get the next dependency
    dependency = dependencies_queue.pop(0)
    
    # Skip dependencies that have already been processed
    if dependency in processed_dependencies:
        continue
    
    # Get the dependencies of the dependency
    output = subprocess.check_output(['otool', '-L', dependency])
    dependency_dependencies = output.decode().split('\n')
    
    # Iterate over the dependencies of the dependency
    for dependency_dependency in dependency_dependencies:
        # Skip empty lines
        if not dependency_dependency:
            continue
        
        # Extract the dependency name and path
        name, _, path = dependency_dependency.strip().partition(' ')
        if name == '@executable_path/':
            continue
        
        # Use install_name_tool to change the dependency to @executable_path
        subprocess.check_call(['install_name_tool', '-change', path, '@executable_path/' + os.path.basename(path), dependency])
        
        # Add the dependency to the queue
        dependencies_queue.append(path)
    
    # Copy the dependency to the .app bundle
    shutil.copy(dependency, os.path.join(app_bundle_path, 'Contents', 'MacOS'))
    
    # Add the dependency to the processed dependencies set
    processed_dependencies.add(dependency)