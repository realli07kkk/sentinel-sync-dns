package huaweicloud_private

import (
	"fmt"
	"log"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
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
	// 添加类型检查日志
	if cfg.Type != "huaweicloud-private" {
		return nil, fmt.Errorf("invalid provider type: %s, expected: huaweicloud-private", cfg.Type)
	}

	log.Printf("创建华为云私有DNS Provider: name=%s, domain=%s", cfg.Name, cfg.Domain)

	auth := basic.NewCredentialsBuilder().
		WithAk(cfg.AccessKey).
		WithSk(cfg.SecretKey).
		Build()

	client := dns.NewDnsClient(
		dns.DnsClientBuilder().
			WithRegion(region.ValueOf(cfg.Region)).
			WithCredential(auth).
			Build())

	return &Provider{
		client: client,
		config: cfg,
	}, nil
}

type RecordSet struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	TTL     int      `json:"ttl"`
	Records []string `json:"records"`
}

type ListResponse struct {
	Recordsets []RecordSet `json:"recordsets"`
}

type Provider struct {
	client *dns.DnsClient
	config *config.DNSProviderConfig
}

func (p *Provider) UpdateDNS(masterName, newIP string) error {
	log.Printf("华为云私有DNS Provider[%s] 开始更新记录", p.config.Name)
	fullDomain := fmt.Sprintf("%s.%s", masterName, p.config.Domain)
	log.Printf("开始更新DNS记录: 域名=%s, 新IP=%s", fullDomain, newIP)

	// 查询现有记录
	request := &model.ListRecordSetsByZoneRequest{}
	request.ZoneId = p.config.ZoneID
	searchMode := "equal"
	request.SearchMode = &searchMode
	request.Name = &fullDomain

	log.Printf("查询DNS记录: ZoneID=%s, Domain=%s", p.config.ZoneID, fullDomain)
	response, err := p.client.ListRecordSetsByZone(request)
	if err != nil {
		return fmt.Errorf("查询DNS记录失败: %v", err)
	}

	log.Printf("查询结果: %+v", response)

	// 检查是否有记录返回
	if response.Recordsets == nil || len(*response.Recordsets) == 0 {
		log.Printf("未找到现有记录，准备创建新记录")
		return p.createDNSRecord(fullDomain, newIP)
	}

	// 获取第一个匹配的记录
	recordset := (*response.Recordsets)[0]

	// 准备更新请求
	updateRequest := &model.UpdateRecordSetRequest{}
	updateRequest.ZoneId = p.config.ZoneID

	if recordset.Id == nil {
		return fmt.Errorf("记录集ID为空")
	}
	updateRequest.RecordsetId = *recordset.Id

	ttl := int32(p.config.Record[0].TTL)
	recordType := p.config.Record[0].Type
	records := []string{newIP}

	updateRequest.Body = &model.UpdateRecordSetReq{
		Records: &records,
		Ttl:     &ttl,
		Type:    &recordType,
	}

	// 执行更新操作
	_, err = p.client.UpdateRecordSet(updateRequest)
	if err != nil {
		return fmt.Errorf("更新DNS记录失败: %v", err)
	}

	return nil
}

func (p *Provider) createDNSRecord(fullDomain, newIP string) error {
	log.Printf("开始创建DNS记录: 域名=%s, IP=%s", fullDomain, newIP)

	request := &model.CreateRecordSetRequest{}
	request.ZoneId = p.config.ZoneID

	records := []string{newIP}
	ttl := int32(p.config.Record[0].TTL)
	recordType := p.config.Record[0].Type

	request.Body = &model.CreateRecordSetRequestBody{
		Records: records,
		Ttl:     &ttl,
		Type:    recordType,
		Name:    fullDomain,
	}

	log.Printf("创建请求参数: ZoneID=%s, Type=%s, TTL=%d",
		p.config.ZoneID, recordType, ttl)

	response, err := p.client.CreateRecordSet(request)
	if err != nil {
		log.Printf("创建DNS记录失败: %v", err)
		return fmt.Errorf("创建DNS记录失败: %v", err)
	}

	log.Printf("创建DNS记录成功: %+v", response)
	return nil
}
