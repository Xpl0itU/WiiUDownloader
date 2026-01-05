import os
import subprocess
import shutil
import sys


def run_lipo(output_path, input_paths):
    """Runs lipo to create a universal binary."""
    cmd = ["lipo", "-create"] + input_paths + ["-output", output_path]
    print(f"Running: {' '.join(cmd)}")
    subprocess.check_call(cmd)


def merge_apps(intel_app, arm_app, output_app):
    """Merges an Intel .app and an ARM .app into a Universal .app."""

    if os.path.exists(output_app):
        shutil.rmtree(output_app)

    # Copy the ARM app as the base for the Universal app (arbitrary choice, mostly same resources)
    print(f"Copying {arm_app} to {output_app}...")
    shutil.copytree(arm_app, output_app, symlinks=True)

    # Walk through the bundle to find binaries to merge
    # We mainly care about the main executable and dylibs in Contents/MacOS and Contents/MacOS/lib

    # Path to the main executable setup
    executable_name = "WiiUDownloader"  # Known from create_bundle.py

    # 1. Merge the main executable
    intel_exe = os.path.join(intel_app, "Contents", "MacOS", executable_name)
    arm_exe = os.path.join(arm_app, "Contents", "MacOS", executable_name)
    out_exe = os.path.join(output_app, "Contents", "MacOS", executable_name)

    if os.path.exists(intel_exe) and os.path.exists(arm_exe):
        print(f"Merging main executable: {executable_name}")
        run_lipo(out_exe, [intel_exe, arm_exe])
    else:
        print(f"Error: Main executable not found in one of the bundles.")
        sys.exit(1)

    # 2. Merge dylibs in Contents/MacOS/lib
    # dylibbundler puts them there.
    intel_lib_dir = os.path.join(intel_app, "Contents", "MacOS", "lib")
    arm_lib_dir = os.path.join(arm_app, "Contents", "MacOS", "lib")
    out_lib_dir = os.path.join(output_app, "Contents", "MacOS", "lib")

    if os.path.exists(intel_lib_dir) and os.path.exists(arm_lib_dir):
        # We assume both have roughly the same libs. We iterate over the output dir (copied from ARM)
        for root, dirs, files in os.walk(out_lib_dir):
            for file in files:
                if file.endswith(".dylib"):
                    rel_path = os.path.relpath(os.path.join(root, file), out_lib_dir)
                    intel_dylib = os.path.join(intel_lib_dir, rel_path)
                    arm_dylib = os.path.join(arm_lib_dir, rel_path)

                    if os.path.exists(intel_dylib) and os.path.exists(arm_dylib):
                        print(f"Merging library: {rel_path}")
                        # Overwrite the one in output_app (which is currently just the ARM one)
                        out_dylib = os.path.join(root, file)
                        try:
                            run_lipo(out_dylib, [intel_dylib, arm_dylib])
                        except subprocess.CalledProcessError:
                            print(
                                f"Warning: Failed to merge {rel_path}. Maybe it's already universal or specific to one arch?"
                            )
                    else:
                        print(f"Warning: {rel_path} missing in one architecture.")
    else:
        print("No lib directory found, skipping dylib merge.")

    print(f"Universal app created at {output_app}")


if __name__ == "__main__":
    if len(sys.argv) != 4:
        print(
            "Usage: python3 merge_universal_app.py <intel_app_path> <arm_app_path> <output_app_path>"
        )
        sys.exit(1)

    intel_path = sys.argv[1]
    arm_path = sys.argv[2]
    out_path = sys.argv[3]

    merge_apps(intel_path, arm_path, out_path)
