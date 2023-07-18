package main

import wiiudownloader "github.com/Xpl0itU/WiiUDownloader"

func main() {
	win := NewMainWindow(wiiudownloader.GetTitleEntries(wiiudownloader.TITLE_CATEGORY_GAME))

	win.ShowAll()
	Main()
}
