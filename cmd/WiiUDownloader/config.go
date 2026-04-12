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
	SuggestRelatedContent   bool   `koanf:"suggestRelatedContent"`
	SelectedRegion          uint8  `koanf:"selectedRegion"`
	DidInitialSetup         bool   `koanf:"didInitialSetup"`
	LastSelectedPath        string `koanf:"lastSelectedPath"`
	RememberLastPath        bool   `koanf:"rememberLastPath"`
	ShowDonationBar         bool   `koanf:"showDonationBar"`
	GetSizeOnQueue          bool   `koanf:"getSizeOnQueue"`
	saveConfigCallback      func()
	saveMutex               *sync.Mutex
}

const (
	WIIUDOWNLOADER_CONFIG_DIR = "WiiUDownloader"
	CONFIG_FILENAME           = "config.json"
	CONFIG_DIR_PERM           = 0o755
	CONFIG_FILE_PERM          = 0o644
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
		SuggestRelatedContent:   true,
		SelectedRegion:          wiiudownloader.MCP_REGION_EUROPE | wiiudownloader.MCP_REGION_USA | wiiudownloader.MCP_REGION_JAPAN,
		DidInitialSetup:         false,
		RememberLastPath:        false,
		ShowDonationBar:         true,
		GetSizeOnQueue:          true,
		saveConfigCallback:      nil,
		saveMutex:               &sync.Mutex{},
	}
}

func createDefaultConfigFile() error {
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(configDirPath(userConfigDir), CONFIG_DIR_PERM); err != nil {
		return err
	}

	configFile, err := os.Create(configFilePath(userConfigDir))
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

		configPath := configFilePath(userConfigDir)
		if errConf := k.Load(file.Provider(configPath), json.Parser()); errConf != nil {
			log.Printf("error loading config file: %v, writing defaults...\n", errConf)
			if errConf := createDefaultConfigFile(); errConf != nil {
				err = fmt.Errorf("error creating default config file: %w", errConf)
				return
			}
			if errConf := k.Load(file.Provider(configPath), json.Parser()); errConf != nil {
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
		return fmt.Errorf("failed to load struct into koanf: %w", err)
	}

	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	confBytes, err := k.Marshal(json.Parser())
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON: %w", err)
	}
	if err := os.WriteFile(configFilePath(userConfigDir), confBytes, CONFIG_FILE_PERM); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

func (c *Config) SetValuesFromConfig(newK *koanf.Koanf) error {
	c.saveMutex.Lock()
	defer c.saveMutex.Unlock()
	if err := newK.Unmarshal("", c); err != nil {
		return err
	}
	if !newK.Exists("suggestRelatedContent") {
		c.SuggestRelatedContent = true
	}
	if !newK.Exists("showDonationBar") {
		c.ShowDonationBar = true
	}
	return nil
}

func configDirPath(userConfigDir string) string {
	return filepath.Join(userConfigDir, WIIUDOWNLOADER_CONFIG_DIR)
}

func configFilePath(userConfigDir string) string {
	return filepath.Join(configDirPath(userConfigDir), CONFIG_FILENAME)
}
