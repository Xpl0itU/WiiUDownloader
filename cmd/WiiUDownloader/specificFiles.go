package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"
)

const (
	SPECIFIC_FILES_DIALOG_WIDTH  = 760
	SPECIFIC_FILES_DIALOG_HEIGHT = 620
	SPECIFIC_FILE_ROW_MARGIN     = 8
	SPECIFIC_FILE_ROW_SPACING    = 6
	SPECIFIC_FILE_INDENT         = 18
	SPECIFIC_FILE_ARROW_SLOT     = 26
)

type titleFileNode struct {
	path     string
	name     string
	dir      bool
	depth    int
	parent   *titleFileNode
	children []*titleFileNode
	file     wiiudownloader.TitleFile

	lowerPath string

	check *gtk.CheckButton
	arrow *gtk.Button
	row   gtk.Widgetter

	expanded bool
	match    bool
	selected bool
}

type titleFileRowWidgets struct {
	box       *gtk.Box
	arrowSlot *gtk.Box
	arrow     *gtk.Button
	check     *gtk.CheckButton
	label     *gtk.Label
	size      *gtk.Label
	node      *titleFileNode
}

func (mw *MainWindow) showSpecificFilesDialogFor(entry wiiudownloader.TitleEntry) {
	dialog := newAppDialog(mw.window, WINDOW_TITLE_PREFIX+"Download Specific Files")
	dialog.SetDefaultSize(SPECIFIC_FILES_DIALOG_WIDTH, SPECIFIC_FILES_DIALOG_HEIGHT)

	contentArea := dialog.Content()

	headerLabel := gtk.NewLabel("")
	headerLabel.SetMarkup("<span font='14' weight='bold'>Download specific files</span>")
	headerLabel.SetHAlign(gtk.AlignStart)
	contentArea.Append(headerLabel)

	descText := "Tick the files you want; only the contents they live in are fetched, and the selected files are decrypted into the same folder tree."
	if entry.Name != "" {
		descText = fmt.Sprintf("%s - tick the files you want. Only the contents they live in are fetched, and the selected files are decrypted into the same folder tree.", entry.Name)
	}
	descLabel := gtk.NewLabel(descText)
	descLabel.SetHAlign(gtk.AlignStart)
	descLabel.SetWrap(true)
	contentArea.Append(descLabel)

	searchEntry := gtk.NewSearchEntry()
	searchEntry.SetPlaceholderText("Search files…")
	searchEntry.SetHExpand(true)
	contentArea.Append(searchEntry)

	controlsBox := gtk.NewBox(gtk.OrientationHorizontal, SPECIFIC_FILE_ROW_SPACING)

	expandButton := gtk.NewButtonWithLabel("Expand All")
	SetupButtonAccessibility(expandButton, "Expand every folder")
	controlsBox.Append(expandButton)

	collapseButton := gtk.NewButtonWithLabel("Collapse All")
	SetupButtonAccessibility(collapseButton, "Collapse every folder")
	controlsBox.Append(collapseButton)

	controlsSpacer := gtk.NewBox(gtk.OrientationHorizontal, 0)
	controlsSpacer.SetHExpand(true)
	controlsBox.Append(controlsSpacer)

	allButton := gtk.NewButtonWithLabel("Select All")
	SetupButtonAccessibility(allButton, "Select every file, or every matching file while searching")
	controlsBox.Append(allButton)

	noneButton := gtk.NewButtonWithLabel("Select None")
	SetupButtonAccessibility(noneButton, "Clear every file, or every matching file while searching")
	controlsBox.Append(noneButton)

	contentArea.Append(controlsBox)

	statusLabel := gtk.NewLabel("Loading file list…")
	statusLabel.SetHAlign(gtk.AlignStart)
	statusLabel.AddCSSClass("dim-label")
	statusLabel.SetWrap(true)
	contentArea.Append(statusLabel)

	scrolledWindow := gtk.NewScrolledWindow()
	scrolledWindow.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scrolledWindow.SetVExpand(true)
	contentArea.Append(scrolledWindow)

	rows := newRowStore(nil)
	nodeByKey := make(map[string]*titleFileNode)
	rowWidgets := make(map[uintptr]*titleFileRowWidgets)

	var (
		allNodes      []*titleFileNode
		fileNodes     []*titleFileNode
		rootNodes     []*titleFileNode
		query         string
		matchCount    int
		tree          *wiiudownloader.TitleFileTree
		treeTotalSize uint64
		state         = filePickerLoading
		updateHint    func()
	)

	bulk := false

	version := wiiudownloader.VersionLatest
	if entry.Version > 0 {
		version = entry.Version
	}

	filter := gtk.NewCustomFilter(func(item *coreglib.Object) bool {
		node := nodeByKey[rowKey(item)]
		if node == nil {
			return false
		}
		if query != "" {
			return node.match
		}
		for parent := node.parent; parent != nil; parent = parent.parent {
			if !parent.expanded {
				return false
			}
		}
		return true
	})
	model := gtk.NewFilterListModel(rows, &filter.Filter)
	view := gtk.NewListView(gtk.NewNoSelection(model), nil)
	view.SetShowSeparators(false)
	SetupListViewAccessibility(view)
	scrolledWindow.SetChild(view)
	lastTitleFilePicker.model = model
	lastTitleFilePicker.store = rows
	lastTitleFilePicker.view = view
	lastTitleFilePicker.query = func() string { return query }
	lastTitleFilePicker.search = searchEntry
	lastTitleFilePicker.selectAll = allButton
	lastTitleFilePicker.selectNone = noneButton
	lastTitleFilePicker.expandAll = expandButton
	lastTitleFilePicker.collapseAll = collapseButton
	lastTitleFilePicker.status = statusLabel

	refreshFilter := func() {
		filter.Changed(gtk.FilterChangeDifferent)
		uiIdleAdd(func() {
			if model.NItems() > 0 {
				view.ScrollTo(0, gtk.ListScrollNone, nil)
			}
		})
	}

	var matchedNodes []*titleFileNode
	updateMatches := func(text string) {
		query = strings.ToLower(strings.TrimSpace(text))
		for _, node := range matchedNodes {
			node.match = false
		}
		matchedNodes = matchedNodes[:0]
		matchCount = 0
		if query == "" {
			for _, node := range allNodes {
				node.match = true
			}
			matchedNodes = append(matchedNodes, allNodes...)
			matchCount = len(fileNodes)
		} else {
			for _, node := range fileNodes {
				if !strings.Contains(node.lowerPath, query) {
					continue
				}
				node.match = true
				matchedNodes = append(matchedNodes, node)
				matchCount++
				for parent := node.parent; parent != nil; parent = parent.parent {
					if parent.match {
						break
					}
					parent.match = true
					matchedNodes = append(matchedNodes, parent)
				}
			}
		}
		refreshFilter()
	}

	selectedCountSize := func() (count int, size uint64) {
		for _, node := range fileNodes {
			if node.selected {
				count++
				size += node.file.Size
			}
		}
		return count, size
	}

	refreshStatus := func() {
		if state != filePickerReady {
			return
		}
		lastTitleFilePicker.statusPasses++
		count, size := selectedCountSize()
		// With a search active, report how many files match out of the total,
		// so the count on screen always matches the rows on screen.
		files := fmt.Sprintf("%d file(s)", len(fileNodes))
		if query != "" {
			files = fmt.Sprintf("%d of %d file(s)", matchCount, len(fileNodes))
		}
		statusLabel.SetText(fmt.Sprintf("%s, %s total. %d selected (%s).",
			files, formatBytes(treeTotalSize), count, formatBytes(size)))
	}

	var dirCounts func(*titleFileNode) (int, int)
	dirCounts = func(node *titleFileNode) (int, int) {
		active, total := 0, 0
		for _, child := range node.children {
			if !child.dir {
				total++
				if child.selected {
					active++
				}
				continue
			}
			childActive, childTotal := dirCounts(child)
			active += childActive
			total += childTotal
		}
		return active, total
	}

	applyDirCheck := func(node *titleFileNode, active, total int) {
		if node.check == nil {
			return
		}
		node.check.SetActive(total > 0 && active == total)
		node.check.SetInconsistent(active > 0 && active < total)
	}

	refreshDirState := func(node *titleFileNode) {
		if !node.dir {
			return
		}
		active, total := dirCounts(node)
		applyDirCheck(node, active, total)
	}

	refreshAncestors := func(node *titleFileNode) {
		for parent := node.parent; parent != nil; parent = parent.parent {
			refreshDirState(parent)
		}
	}

	refreshAllDirStates := func() {
		lastTitleFilePicker.folderPasses++
		var walk func(*titleFileNode) (int, int)
		walk = func(node *titleFileNode) (int, int) {
			if !node.dir {
				if node.selected {
					return 1, 1
				}
				return 0, 1
			}
			active, total := 0, 0
			for _, child := range node.children {
				childActive, childTotal := walk(child)
				active += childActive
				total += childTotal
			}
			applyDirCheck(node, active, total)
			return active, total
		}
		for _, root := range rootNodes {
			walk(root)
		}
	}

	var setNodeSelected func(*titleFileNode, bool)
	setNodeSelected = func(node *titleFileNode, active bool) {
		node.selected = active
		if node.check != nil {
			node.check.SetActive(active)
		}
		if !node.dir {
			return
		}
		for _, child := range node.children {
			setNodeSelected(child, active)
		}
		if node.check != nil {
			node.check.SetInconsistent(false)
		}
	}

	withBulk := func(fn func()) {
		previous := bulk
		bulk = true
		fn()
		bulk = previous
	}

	onRowCheckToggled := func(item *gtk.ListItem) {
		if bulk || item == nil {
			return
		}
		node := nodeByKey[listItemKey(item)]
		widgets := rowWidgets[item.Native()]
		if node == nil || widgets == nil {
			return
		}
		active := widgets.check.Active()
		if active == node.selected {
			return
		}
		withBulk(func() {
			setNodeSelected(node, active)
			if node.dir {
				refreshDirState(node)
			}
			refreshAncestors(node)
		})
		refreshStatus()
	}

	onRowArrowClicked := func(item *gtk.ListItem) {
		if item == nil {
			return
		}
		node := nodeByKey[listItemKey(item)]
		widgets := rowWidgets[item.Native()]
		if node == nil || widgets == nil || !node.dir {
			return
		}
		node.expanded = !node.expanded
		if node.expanded {
			widgets.arrow.SetIconName("pan-down-symbolic")
		} else {
			widgets.arrow.SetIconName("pan-end-symbolic")
		}
		refreshFilter()
	}

	factory := gtk.NewSignalListItemFactory()
	factory.ConnectSetup(func(obj *coreglib.Object) {
		item := listItem(obj)
		if item == nil {
			return
		}
		box := gtk.NewBox(gtk.OrientationHorizontal, SPECIFIC_FILE_ROW_SPACING)
		box.SetMarginEnd(SPECIFIC_FILE_ROW_MARGIN)
		box.SetMarginTop(4)
		box.SetMarginBottom(4)

		arrowSlot := gtk.NewBox(gtk.OrientationHorizontal, 0)
		arrowSlot.SetSizeRequest(SPECIFIC_FILE_ARROW_SLOT, -1)
		arrow := gtk.NewButton()
		arrow.AddCSSClass("flat")
		arrow.AddCSSClass("file-picker-arrow")
		arrow.SetHAlign(gtk.AlignCenter)
		arrow.SetVAlign(gtk.AlignCenter)
		arrowSlot.Append(arrow)
		box.Append(arrowSlot)

		check := gtk.NewCheckButton()
		check.SetVAlign(gtk.AlignCenter)
		box.Append(check)

		label := gtk.NewLabel("")
		label.SetHAlign(gtk.AlignStart)
		label.SetHExpand(true)
		label.SetEllipsize(pango.EllipsizeEnd)
		box.Append(label)

		sizeLabel := gtk.NewLabel("")
		sizeLabel.AddCSSClass("dim-label")
		sizeLabel.SetHAlign(gtk.AlignEnd)
		box.Append(sizeLabel)

		item.SetChild(box)
		rowWidgets[item.Native()] = &titleFileRowWidgets{
			box: box, arrowSlot: arrowSlot, arrow: arrow,
			check: check, label: label, size: sizeLabel,
		}

		check.ConnectToggled(func() { onRowCheckToggled(item) })
		arrow.ConnectClicked(func() { onRowArrowClicked(item) })
	})
	factory.ConnectBind(func(obj *coreglib.Object) {
		item := listItem(obj)
		if item == nil {
			return
		}
		widgets := rowWidgets[item.Native()]
		node := nodeByKey[listItemKey(item)]
		if widgets == nil || node == nil {
			return
		}

		widgets.node = node
		node.row = widgets.box
		node.check = widgets.check
		node.arrow = widgets.arrow

		widgets.box.SetMarginStart(SPECIFIC_FILE_ROW_MARGIN + node.depth*SPECIFIC_FILE_INDENT)
		// Set the widget before anything can read it back: the handler compares
		// against the node, so a recycled state cannot be mistaken for a click.
		widgets.check.SetActive(node.selected)

		// Only the arrow is toggled: the slot itself must stay laid out for every
		// row. Hiding it for files took the indent step with it, which left a
		// child row to the left of its own parent.
		widgets.arrow.SetVisible(node.dir)

		if node.dir {
			if node.expanded {
				widgets.arrow.SetIconName("pan-down-symbolic")
			} else {
				widgets.arrow.SetIconName("pan-end-symbolic")
			}
			widgets.label.SetText(node.name + "/")
			widgets.size.SetVisible(false)
			active, total := dirCounts(node)
			applyDirCheck(node, active, total)
			SetupCheckButtonAccessibility(widgets.check, fmt.Sprintf("Select every file under %s", node.name))
			SetupButtonAccessibility(widgets.arrow, fmt.Sprintf("Collapse or expand %s", node.name))
			return
		}

		widgets.label.SetText(node.name)
		widgets.size.SetText(formatBytes(node.file.Size))
		widgets.size.SetVisible(true)
		widgets.check.SetInconsistent(false)
		SetupCheckButtonAccessibility(widgets.check, fmt.Sprintf("Download %s", node.path))
	})
	factory.ConnectUnbind(func(obj *coreglib.Object) {
		item := listItem(obj)
		if item == nil {
			return
		}
		widgets := rowWidgets[item.Native()]
		if widgets == nil || widgets.node == nil {
			return
		}
		node := widgets.node
		widgets.node = nil
		node.row = nil
		node.check = nil
		node.arrow = nil
	})
	view.SetFactory(&factory.ListItemFactory)

	handOff := func(paths []string) {
		state = filePickerHandedOff
		mw.chooseTitleFilesTarget(tree, paths)
	}

	downloadButton := dialog.AddActionButton("Download Selected", "suggested-action", func() {
		if state != filePickerReady || tree == nil {
			return
		}
		paths := make([]string, 0, len(fileNodes))
		for _, node := range fileNodes {
			if node.selected {
				paths = append(paths, node.path)
			}
		}
		if len(paths) == 0 {
			state = filePickerAborted
			tree.Close()
			showAlert(mw.window, WINDOW_TITLE_PREFIX+"Download Specific Files", "Select at least one file to download.")
			return
		}
		handOff(paths)
	})
	downloadButton.SetSensitive(false)
	allButton.SetSensitive(false)
	noneButton.SetSensitive(false)
	expandButton.SetSensitive(false)
	collapseButton.SetSensitive(false)
	searchEntry.SetSensitive(false)

	applyBulkSelection := func(active bool) {
		lastTitleFilePicker.bulkPasses++
		withBulk(func() {
			for _, node := range bulkTitleFileNodes(fileNodes, query) {
				node.selected = active
				if node.check != nil {
					node.check.SetActive(active)
				}
			}
			refreshAllDirStates()
		})
		refreshStatus()
	}
	allButton.ConnectClicked(func() { applyBulkSelection(true) })
	noneButton.ConnectClicked(func() { applyBulkSelection(false) })
	setAllExpanded := func(expanded bool) {
		icon := "pan-end-symbolic"
		if expanded {
			icon = "pan-down-symbolic"
		}
		for _, node := range allNodes {
			if !node.dir {
				continue
			}
			node.expanded = expanded
			if node.arrow != nil {
				node.arrow.SetIconName(icon)
			}
		}
		refreshFilter()
	}
	expandButton.ConnectClicked(func() { setAllExpanded(true) })
	collapseButton.ConnectClicked(func() { setAllExpanded(false) })
	searchEntry.ConnectSearchChanged(func() {
		updateMatches(searchEntry.Text())
		refreshStatus()
	})
	updateHint = refreshStatus

	dialog.AddButton("Cancel", nil)
	dialog.ConnectCloseRequest(func() bool {
		// The download hands the tree over before the window closes, so only an
		// abandoned dialog frees the temp files.
		if state != filePickerHandedOff && tree != nil {
			tree.Close()
			tree = nil
		}
		if state == filePickerLoading {
			state = filePickerAborted
		}
		return false
	})
	dialog.Present()

	go func() {
		fetched, err := fetchTitleFileTree(entry.TitleID, version, mw.client)
		uiIdleAdd(func() {
			if state == filePickerAborted {
				if fetched != nil {
					fetched.Close()
				}
				return
			}
			if err != nil {
				statusLabel.SetText("Could not load the file list.")
				state = filePickerAborted
				ShowErrorDialog(mw.window, err)
				return
			}
			if len(fetched.Files) == 0 {
				statusLabel.SetText("This title lists no files.")
				state = filePickerAborted
				fetched.Close()
				return
			}

			tree = fetched
			roots, nodes := buildTitleFileNodes(fetched.Files)
			allNodes = nodes
			fileNodes = fileNodes[:0]
			keys := make([]string, 0, len(nodes))
			for index, node := range nodes {
				key := strconv.Itoa(index)
				nodeByKey[key] = node
				keys = append(keys, key)
				if !node.dir {
					fileNodes = append(fileNodes, node)
				}
			}

			rootNodes = roots
			treeTotalSize = fetched.TotalSize()
			lastTitleFileNodes = allNodes
			// The whole tree starts expanded and every file starts ticked, which
			// is also what a folder's checkbox reports. Applied before the model
			// gets the rows: splicing binds the visible ones immediately, and a
			// row bound first would show the zero value.
			for _, node := range allNodes {
				node.match = true
				node.selected = true
			}
			matchedNodes = append(matchedNodes[:0], allNodes...)
			matchCount = len(fileNodes)
			rows.Splice(0, 0, keys)
			refreshFilter()
			state = filePickerReady
			downloadButton.SetSensitive(true)
			allButton.SetSensitive(true)
			noneButton.SetSensitive(true)
			expandButton.SetSensitive(true)
			collapseButton.SetSensitive(true)
			searchEntry.SetSensitive(true)
			if updateHint != nil {
				updateHint()
			}
		})
	}()
}

