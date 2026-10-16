package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/realli07kkk/sentinel-sync-dns/cluster"
	"github.com/realli07kkk/sentinel-sync-dns/config"
	"github.com/realli07kkk/sentinel-sync-dns/provider"
	_ "github.com/realli07kkk/sentinel-sync-dns/provider/huaweicloud-private"
	_ "github.com/realli07kkk/sentinel-sync-dns/provider/tencentcloud-private"
	"github.com/realli07kkk/sentinel-sync-dns/sentinel"
)

const (
	sentinelRetryInterval = 3 * time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置文件失败: %v", err)
	}

	log.Printf("配置加载成功: Sentinel=%s, Master=%v",
		cfg.Sentinel.Host, cfg.Sentinel.MasterName)

	var providers []provider.Provider
	for index := range cfg.DNSProviders {
		providerCfg := &cfg.DNSProviders[index]
		p, err := provider.CreateProvider(providerCfg)
		if err != nil {
			log.Printf("初始化DNSProvider %s 失败: %v", providerCfg.Name, err)
			continue
		}
		providers = append(providers, p)
		log.Printf("DNSProvider %s 初始化成功: Domain=%s",
			providerCfg.Name, providerCfg.Domain)
	}
	if len(providers) == 0 {
		log.Fatalf("没有可用的DNSProvider")
	}

	var coordinator EventCoordinator
	var coordinatorClient *redis.Client
	if cfg.Cluster.Enabled {
		coordinator, coordinatorClient, err = initCoordinator(ctx, &cfg.Cluster)
		if err != nil {
			log.Fatalf("初始化集群协调器失败: %v", err)
		}
		defer coordinatorClient.Close()
		log.Printf("集群模式已启用: namespace=%s node=%s lock_ttl=%s event_ttl=%s renew_interval=%s",
			cfg.Cluster.Namespace, cfg.Cluster.NodeName, cfg.Cluster.LockTTL.Std(), cfg.Cluster.EventTTL.Std(), cfg.Cluster.RenewInterval.Std())
	} else {
		log.Printf("单机模式运行")
	}

	processor := newEventProcessor(
		ctx,
		providers,
		sentinel.BuildMasterFilter(cfg.Sentinel.MasterName),
		coordinator,
	)

	sentinelAddrs := splitAndTrim(cfg.Sentinel.Host)
	if len(sentinelAddrs) == 0 {
		log.Fatalf("sentinel.host 不能为空")
	}

	if err := runSentinelSubscriber(ctx, sentinelAddrs, cfg.Sentinel.Password, processor); err != nil {
		log.Fatalf("Sentinel订阅退出: %v", err)
	}
}

func initCoordinator(ctx context.Context, cfg *config.ClusterConfig) (*cluster.Coordinator, *redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		client.Close()
		return nil, nil, err
	}

	coordinator := cluster.NewCoordinator(
		cluster.NewRedisStore(client),
		cfg.Namespace,
		cfg.NodeName,
		cfg.LockTTL.Std(),
		cfg.EventTTL.Std(),
		cfg.RenewInterval.Std(),
	)
	return coordinator, client, nil
}

func runSentinelSubscriber(ctx context.Context, addrs []string, password string, processor *eventProcessor) error {
	for {
		client, pubsub, addr, err := connectSentinelPubSub(ctx, addrs, password)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("连接Sentinel失败: %v，%s后重试", err, sentinelRetryInterval)
			if !waitForRetry(ctx, sentinelRetryInterval) {
				return nil
			}
			continue
		}

		log.Printf("开始监听Sentinel事件: %s", addr)
		err = consumePubSub(ctx, pubsub, processor)
		_ = pubsub.Close()
		_ = client.Close()
		if ctx.Err() != nil {
			return nil
		}

		log.Printf("Sentinel订阅已断开: %v，准备重连", err)
		if !waitForRetry(ctx, sentinelRetryInterval) {
			return nil
		}
	}
}

func connectSentinelPubSub(ctx context.Context, addrs []string, password string) (*redis.Client, *redis.PubSub, string, error) {
	var lastErr error

	for _, addr := range addrs {
		client := redis.NewClient(&redis.Options{
			Addr:            addr,
			Password:        password,
			MaxRetries:      3,
			MinRetryBackoff: time.Second,
			MaxRetryBackoff: 3 * time.Second,
		})

		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := client.Ping(pingCtx).Err()
		cancel()
		if err != nil {
			lastErr = fmt.Errorf("addr=%s ping failed: %w", addr, err)
			_ = client.Close()
			continue
		}

		pubsub := client.Subscribe(ctx, "+switch-master")
		if _, err := pubsub.Receive(ctx); err != nil {
			lastErr = fmt.Errorf("addr=%s subscribe failed: %w", addr, err)
			_ = pubsub.Close()
			_ = client.Close()
			continue
		}

		return client, pubsub, addr, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("没有可用的Sentinel地址")
	}
	return nil, nil, "", lastErr
}

func consumePubSub(ctx context.Context, pubsub *redis.PubSub, processor *eventProcessor) error {
	channel := pubsub.Channel()

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-channel:
			if !ok {
				return fmt.Errorf("sentinel pubsub channel closed")
			}
			processor.HandleMessage(msg)
		}
	}
}

func splitAndTrim(raw string) []string {
	parts := strings.Split(raw, ",")
	addrs := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		addrs = append(addrs, trimmed)
	}
	return addrs
}

func waitForRetry(ctx context.Context, duration time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(duration):
		return true
	}
}
