package provider

import (
	"context"
	"fmt"

	"github.com/realli07kkk/sentinel-sync-dns/config"
)

type Provider interface {
	EnsureDNS(ctx context.Context, masterName, newIP string) (bool, error)
	GetName() string
}

type ProviderFactory interface {
	Create(cfg *config.DNSProviderConfig) (Provider, error)
}

var providers = make(map[string]ProviderFactory)

func RegisterProvider(name string, factory ProviderFactory) {
	providers[name] = factory
}

func CreateProvider(cfg *config.DNSProviderConfig) (Provider, error) {
	factory, ok := providers[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unsupported provider type: %s", cfg.Type)
	}
	return factory.Create(cfg)
}
