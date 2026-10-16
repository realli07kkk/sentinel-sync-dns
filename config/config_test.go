package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfigDefaultsToSingleMode(t *testing.T) {
	configPath := writeTempConfig(t, `
sentinel:
  host: 127.0.0.1:26379
dns-providers: []
`)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if cfg.Cluster.Enabled {
		t.Fatalf("expected cluster mode disabled by default")
	}
	if cfg.Cluster.LockTTL.Std() != 15*time.Second {
		t.Fatalf("unexpected default lock ttl: %s", cfg.Cluster.LockTTL.Std())
	}
	if cfg.Cluster.Namespace != "sentinel-sync-dns" {
		t.Fatalf("unexpected default cluster namespace: %s", cfg.Cluster.Namespace)
	}
	if cfg.Cluster.EventTTL.Std() != 60*time.Second {
		t.Fatalf("unexpected default event ttl: %s", cfg.Cluster.EventTTL.Std())
	}
	if cfg.Cluster.RenewInterval.Std() != 5*time.Second {
		t.Fatalf("unexpected default renew interval: %s", cfg.Cluster.RenewInterval.Std())
	}
	if cfg.Cluster.NodeName == "" {
		t.Fatalf("expected default node name to be set")
	}
	if len(cfg.DNSProviders) != 0 {
		t.Fatalf("expected no dns providers in this config")
	}
}

func TestLoadConfigRejectsClusterWithoutRedisAddr(t *testing.T) {
	configPath := writeTempConfig(t, `
sentinel:
  host: 127.0.0.1:26379
dns-providers: []
cluster:
  enabled: true
`)

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatalf("expected validation error when cluster redis addr is missing")
	}
}

func TestLoadConfigParsesClusterDurations(t *testing.T) {
	configPath := writeTempConfig(t, `
sentinel:
  host: 127.0.0.1:26379
dns-providers: []
cluster:
  enabled: true
  namespace: sentinel-prod
  node_name: node-a
  lock_ttl: 20s
  event_ttl: 90s
  renew_interval: 4s
  redis:
    addr: 127.0.0.1:6379
`)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if cfg.Cluster.NodeName != "node-a" {
		t.Fatalf("unexpected node name: %s", cfg.Cluster.NodeName)
	}
	if cfg.Cluster.Namespace != "sentinel-prod" {
		t.Fatalf("unexpected namespace: %s", cfg.Cluster.Namespace)
	}
	if cfg.Cluster.LockTTL.Std() != 20*time.Second {
		t.Fatalf("unexpected lock ttl: %s", cfg.Cluster.LockTTL.Std())
	}
	if cfg.Cluster.EventTTL.Std() != 90*time.Second {
		t.Fatalf("unexpected event ttl: %s", cfg.Cluster.EventTTL.Std())
	}
	if cfg.Cluster.RenewInterval.Std() != 4*time.Second {
		t.Fatalf("unexpected renew interval: %s", cfg.Cluster.RenewInterval.Std())
	}
}

func TestLoadConfigAppliesProviderTimeoutDefault(t *testing.T) {
	configPath := writeTempConfig(t, `
sentinel:
  host: 127.0.0.1:26379
dns-providers:
  - name: test
    type: tencentcloud-private
    domain: demo.com
    zone_id: zone-1
    record:
      - type: A
        ttl: 60
    tencentcloud:
      secretId: sid
      secretKey: sk
`)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if got := cfg.DNSProviders[0].RequestTimeout.Std(); got != 30*time.Second {
		t.Fatalf("unexpected default provider timeout: %s", got)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return configPath
}
