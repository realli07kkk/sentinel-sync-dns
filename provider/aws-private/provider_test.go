package aws_private

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/realli07kkk/sentinel-sync-dns/config"
)

func TestRecordName(t *testing.T) {
	p := &Provider{config: &config.DNSProviderConfig{Domain: "demo.com."}}

	if got := p.recordName("codis-demo-1"); got != "codis-demo-1.demo.com." {
		t.Fatalf("unexpected record name: %s", got)
	}
}

func TestRecordMatches(t *testing.T) {
	p := &Provider{
		config: &config.DNSProviderConfig{
			Record: []config.RecordConfig{{Type: "A", TTL: 60}},
		},
	}
	const recordName = "codis-demo-1.demo.com."

	tests := []struct {
		name   string
		modify func(*types.ResourceRecordSet)
		ip     string
		want   bool
	}{
		{
			name: "matching record",
			ip:   "10.0.0.1",
			want: true,
		},
		{
			name: "matching record ignores case",
			modify: func(record *types.ResourceRecordSet) {
				record.Name = aws.String("CODIS-DEMO-1.DEMO.COM")
			},
			ip:   "10.0.0.1",
			want: true,
		},
		{
			name: "different value",
			ip:   "10.0.0.2",
			want: false,
		},
		{
			name: "different ttl",
			modify: func(record *types.ResourceRecordSet) {
				record.TTL = aws.Int64(300)
			},
			ip:   "10.0.0.1",
			want: false,
		},
		{
			name: "different type",
			modify: func(record *types.ResourceRecordSet) {
				record.Type = types.RRTypeAaaa
			},
			ip:   "10.0.0.1",
			want: false,
		},
		{
			name: "different name",
			modify: func(record *types.ResourceRecordSet) {
				record.Name = aws.String("other.demo.com.")
			},
			ip:   "10.0.0.1",
			want: false,
		},
		{
			name: "multiple values",
			modify: func(record *types.ResourceRecordSet) {
				record.ResourceRecords = append(record.ResourceRecords,
					types.ResourceRecord{Value: aws.String("10.0.0.2")})
			},
			ip:   "10.0.0.1",
			want: false,
		},
		{
			name: "missing name",
			modify: func(record *types.ResourceRecordSet) {
				record.Name = nil
			},
			ip:   "10.0.0.1",
			want: false,
		},
		{
			name: "missing ttl",
			modify: func(record *types.ResourceRecordSet) {
				record.TTL = nil
			},
			ip:   "10.0.0.1",
			want: false,
		},
		{
			name: "missing value",
			modify: func(record *types.ResourceRecordSet) {
				record.ResourceRecords = []types.ResourceRecord{{Value: nil}}
			},
			ip:   "10.0.0.1",
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := &types.ResourceRecordSet{
				Name:            aws.String(recordName),
				Type:            types.RRTypeA,
				TTL:             aws.Int64(60),
				ResourceRecords: []types.ResourceRecord{{Value: aws.String("10.0.0.1")}},
			}
			if test.modify != nil {
				test.modify(record)
			}

			if got := p.recordMatches(record, recordName, test.ip); got != test.want {
				t.Fatalf("recordMatches = %v, want %v", got, test.want)
			}
		})
	}
}

func TestLoadAWSConfigStaticCredentials(t *testing.T) {
	isolateAWSEnvironment(t)

	cfg := &config.DNSProviderConfig{
		RequestTimeout: config.Duration(5 * time.Second),
		AWS: &config.AWSConfig{
			AccessKey:    "ak",
			SecretKey:    "sk",
			SessionToken: "token",
			Region:       "ap-southeast-1",
		},
	}

	awsCfg, err := loadAWSConfig(cfg)
	if err != nil {
		t.Fatalf("loadAWSConfig returned error: %v", err)
	}
	if awsCfg.Region != "ap-southeast-1" {
		t.Fatalf("unexpected region: %s", awsCfg.Region)
	}

	creds, err := awsCfg.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("retrieve credentials: %v", err)
	}
	if creds.AccessKeyID != "ak" || creds.SecretAccessKey != "sk" || creds.SessionToken != "token" {
		t.Fatalf("unexpected credentials: %+v", creds)
	}
}

func TestLoadAWSConfigFallsBackToDefaultChain(t *testing.T) {
	isolateAWSEnvironment(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "env-ak")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "env-sk")

	cfg := &config.DNSProviderConfig{
		RequestTimeout: config.Duration(5 * time.Second),
	}

	awsCfg, err := loadAWSConfig(cfg)
	if err != nil {
		t.Fatalf("loadAWSConfig returned error: %v", err)
	}
	if awsCfg.Region != defaultAWSRegion {
		t.Fatalf("unexpected default region: %s", awsCfg.Region)
	}

	creds, err := awsCfg.Credentials.Retrieve(context.Background())
	if err != nil {
		t.Fatalf("retrieve credentials: %v", err)
	}
	if creds.AccessKeyID != "env-ak" || creds.SecretAccessKey != "env-sk" {
		t.Fatalf("unexpected credentials: %+v", creds)
	}
}

func isolateAWSEnvironment(t *testing.T) {
	t.Helper()

	configDir := t.TempDir()
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(configDir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(configDir, "credentials"))
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

func TestFactoryRejectsInvalidConfig(t *testing.T) {
	validConfig := func() *config.DNSProviderConfig {
		return &config.DNSProviderConfig{
			Name:   "aws-1",
			Type:   "aws-private",
			Domain: "demo.com",
			ZoneID: "Z0123456789",
			Record: []config.RecordConfig{{Type: "A", TTL: 60}},
			AWS:    &config.AWSConfig{AccessKey: "ak", SecretKey: "sk"},
		}
	}

	tests := []struct {
		name   string
		modify func(*config.DNSProviderConfig)
	}{
		{
			name: "wrong provider type",
			modify: func(cfg *config.DNSProviderConfig) {
				cfg.Type = "aws"
			},
		},
		{
			name: "missing domain",
			modify: func(cfg *config.DNSProviderConfig) {
				cfg.Domain = ""
			},
		},
		{
			name: "missing zone id",
			modify: func(cfg *config.DNSProviderConfig) {
				cfg.ZoneID = ""
			},
		},
		{
			name: "missing record",
			modify: func(cfg *config.DNSProviderConfig) {
				cfg.Record = nil
			},
		},
		{
			name: "invalid ttl",
			modify: func(cfg *config.DNSProviderConfig) {
				cfg.Record[0].TTL = 0
			},
		},
		{
			name: "partial static credentials",
			modify: func(cfg *config.DNSProviderConfig) {
				cfg.AWS.SecretKey = ""
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			test.modify(cfg)

			if _, err := (&Factory{}).Create(cfg); err == nil {
				t.Fatalf("expected Create to fail")
			}
		})
	}
}
