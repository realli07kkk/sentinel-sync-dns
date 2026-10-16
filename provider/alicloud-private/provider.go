package alicloud_private

import (
	"context"
	"fmt"
	"log"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/pvtz"
	"github.com/realli07kkk/sentinel-sync-dns/config"
	"github.com/realli07kkk/sentinel-sync-dns/provider"
)

func init() {
	provider.RegisterProvider("alicloud-private", &Factory{})
}

type Factory struct{}

func (f *Factory) Create(cfg *config.DNSProviderConfig) (provider.Provider, error) {
	if cfg.Type != "alicloud-private" {
		return nil, fmt.Errorf("invalid provider type: %s, expected: alicloud-private", cfg.Type)
	}

	accessKey, secretKey, err := cfg.GetAliCloudConfig()
	if err != nil {
		return nil, fmt.Errorf("invalid alicloud configuration: %v", err)
	}
	if len(cfg.Record) == 0 {
		return nil, fmt.Errorf("record configuration is required")
	}
	if cfg.Record[0].TTL <= 0 {
		return nil, fmt.Errorf("invalid TTL value: %d", cfg.Record[0].TTL)
	}

	log.Printf("创建阿里云私有DNS Provider: name=%s, domain=%s", cfg.Name, cfg.Domain)

	credential := credentials.NewAccessKeyCredential(accessKey, secretKey)
	// pvtz 是中心化服务，SDK 内置 endpoint pvtz.aliyuncs.com，无需 regionId
	client, err := pvtz.NewClientWithOptions("", sdk.NewConfig().WithTimeout(cfg.RequestTimeout.Std()), credential)
	if err != nil {
		return nil, fmt.Errorf("创建阿里云客户端失败: %v", err)
	}

	return &Provider{
		name:   cfg.Name,
		client: client,
		config: cfg,
	}, nil
}

type Provider struct {
	name   string
	client *pvtz.Client
	config *config.DNSProviderConfig
}

func (p *Provider) GetName() string {
	return p.name
}

func (p *Provider) EnsureDNS(ctx context.Context, masterName, newIP string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	log.Printf("阿里云私有DNS Provider[%s] 开始更新记录", p.name)
	fullDomain := fmt.Sprintf("%s.%s", masterName, p.config.Domain)
	log.Printf("开始更新DNS记录: 域名=%s, 新IP=%s", fullDomain, newIP)

	// 查询现有记录
	request := pvtz.CreateDescribeZoneRecordsRequest()
	request.ZoneId = p.config.ZoneID
	request.Keyword = masterName
	request.SearchMode = "EXACT"
	request.PageSize = requests.NewInteger(100)

	response, err := p.client.DescribeZoneRecords(request)
	if err != nil {
		return false, fmt.Errorf("查询DNS记录失败: %v", err)
	}

	// 查找匹配的记录
	var matchedRecord *pvtz.Record
	for index := range response.Records.Record {
		record := &response.Records.Record[index]
		if record.Rr == masterName && record.Type == p.config.Record[0].Type {
			matchedRecord = record
			log.Printf("找到匹配记录: RecordId=%d, Rr=%s, CurrentValue=%s",
				record.RecordId, record.Rr, record.Value)
			break
		}
	}

	if matchedRecord == nil {
		log.Printf("未找到匹配记录[%s]，准备创建新记录", masterName)
		return p.createDNSRecord(ctx, masterName, newIP)
	}
	if p.recordMatches(matchedRecord, newIP) {
		log.Printf("阿里云私有DNS Provider[%s] 记录已是目标状态，跳过更新", p.name)
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	updateRequest := pvtz.CreateUpdateZoneRecordRequest()
	updateRequest.RecordId = requests.NewInteger(int(matchedRecord.RecordId))
	updateRequest.Rr = masterName
	updateRequest.Type = p.config.Record[0].Type
	updateRequest.Value = newIP
	updateRequest.Ttl = requests.NewInteger(p.config.Record[0].TTL)

	log.Printf("更新记录请求参数: ZoneId=%s, RecordId=%d, Rr=%s, Type=%s, Value=%s, TTL=%d",
		p.config.ZoneID, matchedRecord.RecordId, masterName, p.config.Record[0].Type, newIP, p.config.Record[0].TTL)

	if _, err := p.client.UpdateZoneRecord(updateRequest); err != nil {
		return false, fmt.Errorf("更新DNS记录失败: %v", err)
	}

	log.Printf("DNS记录更新成功")
	return true, nil
}

func (p *Provider) createDNSRecord(ctx context.Context, masterName, newIP string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	request := pvtz.CreateAddZoneRecordRequest()
	request.ZoneId = p.config.ZoneID
	request.Rr = masterName
	request.Type = p.config.Record[0].Type
	request.Value = newIP
	request.Ttl = requests.NewInteger(p.config.Record[0].TTL)

	log.Printf("创建记录请求参数: ZoneId=%s, Rr=%s, Type=%s, Value=%s, TTL=%d",
		p.config.ZoneID, masterName, p.config.Record[0].Type, newIP, p.config.Record[0].TTL)

	response, err := p.client.AddZoneRecord(request)
	if err != nil {
		return false, fmt.Errorf("创建DNS记录失败: %v", err)
	}

	log.Printf("DNS记录创建成功: RecordId=%d", response.RecordId)
	return true, nil
}

func (p *Provider) recordMatches(record *pvtz.Record, newIP string) bool {
	if record == nil {
		return false
	}
	if record.Type != p.config.Record[0].Type {
		return false
	}
	if record.Value != newIP {
		return false
	}
	if record.Ttl != p.config.Record[0].TTL {
		return false
	}
	return true
}
