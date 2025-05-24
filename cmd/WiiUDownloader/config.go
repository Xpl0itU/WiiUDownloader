package main

import (
	"log"
	"os"
	"path/filepath"
	"sync"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	DarkMode                bool   `koanf:"darkMode"`
	DecryptContents         bool   `koanf:"decryptContents"`
	DeleteEncryptedContents bool   `koanf:"deleteEncryptedContents"`
	SelectedRegion          uint8  `koanf:"selectedRegion"`
	DidInitialSetup         bool   `koanf:"didInitialSetup"`
	LastSelectedPath        string `koanf:"lastSelectedPath"`
	RememberLastPath        bool   `koanf:"rememberLastPath"`
	saveConfigCallback      func()
	saveMutex               *sync.Mutex
}

const (
	wiiudownloaderConfigDir = "WiiUDownloader"
	configFilename          = "config.json"
)

var globalConfig *Config
var k = koanf.NewWithConf(koanf.Conf{
	Delim: ".",
})

func getDefaultConfig() *Config {
	return &Config{
		DarkMode:                isDarkMode(),
		DecryptContents:         false,
		DeleteEncryptedContents: false,
		SelectedRegion:          wiiudownloader.MCP_REGION_EUROPE | wiiudownloader.MCP_REGION_USA | wiiudownloader.MCP_REGION_JAPAN,
		DidInitialSetup:         false,
		LastSelectedPath:        "",
		RememberLastPath:        false,
		saveConfigCallback:      nil,
		saveMutex:               &sync.Mutex{},
	}
}

func createDefaultConfigFile() error {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(userConfigDir, wiiudownloaderConfigDir), 0755); err != nil {
		return err
	}

	configFile, err := os.Create(filepath.Join(userConfigDir, wiiudownloaderConfigDir, configFilename))
	if err != nil {
		return err
	}
	defer configFile.Close()

	if _, err := configFile.WriteString("{}"); err != nil {
		return err
	}

	return nil
}

func loadConfig() (*Config, error) {
	if globalConfig != nil {
		return globalConfig, nil
	}

	globalConfig = getDefaultConfig()

	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatalf("error getting user config dir: %v", err)
	}

	if err := k.Load(file.Provider(filepath.Join(userConfigDir, wiiudownloaderConfigDir, configFilename)), json.Parser()); err != nil {
		log.Printf("error loading config file: %v, writing defaults...\n", err)
		if err := createDefaultConfigFile(); err != nil {
			log.Fatalf("error creating default config file: %v", err)
		}
		if err := k.Load(file.Provider(filepath.Join(userConfigDir, wiiudownloaderConfigDir, configFilename)), json.Parser()); err != nil {
			log.Fatalf("error loading config file: %v", err)
		}
	}

	globalConfig.SetValuesFromConfig(k)

	return globalConfig, nil
}

func (c *Config) Save() error {
	c.saveMutex.Lock()
	defer c.saveMutex.Unlock()
	if c.saveConfigCallback != nil {
		c.saveConfigCallback()
	}

	if err := k.Load(structs.Provider(c, "koanf"), nil); err != nil {
		return err
	}

	// write the config to the file
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	confBytes, err := k.Marshal(json.Parser())
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(userConfigDir, wiiudownloaderConfigDir, configFilename), confBytes, 0644)
}

func (c *Config) SetValuesFromConfig(newK *koanf.Koanf) {
	c.saveMutex.Lock()
	defer c.saveMutex.Unlock()
	if err := newK.Unmarshal("", c); err != nil {
		log.Fatalf("error setting values from config: %v", err)
	}
}
