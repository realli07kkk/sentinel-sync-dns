package huaweicloud_private

import (
	"context"
	"fmt"
	"log"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	httpconfig "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	dns "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dns/v2"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dns/v2/model"
	region "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dns/v2/region"
	"github.com/realli07kkk/sentinel-sync-dns/config"
	"github.com/realli07kkk/sentinel-sync-dns/provider"
)

func init() {
	provider.RegisterProvider("huaweicloud-private", &Factory{})
}

type Factory struct{}

func (f *Factory) Create(cfg *config.DNSProviderConfig) (provider.Provider, error) {
	if cfg.Type != "huaweicloud-private" {
		return nil, fmt.Errorf("invalid provider type: %s, expected: huaweicloud-private", cfg.Type)
	}

	accessKey, secretKey, regionStr, err := cfg.GetHuaweiCloudConfig()
	if err != nil {
		return nil, fmt.Errorf("invalid huaweicloud configuration: %v", err)
	}

	if len(cfg.Record) == 0 {
		return nil, fmt.Errorf("record configuration is required")
	}

	if cfg.Record[0].TTL <= 0 {
		return nil, fmt.Errorf("invalid TTL value: %d", cfg.Record[0].TTL)
	}

	recordConfig := RecordConfig{
		Type: cfg.Record[0].Type,
		TTL:  cfg.Record[0].TTL,
	}

	log.Printf("华为云私有DNS Provider[%s]: domain=%s, zone=%s, type=%s, ttl=%d",
		cfg.Name, cfg.Domain, cfg.ZoneID, recordConfig.Type, recordConfig.TTL)

	auth := basic.NewCredentialsBuilder().
		WithAk(accessKey).
		WithSk(secretKey).
		Build()
	httpClientConfig := httpconfig.DefaultHttpConfig().WithTimeout(cfg.RequestTimeout.Std())

	regionId := region.ValueOf(regionStr)
	if regionId == nil {
		return nil, fmt.Errorf("invalid region: %s", regionStr)
	}

	client := dns.NewDnsClient(
		dns.DnsClientBuilder().
			WithRegion(regionId).
			WithCredential(auth).
			WithHttpConfig(httpClientConfig).
			Build())

	if cfg.ZoneID == "" {
		return nil, fmt.Errorf("zone_id is required")
	}

	request := &model.ShowPrivateZoneRequest{}
	request.ZoneId = cfg.ZoneID
	_, err = client.ShowPrivateZone(request)
	if err != nil {
		return nil, fmt.Errorf("验证区域配置[%s]失败: %v", cfg.ZoneID, err)
	}

	return &Provider{
		name:         cfg.Name,
		client:       client,
		config:       cfg,
		zoneId:       cfg.ZoneID,
		recordConfig: recordConfig,
	}, nil
}

type RecordConfig struct {
	Type string
	TTL  int
}

type Provider struct {
	name         string
	client       *dns.DnsClient
	config       *config.DNSProviderConfig
	zoneId       string
	recordConfig RecordConfig
}

func (p *Provider) GetName() string {
	return p.name
}

func (p *Provider) EnsureDNS(ctx context.Context, masterName, newIP string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	log.Printf("华为云私有DNS Provider[%s] 开始更新记录", p.name)
	fullDomain := fmt.Sprintf("%s.%s", masterName, p.config.Domain)
	log.Printf("开始更新DNS记录: 域名=%s, 新IP=%s", fullDomain, newIP)

	request := &model.ListRecordSetsByZoneRequest{}
	request.ZoneId = p.zoneId
	searchMode := "equal"
	request.SearchMode = &searchMode
	request.Name = &fullDomain

	log.Printf("查询DNS记录: ZoneID=%s, Domain=%s", p.zoneId, fullDomain)
	response, err := p.client.ListRecordSetsByZone(request)
	if err != nil {
		return false, fmt.Errorf("查询DNS记录失败: %v", err)
	}

	log.Printf("查询结果: %+v", response)

	if response.Recordsets == nil || len(*response.Recordsets) == 0 {
		log.Printf("未找到现有记录，准备创建新记录")
		return p.createDNSRecord(ctx, fullDomain, newIP)
	}

	recordset := (*response.Recordsets)[0]
	if p.recordMatches(recordset, newIP) {
		log.Printf("华为云私有DNS Provider[%s] 记录已是目标状态，跳过更新", p.name)
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	updateRequest := &model.UpdateRecordSetRequest{}
	updateRequest.ZoneId = p.zoneId

	if recordset.Id == nil {
		return false, fmt.Errorf("记录集ID为空")
	}
	updateRequest.RecordsetId = *recordset.Id

	ttl := int32(p.recordConfig.TTL)
	log.Printf("华为云DNS记录TTL配置值: %d", ttl)
	recordType := p.recordConfig.Type
	records := []string{newIP}

	updateRequest.Body = &model.UpdateRecordSetReq{
		Records: &records,
		Ttl:     &ttl,
		Type:    &recordType,
	}

	_, err = p.client.UpdateRecordSet(updateRequest)
	if err != nil {
		return false, fmt.Errorf("更新DNS记录失败: %v", err)
	}

	return true, nil
}

func (p *Provider) createDNSRecord(ctx context.Context, fullDomain, newIP string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	log.Printf("开始创建DNS记录: 域名=%s, IP=%s", fullDomain, newIP)

	request := &model.CreateRecordSetRequest{}
	request.ZoneId = p.zoneId

	records := []string{newIP}
	ttl := int32(p.recordConfig.TTL)
	log.Printf("华为云DNS记录TTL配置值: %d", ttl)
	recordType := p.recordConfig.Type

	request.Body = &model.CreateRecordSetRequestBody{
		Records: records,
		Ttl:     &ttl,
		Type:    recordType,
		Name:    fullDomain,
	}

	log.Printf("创建请求参数: ZoneID=%s, Type=%s, TTL=%d",
		p.zoneId, recordType, ttl)

	response, err := p.client.CreateRecordSet(request)
	if err != nil {
		log.Printf("创建DNS记录失败: %v", err)
		return false, fmt.Errorf("创建DNS记录失败: %v", err)
	}

	log.Printf("创建DNS记录成功: %+v", response)
	return true, nil
}

func (p *Provider) recordMatches(recordset model.ListRecordSets, newIP string) bool {
	if recordset.Type == nil || *recordset.Type != p.recordConfig.Type {
		return false
	}
	if recordset.Ttl == nil || int(*recordset.Ttl) != p.recordConfig.TTL {
		return false
	}
	if recordset.Records == nil || len(*recordset.Records) != 1 {
		return false
	}
	return (*recordset.Records)[0] == newIP
}
