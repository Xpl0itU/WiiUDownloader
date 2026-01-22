package main

import (
	"fmt"
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
	ContinueOnError         bool   `koanf:"continueOnError"`
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

var (
	globalConfig     *Config
	globalConfigOnce sync.Once
	k                = koanf.NewWithConf(koanf.Conf{
		Delim: ".",
	})
)

func getDefaultConfig() *Config {
	return &Config{
		DarkMode:                isDarkMode(),
		DecryptContents:         false,
		DeleteEncryptedContents: false,
		ContinueOnError:         true,
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
	var err error
	globalConfigOnce.Do(func() {
		globalConfig = getDefaultConfig()

		userConfigDir, errConf := os.UserConfigDir()
		if errConf != nil {
			log.Printf("error getting user config dir: %v", errConf)
			err = errConf
			return
		}

		if errConf := k.Load(file.Provider(filepath.Join(userConfigDir, wiiudownloaderConfigDir, configFilename)), json.Parser()); errConf != nil {
			log.Printf("error loading config file: %v, writing defaults...\n", errConf)
			if errConf := createDefaultConfigFile(); errConf != nil {
				err = fmt.Errorf("error creating default config file: %w", errConf)
				return
			}
			if errConf := k.Load(file.Provider(filepath.Join(userConfigDir, wiiudownloaderConfigDir, configFilename)), json.Parser()); errConf != nil {
				err = fmt.Errorf("error loading config file: %w", errConf)
				return
			}
		}

		if errConf := globalConfig.SetValuesFromConfig(k); errConf != nil {
			err = fmt.Errorf("error setting values from config: %w", errConf)
			return
		}
	})

	return globalConfig, err
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

func (c *Config) SetValuesFromConfig(newK *koanf.Koanf) error {
	c.saveMutex.Lock()
	defer c.saveMutex.Unlock()
	if err := newK.Unmarshal("", c); err != nil {
		return err
	}
	return nil
}