const (
	filePickerLoading = iota
	filePickerReady
	filePickerHandedOff
	filePickerAborted
)

// fetchTitleFileTree is swapped by the smoke harness so the picker can render a
// synthetic tree without touching the network.
var fetchTitleFileTree = wiiudownloader.FetchTitleFileTree

// lastTitleFileNodes is the most recently built picker tree, so the smoke
// harness can assert the rows are actually on screen.
var lastTitleFileNodes []*titleFileNode

// lastTitleFilePicker exposes the most recent picker's controls and work counters
// to the smoke run, so a bulk change is pinned to a bounded number of passes.
var lastTitleFilePicker struct {
	search       *gtk.SearchEntry
	selectAll    *gtk.Button
	selectNone   *gtk.Button
	expandAll    *gtk.Button
	collapseAll  *gtk.Button
	status       *gtk.Label
	model        *gtk.FilterListModel
	store        *gtk.StringList
	view         *gtk.ListView
	query        func() string
	bulkPasses   int
	folderPasses int
	statusPasses int
}

// bulkTitleFileNodes is the set Select All/None act on: every file, or only the
// matching files while a search is active. A collapsed folder never narrows it —
// its rows are off screen, and scoping the buttons to hidden rows is what made
// them look broken on large titles.
func bulkTitleFileNodes(fileNodes []*titleFileNode, query string) []*titleFileNode {
	if query == "" {
		return fileNodes
	}
	shown := make([]*titleFileNode, 0, 32)
	for _, node := range fileNodes {
		if node.match {
			shown = append(shown, node)
		}
	}
	return shown
}

