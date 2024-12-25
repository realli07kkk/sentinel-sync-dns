package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

type RecordConfig struct {
	Type string `yaml:"type"`
	TTL  int    `yaml:"ttl"`
}

type HuaweiCloudConfig struct {
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	Region    string `yaml:"region"`
}

type TencentCloudConfig struct {
	SecretId  string `yaml:"secretId"`
	SecretKey string `yaml:"secretKey"`
}

type DNSProviderConfig struct {
	Name   string         `yaml:"name"`
	Type   string         `yaml:"type"`
	Domain string         `yaml:"domain"`
	ZoneID string         `yaml:"zone_id"`
	Record []RecordConfig `yaml:"record"`

	HuaweiCloud  *HuaweiCloudConfig  `yaml:"huaweicloud,omitempty"`
	TencentCloud *TencentCloudConfig `yaml:"tencentcloud,omitempty"`
}

func (c *DNSProviderConfig) GetHuaweiCloudConfig() (string, string, string, error) {
	if c.HuaweiCloud == nil {
		return "", "", "", fmt.Errorf("huaweicloud configuration is missing")
	}
	return c.HuaweiCloud.AccessKey, c.HuaweiCloud.SecretKey, c.HuaweiCloud.Region, nil
}

func (c *DNSProviderConfig) GetTencentCloudConfig() (string, string, error) {
	if c.TencentCloud == nil {
		return "", "", fmt.Errorf("tencentcloud configuration is missing")
	}
	return c.TencentCloud.SecretId, c.TencentCloud.SecretKey, nil
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
