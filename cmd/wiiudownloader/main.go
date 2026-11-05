package main

import (
	"os"

	"github.com/Xpl0itU/WiiUDownloader/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