// buildTitleFileNodes turns the flat FST paths into a directory tree. Directory
// prefixes are sliced out of the path rather than rebuilt per level, so the
// 19k-node titles do not allocate a prefix string per directory step.
func buildTitleFileNodes(files []wiiudownloader.TitleFile) ([]*titleFileNode, []*titleFileNode) {
	roots := make([]*titleFileNode, 0)
	dirs := make(map[string]*titleFileNode)
	all := make([]*titleFileNode, 0, len(files)+len(files)/8)

	for _, file := range files {
		var parent *titleFileNode
		depth, end := 0, 0
		for {
			slash := strings.IndexByte(file.Path[end:], '/')
			if slash < 0 {
				break
			}
			end += slash
			prefix := file.Path[:end]
			dir, ok := dirs[prefix]
			if !ok {
				name := prefix
				if last := strings.LastIndexByte(prefix, '/'); last >= 0 {
					name = prefix[last+1:]
				}
				dir = &titleFileNode{path: prefix, name: name, dir: true, depth: depth, parent: parent, expanded: true}
				dirs[prefix] = dir
				if parent == nil {
					roots = append(roots, dir)
				} else {
					parent.children = append(parent.children, dir)
				}
				all = append(all, dir)
			}
			parent = dir
			depth++
			end++
		}

		node := &titleFileNode{
			path:      file.Path,
			name:      file.Path[end:],
			depth:     depth,
			parent:    parent,
			file:      file,
			lowerPath: strings.ToLower(file.Path),
		}
		if parent == nil {
			roots = append(roots, node)
		} else {
			parent.children = append(parent.children, node)
		}
		all = append(all, node)
	}
	return roots, all
}

