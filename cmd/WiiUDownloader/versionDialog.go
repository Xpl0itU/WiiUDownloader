package main

import (
	"fmt"
	"log"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/gtk"
)

// showVersionSelectionDialog prompts the user to choose between the latest
// title version or a specific one (including version 0). It returns the
// selected version and true when the user accepts. wiiudownloader.VersionLatest
// is returned for the latest version.
func showVersionSelectionDialog(parent *gtk.Window, title wiiudownloader.TitleEntry) (int, bool) {
	dialog, err := gtk.DialogNew()
	if err != nil {
		log.Printf("failed to create version selection dialog: %v", err)
		return wiiudownloader.VersionLatest, false
	}
	defer dialog.Destroy()

	dialog.SetTitle("Select Title Version")
	dialog.SetModal(true)
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetPosition(gtk.WIN_POS_CENTER_ON_PARENT)
	dialog.AddButton("Cancel", gtk.RESPONSE_CANCEL)
	dialog.AddButton("OK", gtk.RESPONSE_OK)
	dialog.SetDefaultResponse(gtk.RESPONSE_OK)

	contentArea, err := dialog.GetContentArea()
	if err != nil {
		log.Printf("failed to get version dialog content area: %v", err)
		return wiiudownloader.VersionLatest, false
	}
	contentArea.SetSpacing(12)
	contentArea.SetMarginTop(18)
	contentArea.SetMarginBottom(18)
	contentArea.SetMarginStart(18)
	contentArea.SetMarginEnd(18)

	titleLabel, err := gtk.LabelNew("")
	if err == nil {
		titleText := fmt.Sprintf("%s (%s) - %016x", escapeMarkup(title.Name), escapeMarkup(wiiudownloader.GetFormattedRegion(title.Region)), title.TitleID)
		titleLabel.SetMarkup(fmt.Sprintf("<span size='large' weight='bold'>%s</span>", titleText))
		titleLabel.SetHAlign(gtk.ALIGN_START)
		titleLabel.SetLineWrap(true)
		titleLabel.SetMaxWidthChars(50)
		contentArea.PackStart(titleLabel, false, false, 0)
	}

	isSpecific := title.Version >= 0

	latestRadio, err := gtk.RadioButtonNewWithLabel(nil, "Latest version")
	if err != nil {
		log.Printf("failed to create latest version radio: %v", err)
		return wiiudownloader.VersionLatest, false
	}
	latestRadio.SetActive(!isSpecific)
	latestRadio.SetHAlign(gtk.ALIGN_START)
	contentArea.PackStart(latestRadio, false, false, 0)

	specificBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	if err != nil {
		log.Printf("failed to create specific version box: %v", err)
		return wiiudownloader.VersionLatest, false
	}
	specificBox.SetHAlign(gtk.ALIGN_START)

	specificRadio, err := gtk.RadioButtonNewWithLabelFromWidget(latestRadio, "Set version:")
	if err != nil {
		log.Printf("failed to create specific version radio: %v", err)
		return wiiudownloader.VersionLatest, false
	}
	specificRadio.SetActive(isSpecific)
	specificBox.PackStart(specificRadio, false, false, 0)

	adjustment, _ := gtk.AdjustmentNew(float64(max(title.Version, 0)), 0, 65535, 1, 10, 0)
	spinButton, _ := gtk.SpinButtonNew(adjustment, 1, 0)
	spinButton.SetNumeric(true)
	spinButton.SetWidthChars(8)
	spinButton.SetSensitive(isSpecific)
	specificBox.PackStart(spinButton, false, false, 0)

	contentArea.PackStart(specificBox, false, false, 0)

	// Spin is only meaningful while the specific-version radio is active.
	latestRadio.Connect("toggled", func() {
		spinButton.SetSensitive(specificRadio.GetActive())
	})

	descLabel, err := gtk.LabelNew("")
	if err == nil {
		descLabel.SetHAlign(gtk.ALIGN_START)
		contentArea.PackStart(descLabel, false, false, 0)
	}

	linkBox, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 0)
	if err == nil {
		linkBox.SetHAlign(gtk.ALIGN_START)

		linkLabel, _ := gtk.LabelNew("You can find a list of available versions on the ")
		wikiLinkBtn, _ := gtk.LinkButtonNewWithLabel("https://wiiubrew.org/wiki/Title_database", "WiiUBrew Title Database")
		wikiLinkBtn.SetRelief(gtk.RELIEF_NONE)
		wikiLinkBtn.SetMarginStart(0)
		wikiLinkBtn.SetMarginEnd(0)
		if widget, err := wikiLinkBtn.GetChild(); err == nil && widget != nil {
			if lbl, ok := widget.(*gtk.Label); ok {
				lbl.SetMarginStart(0)
				lbl.SetMarginEnd(0)
			}
		}

		linkBox.PackStart(linkLabel, false, false, 0)
		linkBox.PackStart(wikiLinkBtn, false, false, 0)
		contentArea.PackStart(linkBox, false, false, 0)
	}

	hintLabel, err := gtk.LabelNew("")
	if err == nil {
		hintLabel.SetMarkup("<span size='small' alpha='70%'>Tip: You can change this later by clicking the Version column in the queue.</span>")
		hintLabel.SetLineWrap(true)
		hintLabel.SetHAlign(gtk.ALIGN_START)
		contentArea.PackStart(hintLabel, false, false, 0)
	}

	contentArea.ShowAll()

	response := dialog.Run()
	if response != gtk.RESPONSE_OK {
		return wiiudownloader.VersionLatest, false
	}

	if !specificRadio.GetActive() {
		return wiiudownloader.VersionLatest, true
	}
	return spinButton.GetValueAsInt(), true
}
