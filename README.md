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
| Tencent Cloud Private DNS | tencentcloud-private | Private DNS service in Tencent Cloud | ✅ Available |

## Configuration

```yaml
sentinel:
  name: sentinel-1
  host: 1.2.3.4:26379
  password: 
  master_name:
    - codis-demo-1
    - master-sg1

dns-providers:
  - name: huaweicloud-private-1
    type: huaweicloud-private
    domain: demo.com
    zone_id: id
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

## Building

```bash
go mod tidy
go build -o sentinel-sync-dns
```

## Requirements

- Go 1.19 or higher
- Access to Redis Sentinel
- DNS provider credentials
