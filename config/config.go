package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

type DNSProviderConfig struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	Region    string `yaml:"region"`
	Domain    string `yaml:"domain"`
	ZoneID    string `yaml:"zone_id"`
	Record    []struct {
		Type string `yaml:"type"`
		TTL  int    `yaml:"ttl"`
	} `yaml:"record"`
}

type Config struct {
	Sentinel struct {
		Name       string   `yaml:"name"`
		Host       string   `yaml:"host"`
		Password   string   `yaml:"password"`
		MasterName []string `yaml:"master_name"`
	} `yaml:"sentinel"`
	DNSProviders []DNSProviderConfig `yaml:"dns-providers"`
}

func LoadConfig(path string) (*Config, error) {
	var config Config
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}
