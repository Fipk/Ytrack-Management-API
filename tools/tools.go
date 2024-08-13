package tools

import (
	"encoding/json"
	"os"
)

type Config struct {
	CampusName string `json:"campusName"`
	Domain     string `json:"domain"`
}

func LoadConfigFromFile(path string) (Config, error) {
	var config Config
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	err = json.Unmarshal(data, &config)
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

func SaveConfigToFile(path string, config Config) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	err = os.WriteFile(path, data, 0644)
	if err != nil {
		return err
	}
	return nil
}
