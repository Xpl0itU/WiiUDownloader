module github.com/Xpl0itU/WiiUDownloader/cmd/WiiUDownloader

go 1.24.0

toolchain go1.24.3

require (
	github.com/Xpl0itU/WiiUDownloader v0.0.0-00010101000000-000000000000
	github.com/Xpl0itU/dialog v0.0.0-20230805114139-ec888310aded
	github.com/dustin/go-humanize v1.0.1
	github.com/gotk3/gotk3 v0.6.5-0.20240618185848-ff349ae13f56
	github.com/knadh/koanf/parsers/json v1.0.0
	github.com/knadh/koanf/providers/file v1.2.1
	github.com/knadh/koanf/providers/structs v1.0.0
	github.com/knadh/koanf/v2 v2.3.0
	golang.org/x/sync v0.19.0
)

require (
	github.com/TheTitanrain/w32 v0.0.0-20200114052255-2654d97dbd3d // indirect
	github.com/fatih/structs v1.1.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
)

replace github.com/Xpl0itU/WiiUDownloader => ../..
