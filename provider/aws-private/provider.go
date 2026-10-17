package aws_private

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/realli07kkk/sentinel-sync-dns/config"
	"github.com/realli07kkk/sentinel-sync-dns/provider"
)

const defaultAWSRegion = "us-east-1"

func init() {
	provider.RegisterProvider("aws-private", &Factory{})
}

type Factory struct{}

func (f *Factory) Create(cfg *config.DNSProviderConfig) (provider.Provider, error) {
	if cfg.Type != "aws-private" {
		return nil, fmt.Errorf("invalid provider type: %s, expected: aws-private", cfg.Type)
	}
	if cfg.Domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	if cfg.ZoneID == "" {
		return nil, fmt.Errorf("zone_id is required")
	}
	if len(cfg.Record) == 0 {
		return nil, fmt.Errorf("record configuration is required")
	}
	if cfg.Record[0].TTL <= 0 {
		return nil, fmt.Errorf("invalid TTL value: %d", cfg.Record[0].TTL)
	}

	log.Printf("创建AWS私有DNS Provider: name=%s, domain=%s, zone=%s",
		cfg.Name, cfg.Domain, cfg.ZoneID)

	awsCfg, err := loadAWSConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &Provider{
		name:   cfg.Name,
		client: route53.NewFromConfig(awsCfg),
		config: cfg,
	}, nil
}

type Provider struct {
	name   string
	client *route53.Client
	config *config.DNSProviderConfig
}

func (p *Provider) GetName() string {
	return p.name
}

func (p *Provider) EnsureDNS(ctx context.Context, masterName, newIP string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	log.Printf("AWS私有DNS Provider[%s] 开始更新记录", p.name)
	recordName := p.recordName(masterName)
	log.Printf("开始更新DNS记录: 域名=%s, 新IP=%s", recordName, newIP)

	record, err := p.findRecord(ctx, recordName)
	if err != nil {
		return false, fmt.Errorf("查询DNS记录失败: %v", err)
	}
	if record != nil && p.recordMatches(record, recordName, newIP) {
		log.Printf("AWS私有DNS Provider[%s] 记录已是目标状态，跳过更新", p.name)
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}

	recordType := types.RRType(p.config.Record[0].Type)
	ttl := int64(p.config.Record[0].TTL)
	log.Printf("更新记录请求参数: ZoneId=%s, Name=%s, Type=%s, Value=%s, TTL=%d",
		p.config.ZoneID, recordName, recordType, newIP, ttl)

	response, err := p.client.ChangeResourceRecordSets(ctx, &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(p.config.ZoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionUpsert,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            aws.String(recordName),
						Type:            recordType,
						TTL:             aws.Int64(ttl),
						ResourceRecords: []types.ResourceRecord{{Value: aws.String(newIP)}},
					},
				},
			},
		},
	})
	if err != nil {
		return false, fmt.Errorf("更新DNS记录失败: %v", err)
	}

	changeID := ""
	if response.ChangeInfo != nil && response.ChangeInfo.Id != nil {
		changeID = *response.ChangeInfo.Id
	}
	log.Printf("DNS记录更新成功: ChangeId=%s", changeID)
	return true, nil
}

func (p *Provider) findRecord(ctx context.Context, recordName string) (*types.ResourceRecordSet, error) {
	response, err := p.client.ListResourceRecordSets(ctx, &route53.ListResourceRecordSetsInput{
		HostedZoneId:    aws.String(p.config.ZoneID),
		StartRecordName: aws.String(recordName),
		StartRecordType: types.RRType(p.config.Record[0].Type),
		MaxItems:        aws.Int32(1),
	})
	if err != nil {
		return nil, err
	}
	if len(response.ResourceRecordSets) == 0 {
		return nil, nil
	}
	record := &response.ResourceRecordSets[0]
	if record.Name == nil || !sameDNSName(*record.Name, recordName) {
		return nil, nil
	}
	return record, nil
}

func (p *Provider) recordName(masterName string) string {
	name := fmt.Sprintf("%s.%s", masterName, p.config.Domain)
	return strings.TrimSuffix(name, ".") + "."
}

func (p *Provider) recordMatches(record *types.ResourceRecordSet, recordName, newIP string) bool {
	if record == nil || record.Name == nil {
		return false
	}
	if !sameDNSName(*record.Name, recordName) {
		return false
	}
	if !strings.EqualFold(string(record.Type), p.config.Record[0].Type) {
		return false
	}
	if record.TTL == nil || int(*record.TTL) != p.config.Record[0].TTL {
		return false
	}
	if len(record.ResourceRecords) != 1 || record.ResourceRecords[0].Value == nil {
		return false
	}
	return *record.ResourceRecords[0].Value == newIP
}

func sameDNSName(left, right string) bool {
	return strings.EqualFold(strings.TrimSuffix(left, "."), strings.TrimSuffix(right, "."))
}

func loadAWSConfig(cfg *config.DNSProviderConfig) (aws.Config, error) {
	var options []func(*awsconfig.LoadOptions) error

	if cfg.AWS != nil {
		if cfg.AWS.Region != "" {
			options = append(options, awsconfig.WithRegion(cfg.AWS.Region))
		}
		if cfg.AWS.AccessKey != "" || cfg.AWS.SecretKey != "" {
			if cfg.AWS.AccessKey == "" || cfg.AWS.SecretKey == "" {
				return aws.Config{}, fmt.Errorf("aws.access_key and aws.secret_key must be configured together")
			}
			options = append(options, awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(cfg.AWS.AccessKey, cfg.AWS.SecretKey, cfg.AWS.SessionToken),
			))
		}
	}

	options = append(options, awsconfig.WithHTTPClient(&http.Client{Timeout: cfg.RequestTimeout.Std()}))

	ctx, cancel := context.WithTimeout(context.Background(), cfg.RequestTimeout.Std())
	defer cancel()

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return aws.Config{}, fmt.Errorf("加载AWS配置失败: %v", err)
	}
	if awsCfg.Region == "" {
		awsCfg.Region = defaultAWSRegion
	}
	return awsCfg, nil
}
