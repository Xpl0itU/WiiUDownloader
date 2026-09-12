package main

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

func newKofiButton() *gtk.Button {
	content := adw.NewButtonContent()
	content.SetIconName(supportMeIcon)
	content.SetLabel("Buy me a coffee")

	button := gtk.NewButton()
	button.AddCSSClass("kofi-btn")
	button.AddCSSClass("pill")
	button.SetChild(content)

	button.ConnectClicked(func() {
		openURL(kofiPageURL)
	})
	return button
}

func (mw *MainWindow) setupDonationBar() {
	bar := gtk.NewBox(gtk.OrientationHorizontal, DONATION_BAR_SPACING)
	bar.AddCSSClass("gratitude-footer")

	// Two stacked labels instead of one wrapping label: the copy gets proper
	// line spacing and never crowds the button next to it.
	textBox := gtk.NewBox(gtk.OrientationVertical, 6)
	textBox.SetHExpand(true)
	textBox.SetVAlign(gtk.AlignCenter)
	textBox.SetMarginEnd(16)

	headline := gtk.NewLabel("")
	headline.SetHAlign(gtk.AlignStart)
	headline.SetWrap(true)
	headline.SetWrapMode(pango.WrapWord)
	textBox.Append(headline)
	mw.donationLabel = headline

	subline := gtk.NewLabel("")
	subline.AddCSSClass("gratitude-subline")
	subline.SetHAlign(gtk.AlignStart)
	subline.SetMarginTop(2)
	subline.SetWrap(true)
	subline.SetWrapMode(pango.WrapWord)
	textBox.Append(subline)
	mw.donationSubLabel = subline

	bar.Append(textBox)

	btnBox := gtk.NewBox(gtk.OrientationVertical, 8)
	btnBox.SetVAlign(gtk.AlignCenter)
	if button := newKofiButton(); button != nil {
		btnBox.Append(button)
	}
	if supporterLabel := mw.newSupporterLabel(); supporterLabel != nil {
		btnBox.Append(supporterLabel)
	}
	bar.Append(btnBox)

	mw.donationBar = bar
	mw.updateDonationBar(false)
	mw.setDonationBarVisible(mw.showDonationBar)
}

func (mw *MainWindow) updateDonationBar(success bool) {
	if mw.donationLabel == nil || mw.donationBar == nil {
		return
	}
	headline := "<span size='x-large'><b>Games worth $40+ are free here.</b></span>"
	if success {
		headline = "<span size='x-large'><b><span foreground='#16a34a'>Downloads complete.</span> You saved hours.</b></span>"
	}
	mw.donationLabel.SetMarkup(headline)
	if mw.donationSubLabel != nil {
		mw.donationSubLabel.SetMarkup("<span foreground='#ff813f'><b>A coffee keeps them coming.</b></span>")
	}
}

func (mw *MainWindow) setDonationBarVisible(visible bool) {
	mw.showDonationBar = visible
	if mw.donationBar != nil {
		mw.donationBar.SetVisible(visible)
	}
}
