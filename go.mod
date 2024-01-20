module github.com/Xpl0itU/WiiUDownloader

go 1.20

require (
	github.com/Xpl0itU/dialog v0.0.0-20230805114139-ec888310aded
	github.com/dustin/go-humanize v1.0.1
	github.com/gotk3/gotk3 v0.6.2
	golang.org/x/crypto v0.17.0
)

require github.com/ianlancetaylor/cgosymbolizer v0.0.0-20231130194700-cfcb2fd150eb // indirect

require (
	github.com/benesch/cgosymbolizer v0.0.0-20190515212042-bec6fe6e597b // indirect
	github.com/jaskaranSM/aria2go v0.0.0-20210417130736-a4fd19b6cb10
)

replace github.com/jaskaranSM/aria2go => ./pkg/aria2go

require (
	github.com/TheTitanrain/w32 v0.0.0-20200114052255-2654d97dbd3d // indirect
	golang.org/x/sync v0.5.0
)