// chooseTitleFilesTarget asks for an output folder, then extracts the selection.
func (mw *MainWindow) chooseTitleFilesTarget(tree *wiiudownloader.TitleFileTree, paths []string) {
	config, _ := loadConfig()
	chooseFolder(mw.window, WINDOW_TITLE_PREFIX+"Select Download Path", config.LastSelectedPath, func(chosen string) {
		if chosen == "" {
			tree.Close()
			return
		}
		config.LastSelectedPath = chosen
		if saveErr := config.Save(); saveErr != nil {
			ShowErrorDialog(mw.window, saveErr)
		}

		tidStr := fmt.Sprintf("%016x", tree.TitleID)
		titlePath := filepath.Join(chosen, fmt.Sprintf("%s [%s] [%s]", normalizeFilename(tree.Name), wiiudownloader.GetFormattedKind(tree.TitleID), tidStr))
		mw.startTitleFilesRun(tree, paths, titlePath)
	})
}

// startTitleFilesRun reports into the same inline run bar as queue downloads.
func (mw *MainWindow) startTitleFilesRun(tree *wiiudownloader.TitleFileTree, paths []string, outputDir string) {
	run := mw.beginRun(tree.Name)
	run.ClearErrors()

	go func() {
		uiIdleAdd(func() {
			mw.setDownloadControlsSensitive(false)
		})
		defer uiIdleAdd(func() {
			mw.setDownloadControlsSensitive(true)
		})

		tidStr := fmt.Sprintf("%016x", tree.TitleID)
		runErr := tree.DownloadFiles(outputDir, paths, run)

		uiIdleAdd(func() {
			run.Finish()
			tree.Close()
			if runErr != nil {
				run.AddErrorWithType(tree.Name, runErr.Error(), tidStr, detectErrorType(runErr.Error()), tree.Version)
			}
			if errors := run.GetErrors(); len(errors) > 0 {
				mw.showErrorsDialog(errors)
				return
			}
			if !run.Cancelled() {
				showAlert(mw.window, WINDOW_TITLE_PREFIX+"Download Complete", fmt.Sprintf("Extracted %d file(s) to:\n%s", len(paths), outputDir))
			}
		})
	}()
}
