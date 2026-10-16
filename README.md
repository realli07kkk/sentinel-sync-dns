# sentinel-sync-dns

A tool to automatically synchronize DNS records when Redis Sentinel master-slave switching occurs.

## Features

- Monitor Redis Sentinel events for master-slave switching
- Automatically update DNS records when master node changes
- Support multiple DNS providers
- Support multiple master nodes monitoring
- Support single-node mode and optional clustered mode
- Configurable DNS record TTL

## Supported DNS Providers

| Provider | Type | Description | Status |
|----------|------|-------------|---------|
| Huawei Cloud Private DNS | huaweicloud-private | Private DNS service in Huawei Cloud | ✅ Available |
| Tencent Cloud Private DNS | tencentcloud-private | Private DNS service in Tencent Cloud | ✅ Available |

## Configuration

```yaml
sentinel:
  name: sentinel-1
  host: 1.2.3.4:26379,1.2.3.5:26379
  password: 
  master_name:
    - codis-demo-1
    - master-sg1

# Optional clustered mode.
# When enabled, all nodes can subscribe to Sentinel, but only one node
# will update DNS for the same master switch event at a time.
cluster:
  enabled: true
  namespace: sentinel-sync-dns
  node_name: sentinel-sync-dns-01
  lock_ttl: 15s
  event_ttl: 60s
  renew_interval: 5s
  redis:
    addr: 127.0.0.1:6379
    password: ""
    db: 0

dns-providers:
  - name: huaweicloud-private-1
    type: huaweicloud-private
    domain: demo.com
    zone_id: id
    request_timeout: 30s
    record:
      - type: A
        ttl: 1
    huaweicloud:
      access_key: xxx
      secret_key: xxx
      region: cn-east-3
  
  - name: tencentcloud-private-1
    type: tencentcloud-private
    domain: demo.com
    zone_id: id
    request_timeout: 30s
    record:
      - type: A
        ttl: 300
    tencentcloud:
      secretId: xxx
      secretKey: xxx
```

## Usage

```bash
./sentinel-sync-dns -config=/path/to/config.yaml
```

If `cluster.enabled` is omitted or set to `false`, the service runs in the original single-node mode.
When `cluster.enabled=true`, use an independent Redis instance for distributed locking and event de-duplication.
`cluster.namespace` isolates lock and done keys between deployments that share the same Redis.
`request_timeout` controls each DNS provider SDK request timeout; if omitted, the default is `30s`.

## Building

```bash
go mod tidy
go build -o sentinel-sync-dns
```

## Requirements

- Go 1.19 or higher
- Access to Redis Sentinel
- DNS provider credentials
