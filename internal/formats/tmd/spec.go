package tmd

const (
	VersionWii  = 0x00
	VersionWiiU = 0x01
)

const (
	versionOffset      = 0x180
	titleIDOffset      = 0x18C
	titleVersionOffset = 0x1DC
	contentCountOffset = 0x1DE
)

const (
	wiiContentStart  = 0x1E4
	wiiContentStride = 0x24
	wiiHashSize      = 0x14
)

const (
	wiiuContentStart  = 0xB04
	wiiuContentStride = 0x30
	wiiuHashSize      = 0x20
)
