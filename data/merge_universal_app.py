import os
import subprocess
import shutil
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from bundle_libs import verify_bundle
from bundle_paths import rewrite_binary
from theme_compat import install_adwaita_compat_aliases


def run_lipo(output_path, input_paths):
    """Runs lipo to create a universal binary."""
    cmd = ["lipo", "-create"] + input_paths + ["-output", output_path]
    print(f"Running: {' '.join(cmd)}")
    subprocess.check_call(cmd)


def rewrite_loaders_cache_paths(cache_text, old_bundle_paths, new_bundle_path):
    """Rewrite absolute bundle paths inside loaders.cache to the final app path."""
    rewritten = cache_text
    for old_bundle_path in old_bundle_paths:
        if old_bundle_path:
            rewritten = rewritten.replace(old_bundle_path, new_bundle_path)
    return rewritten


def rewrite_output_loaders_cache(output_app, old_bundle_paths):
    cache_path = os.path.join(output_app, "Contents", "Resources", "loaders.cache")
    if not os.path.exists(cache_path):
        return

    with open(cache_path, "r", encoding="utf-8") as f:
        cache_text = f.read()

    rewritten = rewrite_loaders_cache_paths(cache_text, old_bundle_paths, output_app)
    if rewritten != cache_text:
        with open(cache_path, "w", encoding="utf-8") as f:
            f.write(rewritten)
        print(f"Rewrote loaders.cache paths for {output_app}")


def install_output_theme_aliases(output_app):
    adwaita_dir = os.path.join(output_app, "Contents", "Resources", "share", "icons", "Adwaita")
    if os.path.isdir(adwaita_dir):
        install_adwaita_compat_aliases(adwaita_dir)


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
        # Ensure executable permissions
        os.chmod(out_exe, 0o755)
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
                if file.endswith(".dylib") or file.endswith(".so"):
                    rel_path = os.path.relpath(os.path.join(root, file), out_lib_dir)
                    intel_dylib = os.path.join(intel_lib_dir, rel_path)
                    arm_dylib = os.path.join(arm_lib_dir, rel_path)

                    if os.path.exists(intel_dylib) and os.path.exists(arm_dylib):
                        print(f"Merging binary: {rel_path}")
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

    rewrite_output_loaders_cache(output_app, [intel_app, arm_app])
    install_output_theme_aliases(output_app)

    out_macos = os.path.join(output_app, "Contents", "MacOS")
    print("Defensively rewriting universal load commands...")
    rewrite_binary(out_exe, is_main_exe=True, run_fn=print)
    for root, _dirs, files in os.walk(out_macos):
        for f in files:
            if f.endswith(".dylib") or f.endswith(".so"):
                rewrite_binary(os.path.join(root, f), is_main_exe=False, run_fn=print)

    # A dylib present in only one slice is kept as-is above, so the merged app can
    # end up loading something the bundle does not have. Fail before the DMG.
    verify_bundle(out_exe, out_macos)

    # Ad-hoc code signing (lipo strips signatures, macOS SIP requires signed dlopen'd code)
    print("Signing universal app...")
    for root, dirs, files in os.walk(out_macos):
        for f in sorted(files):
            if f.endswith(".so") or f.endswith(".dylib"):
                p = os.path.join(root, f)
                res = subprocess.run(["codesign", "--sign", "-", "--force", "--timestamp=none", p],
                                     capture_output=True, text=True)
                if res.returncode != 0:
                    print(f"Warning: codesign failed for {p}: {res.stderr}")
    res = subprocess.run(["codesign", "--sign", "-", "--force", "--timestamp=none", out_exe],
                         capture_output=True, text=True)
    if res.returncode != 0:
        print(f"Warning: codesign failed for main exe: {res.stderr}")
    res = subprocess.run(["codesign", "--sign", "-", "--force", "--deep", "--timestamp=none", output_app],
                         capture_output=True, text=True)
    if res.returncode != 0:
        print(f"Warning: codesign failed for bundle: {res.stderr}")
    else:
        print("Code signing complete")

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
