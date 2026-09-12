package main

import (
	"fmt"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// showVersionSelectionDialog presents the version picker; onChosen is not called
// on cancel.
func showVersionSelectionDialog(parent *gtk.Window, title wiiudownloader.TitleEntry, onChosen func(version int)) {
	dialog := newAppDialog(parent, WINDOW_TITLE_PREFIX+"Select Title Version")

	contentArea := dialog.Content()

	titleText := fmt.Sprintf("%s (%s) - %016x", glib.MarkupEscapeText(title.Name), glib.MarkupEscapeText(wiiudownloader.GetFormattedRegion(title.Region)), title.TitleID)
	titleLabel := gtk.NewLabel("")
	titleLabel.SetMarkup(fmt.Sprintf("<span size='large' weight='bold'>%s</span>", titleText))
	titleLabel.SetHAlign(gtk.AlignStart)
	titleLabel.SetWrap(true)
	titleLabel.SetMaxWidthChars(50)
	contentArea.Append(titleLabel)

	isSpecific := title.Version >= 0

	// Grouped check buttons are GTK4's stand-in for radio buttons.
	latestRadio := gtk.NewCheckButtonWithLabel("Latest version")
	latestRadio.SetActive(!isSpecific)
	latestRadio.SetHAlign(gtk.AlignStart)
	contentArea.Append(latestRadio)

	specificBox := gtk.NewBox(gtk.OrientationHorizontal, 8)
	specificBox.SetHAlign(gtk.AlignStart)

	specificRadio := gtk.NewCheckButtonWithLabel("Set version:")
	specificRadio.SetGroup(latestRadio)
	specificRadio.SetActive(isSpecific)
	specificBox.Append(specificRadio)

	adjustment := gtk.NewAdjustment(float64(max(title.Version, 0)), 0, 65535, 1, 10, 0)
	spinButton := gtk.NewSpinButton(adjustment, 1, 0)
	spinButton.SetNumeric(true)
	spinButton.SetWidthChars(8)
	spinButton.SetSensitive(isSpecific)
	specificBox.Append(spinButton)

	contentArea.Append(specificBox)

	// Spin is only meaningful while the specific-version button is active.
	latestRadio.ConnectToggled(func() {
		spinButton.SetSensitive(specificRadio.Active())
	})

	linkLabel := gtk.NewLabel("")
	linkLabel.SetMarkup("You can find a list of available versions on the <a href=\"https://wiiubrew.org/wiki/Title_database\">WiiUBrew Title Database</a>")
	linkLabel.SetHAlign(gtk.AlignStart)
	contentArea.Append(linkLabel)

	hintLabel := gtk.NewLabel("")
	hintLabel.SetMarkup("<span size='small' alpha='70%'>Tip: You can change this later by clicking the Version column in the queue.</span>")
	hintLabel.SetWrap(true)
	hintLabel.SetHAlign(gtk.AlignStart)
	contentArea.Append(hintLabel)

	dialog.AddButton("Cancel", nil)
	dialog.AddActionButton("OK", "suggested-action", func() {
		if specificRadio.Active() {
			onChosen(spinButton.ValueAsInt())
			return
		}
		onChosen(wiiudownloader.VersionLatest)
	})
	dialog.Present()
}
