package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/Xpl0itU/dialog"
	"github.com/gotk3/gotk3/gtk"
)

func (mw *MainWindow) onDownloadQueueButtonClicked() {
	if mw.queuePane.IsQueueEmpty() {
		return
	}
	progressWindow, err := createProgressWindow(mw.window)
	if err != nil {
		return
	}
	mw.progressWindow = progressWindow
	config, err := loadConfig()

	if err != nil {
		return
	}

	selectedPath, err := mw.resolveDownloadPath(config)
	if err != nil {
		uiIdleAdd(func() {
			mw.progressWindow.Window.Hide()
		})
		return
	}

	mw.progressWindow.Window.ShowAll()
	decryptContents := mw.decryptContents
	deleteEncryptedContents := mw.getDeleteEncryptedContents()

	go func() {
		uiIdleAdd(func() {
			mw.setDownloadControlsSensitive(false)
		})

		defer uiIdleAdd(func() {
			mw.setDownloadControlsSensitive(true)
		})

		runErr := mw.onDownloadQueueClicked(selectedPath, decryptContents, deleteEncryptedContents, config)
		if runErr != nil {
			uiIdleAdd(func() {
				mw.showError(runErr)
			})
			return
		}

		errors := mw.progressWindow.GetErrors()
		if shouldShowQueueErrorSummary(runErr, errors) {
			uiIdleAdd(func() {
				mw.showErrorsDialog(errors)
			})
		}
	}()
}

func (mw *MainWindow) resolveDownloadPath(config *Config) (string, error) {
	if config.RememberLastPath && isValidPath(config.LastSelectedPath) {
		return config.LastSelectedPath, nil
	}
	builder := dialog.Directory().Title("Select a path to save the games to")
	if isValidPath(config.LastSelectedPath) {
		builder.SetStartDir(config.LastSelectedPath)
	}
	chosen, err := builder.Browse()
	if err != nil {
		return "", err
	}
	config.LastSelectedPath = chosen
	if saveErr := config.Save(); saveErr != nil {
		uiIdleAdd(func() {
			ShowErrorDialog(mw.window, saveErr)
		})
	}
	return chosen, nil
}

func shouldShowQueueErrorSummary(runErr error, errors []DownloadError) bool {
	return runErr == nil && len(errors) > 0
}

func (mw *MainWindow) onDownloadQueueClicked(selectedPath string, decryptContents, deleteEncryptedContents bool, config *Config) error {
	if mw.queuePane.IsQueueEmpty() {
		return nil
	}

	mw.progressWindow.ResetTotalsAndErrors()

	totalInQueue := mw.queuePane.GetTitleQueueSize()
	mw.progressWindow.SetQueueProgress(0, totalInQueue)

	var firstErr error
	for i, title := range mw.queuePane.GetTitleQueue() {
		if mw.progressWindow.Cancelled() {
			break
		}
		mw.progressWindow.SetQueueProgress(i, totalInQueue)

		tidStr := fmt.Sprintf("%016x", title.TitleID)
		titlePath := filepath.Join(selectedPath, fmt.Sprintf("%s [%s] [%s]", normalizeFilename(title.Name), wiiudownloader.GetFormattedKind(title.TitleID), tidStr))
		if title.Version >= 0 {
			titlePath = fmt.Sprintf("%s [v%d]", titlePath, title.Version)
		}
		downloadErr := wiiudownloader.DownloadTitle(tidStr, titlePath, title.Version, decryptContents, mw.progressWindow, deleteEncryptedContents, mw.client, config.DecryptOutputPath)

		step := nextQueueStep(downloadErr, mw.progressWindow.Cancelled(), config.ContinueOnError)
		if step.record {
			errorType := detectErrorType(downloadErr.Error())
			mw.progressWindow.AddErrorWithType(title.Name, downloadErr.Error(), tidStr, errorType, title.Version)
		}
		if step.remove {
			mw.queuePane.RemoveTitle(title)
		}
		if step.returned != nil {
			firstErr = step.returned
		}
		if step.stop {
			break
		}
	}

	uiIdleAdd(func() {
		mw.progressWindow.Window.Hide()
		mw.updateTitlesInQueue()

		errors := mw.progressWindow.GetErrors()
		if len(errors) == 0 && !mw.progressWindow.Cancelled() {
			decryptPathToShow := ""
			if decryptContents && config.DecryptOutputPath != "" {
				decryptPathToShow = config.DecryptOutputPath
			}
			mw.showSuccessDialog(totalInQueue, selectedPath, decryptPathToShow)
		}
	})

	return firstErr
}

