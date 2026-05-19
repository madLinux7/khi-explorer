package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Format       string `yaml:"format"`
	DownloadPath string `yaml:"download_path"`
	Player       string `yaml:"player"`
}

func GetConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".khi_explorer.yaml")
}

func LoadConfig() (Config, error) {
	cfg := Config{
		Format:       "flac",
		DownloadPath: filepath.Join(getHomeDir(), "khi_explorer"),
		Player:       "mpv",
	}

	path := GetConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	err = yaml.Unmarshal(data, &cfg)
	return cfg, err
}

func (c Config) Save() error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(GetConfigPath(), data, 0644)
}

func getHomeDir() string {
	home, _ := os.UserHomeDir()
	return home
}
