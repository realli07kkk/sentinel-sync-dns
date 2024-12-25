package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/realli07kkk/sentinel-sync-dns/config"
	"github.com/realli07kkk/sentinel-sync-dns/provider"
	_ "github.com/realli07kkk/sentinel-sync-dns/provider/huaweicloud-private"
	_ "github.com/realli07kkk/sentinel-sync-dns/provider/tencentcloud-private"
)

func main() {

	ctx := context.Background()

	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置文件失败: %v", err)
	}

	log.Printf("配置加载成功: Sentinel=%s, Master=%v",
		cfg.Sentinel.Host, cfg.Sentinel.MasterName)

	// 创建所有DNSProvider
	var providers []provider.Provider
	for _, providerCfg := range cfg.DNSProviders {
		p, err := provider.CreateProvider(&providerCfg)
		if err != nil {
			log.Printf("初始化DNSProvider %s 失败: %v", providerCfg.Name, err)
			continue
		}
		providers = append(providers, p)
		log.Printf("DNSProvider %s 初始化成功: Domain=%s",
			providerCfg.Name, providerCfg.Domain)
	}

	// 连接Sentinel
	sentinelAddrs := strings.Split(cfg.Sentinel.Host, ",")
	rdb := redis.NewClient(&redis.Options{
		Addr:            sentinelAddrs[0],
		Password:        cfg.Sentinel.Password,
		MaxRetries:      3,
		MinRetryBackoff: time.Second,
		MaxRetryBackoff: time.Second * 3,
	})

	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := rdb.Ping(connCtx).Err(); err != nil {
		log.Fatalf("无法连接到Sentinel: %v", err)
	}
	log.Printf("成功连接到Sentinel: %s", sentinelAddrs[0])

	// 订阅 sentinel 事件
	pubsub := rdb.Subscribe(connCtx,
		"__sentinel__:hello",
		"+sentinel",
		"+switch-master",
		"+slave",
		"+reboot",
	)
	defer pubsub.Close()

	log.Printf("开始监听Sentinel事件...")
	ch := pubsub.Channel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("接收到信号: %v, 准备退出...", sig)
		pubsub.Close()
		rdb.Close()
		os.Exit(0)
	}()

	go func() {
		for {
			select {
			case <-connCtx.Done():
				return
			case <-time.After(time.Second * 5):
				if err := rdb.Ping(connCtx).Err(); err != nil {
					log.Printf("Sentinel连接错误: %v, 尝试重新连接...", err)
					rdb = redis.NewClient(&redis.Options{
						Addr:            sentinelAddrs[0],
						Password:        cfg.Sentinel.Password,
						MaxRetries:      3,
						MinRetryBackoff: time.Second,
						MaxRetryBackoff: time.Second * 3,
					})
				}
			}
		}
	}()

	for msg := range ch {
		log.Printf("收到Sentinel事件: Channel=%s, Payload=%s", msg.Channel, msg.Payload)

		// 处理不同类型的事件
		switch msg.Channel {
		case "+switch-master":
			parts := strings.Split(msg.Payload, " ")
			if len(parts) != 5 {
				log.Printf("无效的switch-master消息格式: %s", msg.Payload)
				continue
			}
			masterName := parts[0]
			newIP := parts[3]
			log.Printf("处理master切换事件: master=%s, newIP=%s", masterName, newIP)

			for _, p := range providers {
				log.Printf("使用 Provider[%s] 更新DNS记录", p.GetName())
				err := p.UpdateDNS(masterName, newIP)
				if err != nil {
					log.Printf("更新DNS记录失败: %v", err)
					continue
				}
				log.Printf("DNS记录更新成功")
			}

		case "*:convert-to-master":

			log.Printf("节点转换为master事件: %s", msg.Payload)

		default:
			log.Printf("收到其他Sentinel事件: %s", msg.Channel)
		}
	}
}
