package main

import (
	"context"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	glib "github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// showAlert presents a one-button libadwaita alert. AdwAlertDialog renders inside
// the parent window when there is one, so it cannot end up behind it.
func showAlert(parent *gtk.Window, headline, message string) *adw.AlertDialog {
	dialog := adw.NewAlertDialog(headline, message)
	dialog.AddResponse("ok", "OK")
	dialog.SetDefaultResponse("ok")
	dialog.SetCloseResponse("ok")

	var anchor gtk.Widgetter
	if parent != nil {
		anchor = parent
	}
	dialog.Choose(context.Background(), anchor, func(res gio.AsyncResulter) {
		dialog.ChooseFinish(res)
	})
	return dialog
}

type appDialog struct {
	*gtk.Window
	content *gtk.Box
	buttons *gtk.Box
}

func newAppDialog(parent *gtk.Window, title string) *appDialog {
	window := adw.NewWindow()
	window.SetTitle(title)
	window.SetModal(true)
	if parent != nil {
		window.SetTransientFor(parent)
	}

	header := adw.NewHeaderBar()

	content := gtk.NewBox(gtk.OrientationVertical, DIALOG_CONTENT_SPACING)
	content.SetMarginTop(DIALOG_CONTENT_MARGIN)
	content.SetMarginStart(DIALOG_CONTENT_MARGIN)
	content.SetMarginEnd(DIALOG_CONTENT_MARGIN)

	buttons := gtk.NewBox(gtk.OrientationHorizontal, 0)
	buttons.SetHAlign(gtk.AlignEnd)
	buttons.SetMarginStart(DIALOG_CONTENT_MARGIN)
	buttons.SetMarginEnd(DIALOG_CONTENT_MARGIN)
	buttons.SetMarginBottom(DIALOG_CONTENT_MARGIN)
	buttons.AddCSSClass("linked")

	body := gtk.NewBox(gtk.OrientationVertical, DIALOG_CONTENT_SPACING)
	body.Append(content)
	body.Append(buttons)

	toolbar := adw.NewToolbarView()
	toolbar.AddTopBar(header)
	toolbar.SetContent(body)
	window.SetContent(toolbar)

	return &appDialog{Window: &window.Window, content: content, buttons: buttons}
}

func (d *appDialog) Content() *gtk.Box {
	return d.content
}

// AddButton runs onClick before closing the dialog.
func (d *appDialog) AddButton(label string, onClick func()) *gtk.Button {
	return d.AddActionButton(label, "", onClick)
}

// AddActionButton adds a semantic action class (suggested-action,
// destructive-action, confirm-action, warn-action) so each button's colour
// matches its role.
func (d *appDialog) AddActionButton(label, actionClass string, onClick func()) *gtk.Button {
	button := gtk.NewButtonWithLabel(label)
	if actionClass != "" {
		button.AddCSSClass(actionClass)
	}
	button.ConnectClicked(func() {
		if onClick != nil {
			onClick()
		}
		// Tearing a realized window down inside its own click handler is the macOS
		// GDK churn this avoids; close on the next main-loop turn instead.
		window := d.Window
		uiIdleAdd(func() { window.Close() })
	})
	d.buttons.Append(button)
	return button
}

func (d *appDialog) Destroy() {
	window := d.Window
	uiIdleAdd(func() { window.Close() })
}

func (d *appDialog) CloseThen(f func()) {
	window := d.Window
	uiIdleAdd(func() {
		window.Close()
		if f != nil {
			uiIdleAdd(f)
		}
	})
}

// activeFileDialog keeps the newest async chooser referenced: nothing else holds
// the native panel, so the GC could unref it while it is still on screen.
var activeFileDialog *gtk.FileDialog

func retainFileDialog(dialog *gtk.FileDialog) {
	activeFileDialog = dialog
}

// releaseFileDialog drops the pin, but only if a newer chooser has not replaced
// it, so a finishing dialog cannot clear the reference of the one behind it.
func releaseFileDialog(dialog *gtk.FileDialog) {
	if activeFileDialog == dialog {
		activeFileDialog = nil
	}
}

// chooseFolder presents a folder chooser; onSelected gets "" on cancel.
func chooseFolder(parent *gtk.Window, title, startDir string, onSelected func(path string)) {
	dialog := gtk.NewFileDialog()
	retainFileDialog(dialog)
	dialog.SetTitle(title)
	dialog.SetModal(true)
	if startDir != "" {
		dialog.SetInitialFolder(gio.NewFileForPath(startDir))
	}
	dialog.SelectFolder(context.Background(), parent, func(res gio.AsyncResulter) {
		defer releaseFileDialog(dialog)
		file, err := dialog.SelectFolderFinish(res)
		if err != nil || file == nil {
			onSelected("")
			return
		}
		onSelected(file.Path())
	})
}

// chooseFolders presents a multi-folder chooser; onSelected gets an empty slice
// on cancel.
func chooseFolders(parent *gtk.Window, title, startDir string, onSelected func(paths []string)) {
	dialog := gtk.NewFileDialog()
	retainFileDialog(dialog)
	dialog.SetTitle(title)
	dialog.SetModal(true)
	if startDir != "" {
		dialog.SetInitialFolder(gio.NewFileForPath(startDir))
	}
	dialog.SelectMultipleFolders(context.Background(), parent, func(res gio.AsyncResulter) {
		defer releaseFileDialog(dialog)
		files, err := dialog.SelectMultipleFoldersFinish(res)
		if err != nil || files == nil {
			onSelected(nil)
			return
		}

		paths := make([]string, 0, files.NItems())
		for i := uint(0); i < files.NItems(); i++ {
			object := files.Item(i)
			if object == nil {
				continue
			}
			paths = append(paths, (&gio.File{Object: object}).Path())
		}
		onSelected(paths)
	})
}

// chooseFile presents an open-file chooser; onSelected gets "" on cancel.
func chooseFile(parent *gtk.Window, title, filterName string, patterns []string, onSelected func(path string)) {
	dialog := gtk.NewFileDialog()
	retainFileDialog(dialog)
	dialog.SetTitle(title)
	dialog.SetModal(true)
	if filterName != "" && len(patterns) > 0 {
		filter := gtk.NewFileFilter()
		filter.SetName(filterName)
		for _, pattern := range patterns {
			filter.AddPattern(pattern)
		}
		dialog.SetDefaultFilter(filter)
		filters := gio.NewListStore(glib.TypeObject)
		filters.Append(glib.BaseObject(filter))
		dialog.SetFilters(filters)
	}
	dialog.Open(context.Background(), parent, func(res gio.AsyncResulter) {
		defer releaseFileDialog(dialog)
		file, err := dialog.OpenFinish(res)
		if err != nil || file == nil {
			onSelected("")
			return
		}
		onSelected(file.Path())
	})
}
