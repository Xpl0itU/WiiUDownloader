package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

type decryptResultJSON struct {
	Path   string `json:"path"`
	Output string `json:"output"`
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}

func cmdDecrypt(application *app, args []string) error {
	fs := newFlagSet(application, "decrypt", "Decrypt previously downloaded title folders.", `Usage:
  wiiudl decrypt [flags] <dir...>

Each directory must contain title.tmd, title.tik and the encrypted .app files
(as left by a download without --decrypt). By default files are decrypted in
place. With --output, a single directory writes to exactly that path; multiple
directories write to <output>/<directory name>.

Flags:`)
	deleteEncrypted := fs.Bool("delete-encrypted", false, "Delete encrypted .app/.h3 files after decryption")
	outputDir := ""
	fs.StringVar(&outputDir, "output", "", "Write decrypted files to this directory")
	fs.StringVar(&outputDir, "o", "", "Alias for --output")
	dirs, err := parseInterspersed(fs, args)
	if err != nil {
		return flagError(err)
	}
	if len(dirs) == 0 {
		fs.Usage()
		return silentExit{code: 2}
	}

	started := time.Now()
	results := make([]decryptResultJSON, 0, len(dirs))
	failed := 0
	cancelled := false

	for index, argument := range dirs {
		sourceDir := absolutePath(argument)
		targetDir := ""
		if outputDir != "" {
			if len(dirs) == 1 {
				targetDir = absolutePath(outputDir)
			} else {
				targetDir = filepath.Join(absolutePath(outputDir), filepath.Base(sourceDir))
			}
		}

		result := decryptResultJSON{Path: sourceDir, Output: targetDir}
		if result.Output == "" {
			result.Output = sourceDir
		}

		if info, err := os.Stat(sourceDir); err != nil || !info.IsDir() {
			result.Error = fmt.Sprintf("not a directory: %s", sourceDir)
			failed++
			results = append(results, result)
			if !application.quiet {
				fmt.Fprintf(application.stderr, "[%d/%d] %s: %s\n", index+1, len(dirs), sourceDir, result.Error)
			}
			continue
		}

		if !application.quiet {
			fmt.Fprintf(application.stderr, "[%d/%d] %s\n", index+1, len(dirs), sourceDir)
		}
		progress := newTerminalProgress(application.stderr, isTerminal(os.Stderr), application.quiet)
		progress.SetGameTitle(filepath.Base(sourceDir))
		progress.ResetTotals()
		application.interrupts.set(progress)
		decryptErr := wiiudownloader.DecryptContents(sourceDir, progress, *deleteEncrypted, targetDir)
		progress.Finish()
		application.interrupts.set(nil)

		if progress.Cancelled() {
			cancelled = true
			result.Error = "cancelled"
			results = append(results, result)
			break
		}
		if decryptErr != nil {
			result.Error = decryptErr.Error()
			failed++
			results = append(results, result)
			fmt.Fprintf(application.stderr, "Failed: %s: %v\n", sourceDir, decryptErr)
			continue
		}
		result.OK = true
		results = append(results, result)
		if !application.quiet && result.Output != result.Path {
			fmt.Fprintf(application.stderr, "Decrypted to %s\n", result.Output)
		}
	}

	if application.json {
		if err := writeJSON(application.stdout, results); err != nil {
			return err
		}
	} else if !application.quiet || failed > 0 {
		succeeded := len(results) - failed - boolToInt(cancelled)
		fmt.Fprintf(application.stderr, "Decrypted %d of %d folder(s) in %s.\n", succeeded, len(dirs), formatDuration(time.Since(started)))
	}

	if cancelled {
		return interruptedError{}
	}
	if failed > 0 {
		return silentExit{code: 1}
	}
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