type queueStep struct {
	remove   bool  // remove the title from the queue
	stop     bool  // stop processing further titles
	record   bool  // record the error in the progress window
	returned error // non-nil: abort the whole run with this error
}

func nextQueueStep(downloadErr error, cancelled, continueOnError bool) queueStep {
	if downloadErr == nil || downloadErr == context.Canceled {
		return queueStep{remove: true}
	}
	if cancelled {
		return queueStep{stop: true}
	}
	if continueOnError {
		return queueStep{remove: true, record: true}
	}
	return queueStep{record: true, stop: true, returned: downloadErr}
}

func (mw *MainWindow) collectTIDs(titles []wiiudownloader.TitleEntry) []uint64 {
	tids := make([]uint64, len(titles))
	for i, t := range titles {
		tids[i] = t.TitleID
	}
	return tids
}

func dedupeTitles(titles []wiiudownloader.TitleEntry, inQueue func(uint64) bool) []wiiudownloader.TitleEntry {
	seen := make(map[uint64]struct{}, len(titles))
	out := make([]wiiudownloader.TitleEntry, 0, len(titles))
	for _, entry := range titles {
		if _, dup := seen[entry.TitleID]; dup {
			continue
		}
		if inQueue != nil && inQueue(entry.TitleID) {
			continue
		}
		seen[entry.TitleID] = struct{}{}
		out = append(out, entry)
	}
	return out
}

func (mw *MainWindow) addTitlesToQueue(titles []wiiudownloader.TitleEntry) {
	toAdd := dedupeTitles(titles, func(tid uint64) bool {
		return mw.queuePane.IsTitleInQueue(wiiudownloader.TitleEntry{TitleID: tid})
	})
	if len(toAdd) == 0 {
		return
	}

	for i, entry := range toAdd {
		// Database entries default to 0 meaning "latest"; remap so v0 is selectable.
		if entry.Version == 0 {
			entry.Version = wiiudownloader.VersionLatest
			toAdd[i] = entry
		}
		mw.queuePane.SetTitleLoadingNoUpdate(entry.TitleID)
	}
	mw.queuePane.AddTitles(toAdd)

	config, _ := loadConfig()
	if !config.GetSizeOnQueue {
		return
	}

	for _, entry := range toAdd {
		go mw.fetchTitleSize(entry)
	}
}

func (mw *MainWindow) fetchTitleSize(entry wiiudownloader.TitleEntry) {
	mw.sizeFetchSemaphore <- struct{}{}
	defer func() { <-mw.sizeFetchSemaphore }()

	if !mw.queuePane.IsTitleInQueue(entry) {
		return
	}

	size, err := wiiudownloader.FetchTMDSize(entry.TitleID, entry.Version, mw.client)

	if !mw.queuePane.IsTitleInQueue(entry) {
		return
	}

	uiIdleAdd(func() {
		if err != nil {
			log.Printf("Failed to fetch size for %016x: %v", entry.TitleID, err)
			mw.queuePane.SetTitleError(entry.TitleID)
		} else {
			mw.queuePane.SetTitleSize(entry.TitleID, size)
		}
	})
}

func (mw *MainWindow) onSetVersionRequested(entries []wiiudownloader.TitleEntry) {
	if len(entries) == 0 {
		return
	}
	if len(entries) > 1 {
		infoDialog := gtk.MessageDialogNew(mw.window, gtk.DIALOG_MODAL, gtk.MESSAGE_INFO, gtk.BUTTONS_OK, "Please select a single title to set its version")
		infoDialog.Run()
		infoDialog.Destroy()
		return
	}

	entry := entries[0]
	version, ok := showVersionSelectionDialog(mw.window, entry)
	if !ok {
		return
	}

	mw.queuePane.SetTitleVersion(entry.TitleID, version)
	updated := entry
	updated.Version = version
	go mw.fetchTitleSize(updated)
}
