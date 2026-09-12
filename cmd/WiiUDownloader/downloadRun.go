package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

func (mw *MainWindow) onDownloadQueueButtonClicked() {
	if mw.queuePane.IsQueueEmpty() {
		return
	}

	config, err := loadConfig()
	if err != nil {
		return
	}

	if config.RememberLastPath && isValidPath(config.LastSelectedPath) {
		mw.startDownloadRun(config.LastSelectedPath, config)
		return
	}

	// GTK4 removed gtk_dialog_run(), so the folder chooser is asynchronous and
	// the run starts from its callback.
	chooseFolder(mw.window, WINDOW_TITLE_PREFIX+"Select Download Path", config.LastSelectedPath, func(chosen string) {
		if chosen == "" {
			return
		}
		config.LastSelectedPath = chosen
		if saveErr := config.Save(); saveErr != nil {
			ShowErrorDialog(mw.window, saveErr)
		}
		mw.startDownloadRun(chosen, config)
	})
}

// startDownloadRun opens the download UI and drains the queue in the
// background. Where that UI lives (progress window or inline in the queue pane)
// is Config.UseInlineDownloadUI's call, not this function's.
func (mw *MainWindow) startDownloadRun(selectedPath string, config *Config) {
	downloadUI := mw.newDownloadUI(config)
	if downloadUI == nil {
		return
	}
	mw.downloadUI = downloadUI
	downloadUI.Present()

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

		errors := downloadUI.GetErrors()
		if shouldShowQueueErrorSummary(runErr, errors) {
			uiIdleAdd(func() {
				mw.showErrorsDialog(errors)
			})
		}
	}()
}

func shouldShowQueueErrorSummary(runErr error, errors []DownloadError) bool {
	return runErr == nil && len(errors) > 0
}

func (mw *MainWindow) onDownloadQueueClicked(selectedPath string, decryptContents, deleteEncryptedContents bool, config *Config) error {
	if mw.queuePane.IsQueueEmpty() {
		return nil
	}

	downloadUI := mw.downloadUI
	if downloadUI == nil {
		return nil
	}
	downloadUI.ResetTotalsAndErrors()

	totalInQueue := mw.queuePane.GetTitleQueueSize()
	downloadUI.SetQueueProgress(0, totalInQueue)

	var firstErr error
	for i, title := range mw.queuePane.GetTitleQueue() {
		if downloadUI.Cancelled() {
			break
		}
		downloadUI.SetQueueProgress(i, totalInQueue)
		downloadUI.SetTitleState(title.TitleID, queueStateDownloading)

		tidStr := fmt.Sprintf("%016x", title.TitleID)
		titlePath := filepath.Join(selectedPath, fmt.Sprintf("%s [%s] [%s]", normalizeFilename(title.Name), wiiudownloader.GetFormattedKind(title.TitleID), tidStr))
		if title.Version >= 0 {
			titlePath = fmt.Sprintf("%s [v%d]", titlePath, title.Version)
		}
		downloadErr := wiiudownloader.DownloadTitle(tidStr, titlePath, title.Version, decryptContents, downloadUI, deleteEncryptedContents, mw.client, config.DecryptOutputPath)

		cancelled := downloadUI.Cancelled()
		step := nextQueueStep(downloadErr, cancelled, config.ContinueOnError)
		switch {
		case step.remove:
			downloadUI.SetTitleState(title.TitleID, queueStateDone)
		case cancelled:
			downloadUI.SetTitleState(title.TitleID, queueStateCancelled)
		default:
			downloadUI.SetTitleState(title.TitleID, queueStateFailed)
		}
		if step.record {
			errorType := detectErrorType(downloadErr.Error())
			downloadUI.AddErrorWithType(title.Name, downloadErr.Error(), tidStr, errorType, title.Version)
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
		downloadUI.Hide()
		mw.updateTitlesInQueue()

		errors := downloadUI.GetErrors()
		if len(errors) == 0 && !downloadUI.Cancelled() {
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
		showAlert(mw.window, WINDOW_TITLE_PREFIX+"Set Title Version", "Please select a single title to set its version.")
		return
	}

	entry := entries[0]
	showVersionSelectionDialog(mw.window, entry, func(version int) {
		mw.queuePane.SetTitleVersion(entry.TitleID, version)
		updated := entry
		updated.Version = version
		go mw.fetchTitleSize(updated)
	})
}
