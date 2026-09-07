package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	wiiudownloader "github.com/Xpl0itU/WiiUDownloader"
)

type Config struct {
	DarkMode                bool   `json:"darkMode"`
	DecryptContents         bool   `json:"decryptContents"`
	DeleteEncryptedContents bool   `json:"deleteEncryptedContents"`
	DecryptOutputPath       string `json:"decryptOutputPath"`
	ContinueOnError         bool   `json:"continueOnError"`
	SuggestRelatedContent   bool   `json:"suggestRelatedContent"`
	SelectedRegion          uint8  `json:"selectedRegion"`
	DidInitialSetup         bool   `json:"didInitialSetup"`
	LastSelectedPath        string `json:"lastSelectedPath"`
	RememberLastPath        bool   `json:"rememberLastPath"`
	ShowDonationBar         bool   `json:"showDonationBar"`
	GetSizeOnQueue          bool   `json:"getSizeOnQueue"`
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

		data, readErr := os.ReadFile(configPath)
		if readErr != nil {
			log.Printf("error loading config file: %v, writing defaults...\n", readErr)
			if createErr := createDefaultConfigFile(); createErr != nil {
				err = fmt.Errorf("error creating default config file: %w", createErr)
				return
			}
			if data, readErr = os.ReadFile(configPath); readErr != nil {
				err = fmt.Errorf("error loading config file: %w", readErr)
				return
			}
		}

		if umErr := decodeConfig(data, globalConfig); umErr != nil {
			log.Printf("error parsing config file: %v, resetting to defaults\n", umErr)
			if createErr := createDefaultConfigFile(); createErr != nil {
				err = fmt.Errorf("error resetting corrupt config file: %w", createErr)
				return
			}
			globalConfig = getDefaultConfig()
		}

		if globalConfig.SelectedRegion > 7 { // Assuming bitmask 0-7
			log.Printf("Warning: invalid region %d, resetting to default", globalConfig.SelectedRegion)
			globalConfig.SelectedRegion = getDefaultConfig().SelectedRegion
		}
	})

	return globalConfig, err
}

func decodeConfig(data []byte, c *Config) error {
	return json.Unmarshal(data, c)
}

func (c *Config) saveTo(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config to JSON: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, CONFIG_FILE_PERM); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("failed to replace config file: %w", err)
	}
	return nil
}

func (c *Config) Save() error {
	c.saveMutex.Lock()
	defer c.saveMutex.Unlock()
	if c.saveConfigCallback != nil {
		c.saveConfigCallback()
	}

	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDirPath(userConfigDir), CONFIG_DIR_PERM); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}
	return c.saveTo(configFilePath(userConfigDir))
}

func configDirPath(userConfigDir string) string {
	return filepath.Join(userConfigDir, WIIUDOWNLOADER_CONFIG_DIR)
}

func configFilePath(userConfigDir string) string {
	return filepath.Join(configDirPath(userConfigDir), CONFIG_FILENAME)
}
