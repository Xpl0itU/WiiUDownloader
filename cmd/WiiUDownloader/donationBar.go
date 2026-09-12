package main

import (
	"log"

	"github.com/gotk3/gotk3/gtk"
	"github.com/gotk3/gotk3/pango"
)

func newKofiButton() *gtk.Button {
	button, err := gtk.ButtonNew()
	if err != nil {
		log.Println("Unable to create Ko-fi button:", err)
		return nil
	}
	addStyleClass(button.GetStyleContext, "kofi-btn")

	icon, _ := gtk.ImageNewFromIconName(supportMeIcon, gtk.ICON_SIZE_BUTTON)
	label, _ := gtk.LabelNew("Buy me a coffee")
	box, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
	box.SetHAlign(gtk.ALIGN_CENTER)
	box.PackStart(icon, false, false, 0)
	box.PackStart(label, false, false, 0)
	button.Add(box)

	button.Connect("clicked", func() {
		openURL(kofiPageURL)
	})
	return button
}

func (mw *MainWindow) setupDonationBar() {
	bar, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 12)
	if err != nil {
		log.Println("Unable to create donation bar:", err)
		return
	}
	bar.SetMarginTop(0)
	bar.SetMarginBottom(0)
	bar.SetMarginStart(0)
	bar.SetMarginEnd(0)
	addStyleClass(bar.GetStyleContext, "gratitude-footer")

	label, err := gtk.LabelNew("")
	if err != nil {
		log.Println("Unable to create label:", err)
		return
	}
	label.SetHAlign(gtk.ALIGN_START)
	label.SetLineWrap(true)
	label.SetLineWrapMode(pango.WRAP_WORD)
	bar.PackStart(label, true, true, 0)
	mw.donationLabel = label

	if button := newKofiButton(); button != nil {
		btnBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 4)
		btnBox.PackStart(button, false, false, 0)
		if supporterLabel := mw.newSupporterLabel(); supporterLabel != nil {
			btnBox.PackStart(supporterLabel, false, false, 0)
		}
		bar.PackEnd(btnBox, false, false, 0)
	}

	mw.donationBar = bar
	mw.updateDonationBar(false)
	mw.setDonationBarVisible(mw.showDonationBar)
}

func (mw *MainWindow) updateDonationBar(success bool) {
	if mw.donationLabel == nil || mw.donationBar == nil {
		return
	}
	text := "<span size='large'><b>Games worth $40+ are free here.</b> <span foreground='#ff813f'>A coffee keeps them coming.</span></span>"
	if success {
		text = "<span size='large'><span foreground='#16a34a'><b>Downloads complete.</b></span> You saved hours. <span foreground='#ff813f'>A coffee keeps them coming.</span></span>"
	}
	mw.donationLabel.SetMarkup(text)
}

func (mw *MainWindow) setDonationBarVisible(visible bool) {
	mw.showDonationBar = visible
	if mw.donationBar != nil {
		mw.donationBar.SetVisible(visible)
	}
}
