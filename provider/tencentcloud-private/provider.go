package tencentcloud_private

import (
	"context"
	"fmt"
	"log"
	"math"

	"github.com/realli07kkk/sentinel-sync-dns/config"
	"github.com/realli07kkk/sentinel-sync-dns/provider"
	common "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	privatedns "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/privatedns/v20201028"
)

func init() {
	provider.RegisterProvider("tencentcloud-private", &Factory{})
}

type Factory struct{}

func (f *Factory) Create(cfg *config.DNSProviderConfig) (provider.Provider, error) {
	if cfg.Type != "tencentcloud-private" {
		return nil, fmt.Errorf("invalid provider type: %s, expected: tencentcloud-private", cfg.Type)
	}

	secretId, secretKey, err := cfg.GetTencentCloudConfig()
	if err != nil {
		return nil, fmt.Errorf("invalid tencentcloud configuration: %v", err)
	}
	if len(cfg.Record) == 0 {
		return nil, fmt.Errorf("record configuration is required")
	}
	if cfg.Record[0].TTL <= 0 {
		return nil, fmt.Errorf("invalid TTL value: %d", cfg.Record[0].TTL)
	}

	log.Printf("创建腾讯云私有DNS Provider: name=%s, domain=%s", cfg.Name, cfg.Domain)

	credential := common.NewCredential(secretId, secretKey)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.Endpoint = "privatedns.tencentcloudapi.com"
	timeoutSeconds := int(math.Ceil(cfg.RequestTimeout.Std().Seconds()))
	if timeoutSeconds < 1 {
		timeoutSeconds = 1
	}
	cpf.HttpProfile.ReqTimeout = timeoutSeconds

	client, err := privatedns.NewClient(credential, "", cpf)
	if err != nil {
		return nil, fmt.Errorf("创建腾讯云客户端失败: %v", err)
	}

	return &Provider{
		name:   cfg.Name,
		client: client,
		config: cfg,
	}, nil
}

type Provider struct {
	name   string
	client *privatedns.Client
	config *config.DNSProviderConfig
}

func (p *Provider) GetName() string {
	return p.name
}

func (p *Provider) EnsureDNS(ctx context.Context, masterName, newIP string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	log.Printf("腾讯云私有DNS Provider[%s] 开始更新记录", p.name)
	fullDomain := fmt.Sprintf("%s.%s", masterName, p.config.Domain)
	log.Printf("开始更新DNS记录: 域名=%s, 新IP=%s", fullDomain, newIP)

	// 查询现有记录
	request := privatedns.NewDescribePrivateZoneRecordListRequest()
	request.ZoneId = common.StringPtr(p.config.ZoneID)
	request.Filters = []*privatedns.Filter{
		{
			Name:   common.StringPtr("SubDomain"),
			Values: common.StringPtrs([]string{masterName}),
		},
	}

	response, err := p.client.DescribePrivateZoneRecordList(request)
	if err != nil {
		return false, fmt.Errorf("查询DNS记录失败: %v", err)
	}

	log.Printf("查询结果：%s", response.ToJsonString())

	// 查找匹配的记录
	var matchedRecord *privatedns.PrivateZoneRecord
	if response.Response != nil && len(response.Response.RecordSet) > 0 {
		for _, record := range response.Response.RecordSet {
			if record.SubDomain != nil && *record.SubDomain == masterName {
				matchedRecord = record
				log.Printf("找到匹配记录: RecordId=%s, SubDomain=%s, CurrentValue=%s",
					*record.RecordId, *record.SubDomain, *record.RecordValue)
				break
			}
		}
	}

	if matchedRecord == nil {
		log.Printf("未找到匹配记录[%s]，准备创建新记录", masterName)
		return p.createDNSRecord(ctx, masterName, newIP)
	}
	if p.recordMatches(matchedRecord, newIP) {
		log.Printf("腾讯云私有DNS Provider[%s] 记录已是目标状态，跳过更新", p.name)
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	updateRequest := privatedns.NewModifyPrivateZoneRecordRequest()
	updateRequest.ZoneId = common.StringPtr(p.config.ZoneID)
	updateRequest.RecordId = matchedRecord.RecordId
	updateRequest.SubDomain = common.StringPtr(masterName)
	updateRequest.RecordType = common.StringPtr(p.config.Record[0].Type)
	updateRequest.RecordValue = common.StringPtr(newIP)
	updateRequest.TTL = common.Int64Ptr(int64(p.config.Record[0].TTL))

	log.Printf("更新记录请求参数: ZoneId=%s, RecordId=%s, SubDomain=%s, Type=%s, Value=%s, TTL=%d",
		p.config.ZoneID, *matchedRecord.RecordId, masterName, p.config.Record[0].Type, newIP, p.config.Record[0].TTL)

	_, err = p.client.ModifyPrivateZoneRecord(updateRequest)
	if err != nil {
		return false, fmt.Errorf("更新DNS记录失败: %v", err)
	}

	log.Printf("DNS记录更新成功")
	return true, nil
}

func (p *Provider) createDNSRecord(ctx context.Context, masterName, newIP string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	request := privatedns.NewCreatePrivateZoneRecordRequest()
	request.ZoneId = common.StringPtr(p.config.ZoneID)
	request.SubDomain = common.StringPtr(masterName)
	request.RecordType = common.StringPtr(p.config.Record[0].Type)
	request.RecordValue = common.StringPtr(newIP)
	request.TTL = common.Int64Ptr(int64(p.config.Record[0].TTL))

	log.Printf("创建记录请求参数: ZoneId=%s, SubDomain=%s, Type=%s, Value=%s, TTL=%d",
		p.config.ZoneID, masterName, p.config.Record[0].Type, newIP, p.config.Record[0].TTL)

	response, err := p.client.CreatePrivateZoneRecord(request)
	if err != nil {
		if sdkErr, ok := err.(*errors.TencentCloudSDKError); ok {
			return false, fmt.Errorf("创建DNS记录失败[%s]: %s", sdkErr.Code, sdkErr.Message)
		}
		return false, fmt.Errorf("创建DNS记录失败: %v", err)
	}

	log.Printf("DNS记录创建成功: %s", response.ToJsonString())
	return true, nil
}

func (p *Provider) recordMatches(record *privatedns.PrivateZoneRecord, newIP string) bool {
	if record == nil {
		return false
	}
	if record.RecordType == nil || *record.RecordType != p.config.Record[0].Type {
		return false
	}
	if record.RecordValue == nil || *record.RecordValue != newIP {
		return false
	}
	if record.TTL == nil || int(*record.TTL) != p.config.Record[0].TTL {
		return false
	}
	return true
}
