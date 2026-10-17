package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	defaultProviderRequestTimeout = 30 * time.Second
	defaultClusterNamespace       = "sentinel-sync-dns"
	defaultClusterLockTTL         = 15 * time.Second
	defaultClusterEventTTL        = 60 * time.Second
	defaultClusterRenewInterval   = 5 * time.Second
)

type RecordConfig struct {
	Type string `yaml:"type"`
	TTL  int    `yaml:"ttl"`
}

type Duration time.Duration

func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var raw interface{}
	if err := unmarshal(&raw); err != nil {
		return err
	}

	switch value := raw.(type) {
	case int:
		*d = Duration(time.Duration(value) * time.Second)
		return nil
	case int64:
		*d = Duration(time.Duration(value) * time.Second)
		return nil
	case string:
		if value == "" {
			*d = 0
			return nil
		}
		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", value, err)
		}
		*d = Duration(duration)
		return nil
	default:
		return fmt.Errorf("unsupported duration value type: %T", raw)
	}
}

func (d Duration) Std() time.Duration {
	return time.Duration(d)
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

type AliCloudConfig struct {
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
}

type AWSConfig struct {
	AccessKey    string `yaml:"access_key"`
	SecretKey    string `yaml:"secret_key"`
	SessionToken string `yaml:"session_token"`
	Region       string `yaml:"region"`
}

type DNSProviderConfig struct {
	Name           string         `yaml:"name"`
	Type           string         `yaml:"type"`
	Domain         string         `yaml:"domain"`
	ZoneID         string         `yaml:"zone_id"`
	Record         []RecordConfig `yaml:"record"`
	RequestTimeout Duration       `yaml:"request_timeout"`

	HuaweiCloud  *HuaweiCloudConfig  `yaml:"huaweicloud,omitempty"`
	TencentCloud *TencentCloudConfig `yaml:"tencentcloud,omitempty"`
	AliCloud     *AliCloudConfig     `yaml:"alicloud,omitempty"`
	AWS          *AWSConfig          `yaml:"aws,omitempty"`
}

func (c *DNSProviderConfig) ApplyDefaults() {
	if c.RequestTimeout.Std() <= 0 {
		c.RequestTimeout = Duration(defaultProviderRequestTimeout)
	}
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

func (c *DNSProviderConfig) GetAliCloudConfig() (string, string, error) {
	if c.AliCloud == nil {
		return "", "", fmt.Errorf("alicloud configuration is missing")
	}
	return c.AliCloud.AccessKey, c.AliCloud.SecretKey, nil
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type ClusterConfig struct {
	Enabled       bool        `yaml:"enabled"`
	Namespace     string      `yaml:"namespace"`
	NodeName      string      `yaml:"node_name"`
	LockTTL       Duration    `yaml:"lock_ttl"`
	EventTTL      Duration    `yaml:"event_ttl"`
	RenewInterval Duration    `yaml:"renew_interval"`
	Redis         RedisConfig `yaml:"redis"`
}

func (c *ClusterConfig) ApplyDefaults() {
	if c.Namespace == "" {
		c.Namespace = defaultClusterNamespace
	}
	if c.LockTTL.Std() <= 0 {
		c.LockTTL = Duration(defaultClusterLockTTL)
	}
	if c.EventTTL.Std() <= 0 {
		c.EventTTL = Duration(defaultClusterEventTTL)
	}
	if c.RenewInterval.Std() <= 0 {
		c.RenewInterval = Duration(defaultClusterRenewInterval)
	}
	if c.NodeName == "" {
		hostname, err := os.Hostname()
		if err != nil || hostname == "" {
			c.NodeName = "unknown-node"
			return
		}
		c.NodeName = hostname
	}
}

func (c *ClusterConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Redis.Addr == "" {
		return fmt.Errorf("cluster.redis.addr is required when cluster.enabled=true")
	}
	if c.LockTTL.Std() <= 0 {
		return fmt.Errorf("cluster.lock_ttl must be greater than 0")
	}
	if c.EventTTL.Std() <= 0 {
		return fmt.Errorf("cluster.event_ttl must be greater than 0")
	}
	if c.RenewInterval.Std() <= 0 {
		return fmt.Errorf("cluster.renew_interval must be greater than 0")
	}
	if c.RenewInterval.Std() >= c.LockTTL.Std() {
		return fmt.Errorf("cluster.renew_interval must be less than cluster.lock_ttl")
	}
	if c.Namespace == "" {
		return fmt.Errorf("cluster.namespace must not be empty")
	}
	return nil
}

type Config struct {
	Sentinel struct {
		Name       string   `yaml:"name"`
		Host       string   `yaml:"host"`
		Password   string   `yaml:"password"`
		MasterName []string `yaml:"master_name"`
	} `yaml:"sentinel"`
	DNSProviders []DNSProviderConfig `yaml:"dns-providers"`
	Cluster      ClusterConfig       `yaml:"cluster"`
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

	for index := range config.DNSProviders {
		config.DNSProviders[index].ApplyDefaults()
	}

	config.Cluster.ApplyDefaults()
	if err := config.Cluster.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}
