# WiiUDownloader

WiiUDownloader is a Golang program that allows you to download Wii U games from Nintendo's servers. It provides a user-friendly GUI built using GTK to browse and download Wii U titles directly to your local storage. Additionally, it supports decryption of downloaded contents for use on your Wii U console.

## Features

- Browse and search for Wii U games, updates, DLC, demos, and more.
- Download selected titles or queue multiple titles for batch download.
- Decrypt downloaded contents for use on your Wii U console.
- Delete encrypted contents after decryption (optional).
- Filter titles based on name or title ID.
- Select regions (Japan, USA, and Europe) to filter available titles.

## Installation

To install WiiUDownloader, download the appropriate binary for your operating system from the links below:

- [WiiUDownloader-Linux-x86_64.AppImage](https://github.com/Xpl0itU/WiiUDownloader/releases/latest/download/WiiUDownloader-Linux-x86_64.AppImage)
- [WiiUDownloader-macOS-Universal.dmg](https://github.com/Xpl0itU/WiiUDownloader/releases/latest/download/WiiUDownloader-macOS-Universal.dmg)
- [WiiUDownloader-Windows.zip](https://github.com/Xpl0itU/WiiUDownloader/releases/latest/download/WiiUDownloader-Windows.zip)

For Linux, you may need to give execution permission to the downloaded binary:

```bash
chmod +x WiiUDownloader-Linux-x86_64.AppImage   # For Linux
```

## Usage

1. Double-click the downloaded binary to launch WiiUDownloader.
2. The WiiUDownloader GUI window will appear, showing a list of available Wii U titles.
3. Use the search bar to filter titles by name or title ID.
4. Click on the category buttons to filter titles by type (Game, Update, DLC, Demo, All).
5. Click on the checkboxes to select the desired region(s) for filtering (Japan, USA, Europe).
6. Click on the "Add to queue" button to add selected titles to the download queue. The button label will change to "Remove from queue" if titles are already in the queue.
7. Click on the "Download queue" button to choose a location to save the downloaded games. The program will start downloading the queued titles.
8. If you enable "Decrypt contents," the program will decrypt the downloaded files. You can also choose to delete encrypted contents after decryption (optional).
9. If you already have downloaded files that aren't decrypted, you can go to Tools > Decrypt Contents and select the folder to decrypt.

## Important Notes

- WiiUDownloader provides access to Nintendo's servers for downloading titles. Please make sure to follow all legal and ethical guidelines when using this program.
- Downloading and using copyrighted material without proper authorization may violate copyright laws in your country.

## License

This program is distributed under the GPLv3 License. For more information, see the [LICENSE](LICENSE) file.

## Acknowledgments

WiiUDownloader uses several open-source libraries and dependencies to provide its functionality:

- [github.com/gotk3/gotk3](https://github.com/gotk3/gotk3): Go bindings for GTK+3.
