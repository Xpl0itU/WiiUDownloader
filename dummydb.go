package wiiudownloader

type TitleEntry struct {
	Name     string
	TitleID  uint64
	Region   uint8
	Key      uint8
	Category uint8
}

var titleEntry = []TitleEntry{}
