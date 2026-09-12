package main

import (
	"fmt"
	"log"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

func showVersionSelectionDialog(parent *gtk.Window, title wiiudownloader.TitleEntry) (int, bool) {
	dialog, err := gtk.DialogNew()
	if err != nil {
		log.Printf("failed to create version selection dialog: %v", err)
		return wiiudownloader.VersionLatest, false
	}
	defer dialog.Destroy()

	dialog.SetTitle(WINDOW_TITLE_PREFIX + "Select Title Version")
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
	contentArea.SetSpacing(DIALOG_CONTENT_SPACING)
	contentArea.SetMarginTop(DIALOG_CONTENT_MARGIN)
	contentArea.SetMarginBottom(DIALOG_CONTENT_MARGIN)
	contentArea.SetMarginStart(DIALOG_CONTENT_MARGIN)
	contentArea.SetMarginEnd(DIALOG_CONTENT_MARGIN)

	titleLabel, err := gtk.LabelNew("")
	if err == nil {
		titleText := fmt.Sprintf("%s (%s) - %016x", glib.MarkupEscapeText(title.Name), glib.MarkupEscapeText(wiiudownloader.GetFormattedRegion(title.Region)), title.TitleID)
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

	linkLabel, err := gtk.LabelNew("")
	if err == nil {
		linkLabel.SetMarkup("You can find a list of available versions on the <a href=\"https://wiiubrew.org/wiki/Title_database\">WiiUBrew Title Database</a>")
		linkLabel.SetHAlign(gtk.ALIGN_START)
		contentArea.PackStart(linkLabel, false, false, 0)
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
