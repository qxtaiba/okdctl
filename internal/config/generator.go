package config

import (
	"os"
	"path/filepath"
)

func GetGeneratedConfigDir(clusterName string) string {
	return filepath.Join("configs", "generated", clusterName)
}

func CleanGeneratedConfigs(clusterName string) error {
	configDir := GetGeneratedConfigDir(clusterName)
	return os.RemoveAll(configDir)
}
