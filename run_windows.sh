#!/bin/bash
set -e

# Get GOPATH and handle Windows paths (Git Bash/MSYS2)
GOPATH=$(go env GOPATH)
if command -v cygpath &> /dev/null; then
    GOPATH=$(cygpath -u "$GOPATH")
fi

# Add GOPATH/bin and $HOME/go/bin to PATH
export PATH=$PATH:$GOPATH/bin:$HOME/go/bin

# Install goversioninfo
if ! command -v goversioninfo &> /dev/null; then
    echo "Installing goversioninfo..."
    go install github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest
fi

# Verify installation
if ! command -v goversioninfo &> /dev/null; then
    echo "Error: goversioninfo not found even after installation."
    echo "Please ensure \$GOPATH/bin is in your PATH."
    exit 1
fi

    go generate
    # Force 64-bit build to match resources
    GOARCH=amd64 go build -ldflags="-H=windowsgui"
else
fi
cd ../..

echo "Generating icon..."
if command -v magick &> /dev/null; then
    magick data/WiiUDownloader.png -define icon:auto-resize=256,128,64,48,32,16 cmd/WiiUDownloader/WiiUDownloader.ico
else
    echo "Warning: ImageMagick (magick) not found. Icon generation skipped."
    echo "Build might fail if WiiUDownloader.ico is missing."
fi

echo "Building WiiUDownloader..."
cd cmd/WiiUDownloader
go generate
# Force 64-bit build to match resources
GOARCH=amd64 go build -v -ldflags="-H=windowsgui"

echo "Copying artifacts..."
fi
fi

echo "Build complete. Binaries are ready."
./WiiUDownloader.exe
