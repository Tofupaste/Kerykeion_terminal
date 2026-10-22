package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	DisplayName           string `json:"display_name"`
	TCPPort               int    `json:"tcp_port"`
	UDPPort               int    `json:"udp_port"`
	ConfigDir             string `json:"config_dir"`
	DataDir               string `json:"data_dir"`
	DownloadsDir          string `json:"downloads_dir"`
	PartialDir            string `json:"partial_dir"`
	CacheDir              string `json:"cache_dir"`
	MaxFileSize           int64  `json:"max_file_size"`
	AutoDownloadImageMax  int64  `json:"auto_download_image_max"`
	RetentionDays         int    `json:"retention_days"`
	OfficeKey             string `json:"office_key,omitempty"`
}

func DefaultConfig() (*Config, error) {
	userConfig, err := os.UserConfigDir()
	if err != nil {
		userConfig = "."
	}
	userCache, err := os.UserCacheDir()
	if err != nil {
		userCache = userConfig
	}

	configDir := filepath.Join(userConfig, "kantorchat")
	dataDir := filepath.Join(userCache, "kantorchat")
	downloadsDir := filepath.Join(dataDir, "downloads")
	partialDir := filepath.Join(downloadsDir, ".partial")
	cacheDir := filepath.Join(dataDir, "cache")

	return &Config{
		DisplayName:          "",
		TCPPort:              42100,
		UDPPort:              42101,
		ConfigDir:            configDir,
		DataDir:              dataDir,
		DownloadsDir:         downloadsDir,
		PartialDir:           partialDir,
		CacheDir:             cacheDir,
		MaxFileSize:          1024 * 1024 * 1024, // 1 GB
		AutoDownloadImageMax: 5 * 1024 * 1024,    // 5 MB
		RetentionDays:        30,
	}, nil
}

func (c *Config) EnsureDirs() error {
	for _, dir := range []string{c.ConfigDir, c.DataDir, c.DownloadsDir, c.PartialDir, c.CacheDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return nil
}

func LoadConfig(customDir string) (*Config, error) {
	cfg, err := DefaultConfig()
	if err != nil {
		return nil, err
	}

	if customDir != "" {
		cfg.ConfigDir = customDir
		cfg.DataDir = customDir
		cfg.DownloadsDir = filepath.Join(customDir, "downloads")
		cfg.PartialDir = filepath.Join(cfg.DownloadsDir, ".partial")
		cfg.CacheDir = filepath.Join(customDir, "cache")
	}

	if err := cfg.EnsureDirs(); err != nil {
		return nil, err
	}

	configFile := filepath.Join(cfg.ConfigDir, "config.json")
	data, err := os.ReadFile(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	var stored Config
	if err := json.Unmarshal(data, &stored); err != nil {
		return cfg, nil
	}

	if stored.DisplayName != "" {
		cfg.DisplayName = stored.DisplayName
	}
	if stored.TCPPort != 0 {
		cfg.TCPPort = stored.TCPPort
	}
	if stored.UDPPort != 0 {
		cfg.UDPPort = stored.UDPPort
	}
	if stored.MaxFileSize != 0 {
		cfg.MaxFileSize = stored.MaxFileSize
	}
	if stored.AutoDownloadImageMax != 0 {
		cfg.AutoDownloadImageMax = stored.AutoDownloadImageMax
	}
	if stored.RetentionDays != 0 {
		cfg.RetentionDays = stored.RetentionDays
	}
	cfg.OfficeKey = stored.OfficeKey

	return cfg, nil
}

func (c *Config) Save() error {
	if err := c.EnsureDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	configFile := filepath.Join(c.ConfigDir, "config.json")
	return os.WriteFile(configFile, data, 0600)
}
