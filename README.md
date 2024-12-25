# sentinel-sync-dns

A tool to automatically synchronize DNS records when Redis Sentinel master-slave switching occurs.

## Features

- Monitor Redis Sentinel events for master-slave switching
- Automatically update DNS records when master node changes
- Support multiple DNS providers
- Support multiple master nodes monitoring
- Configurable DNS record TTL

## Supported DNS Providers

| Provider | Type | Description | Status |
|----------|------|-------------|---------|
| Huawei Cloud Private DNS | huaweicloud-private | Private DNS service in Huawei Cloud | ✅ Available |

## Configuration

```yaml
sentinel:
  name: sentinel-1                  # Sentinel instance name
  host: 127.0.0.1:26379            # Sentinel address
  password:                        # Optional password
  master_name:                     # List of master names to monitor
    - master-1
    - master-2

dns-providers:                     # List of DNS providers
  - name: provider-1               # Provider instance name
    type: huaweicloud-private     # Provider type
    access_key: your-ak           # Provider credentials
    secret_key: your-sk
    region: cn-east-3             # Provider region
    domain: example.com           # DNS domain
    zone_id: your-zone-id         # DNS zone ID
    record:                       # Record settings
      - type: A
        ttl: 1
```

## Usage

```bash
./sentinel-sync-dns -config=/path/to/config.yaml
```

## Building

```bash
go mod tidy
go build -o sentinel-sync-dns
```

## Requirements

- Go 1.19 or higher
- Access to Redis Sentinel
- DNS provider credentials
