package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/realli07kkk/sentinel-sync-dns/cluster"
	"github.com/realli07kkk/sentinel-sync-dns/provider"
	"github.com/realli07kkk/sentinel-sync-dns/sentinel"
	"github.com/redis/go-redis/v9"
)

const (
	clusterRetryInterval = 500 * time.Millisecond
	releaseTimeout       = 3 * time.Second
)

type EventCoordinator interface {
	TryLock(ctx context.Context, masterName string) (cluster.Lock, bool, error)
	IsProcessed(ctx context.Context, eventKey string) (bool, error)
	MarkProcessed(ctx context.Context, eventKey string) error
}

type eventProcessor struct {
	ctx          context.Context
	providers    []provider.Provider
	masterFilter map[string]struct{}
	coordinator  EventCoordinator
	retryDelay   time.Duration

	mu      sync.Mutex
	workers map[string]*masterWorker
}

type masterWorker struct {
	processor *eventProcessor
	master    string
	wakeCh    chan struct{}

	mu    sync.Mutex
	queue []*sentinel.SwitchMasterEvent
}

func newEventProcessor(
	ctx context.Context,
	providers []provider.Provider,
	masterFilter map[string]struct{},
	coordinator EventCoordinator,
) *eventProcessor {
	return &eventProcessor{
		ctx:          ctx,
		providers:    providers,
		masterFilter: masterFilter,
		coordinator:  coordinator,
		retryDelay:   clusterRetryInterval,
		workers:      make(map[string]*masterWorker),
	}
}

func (p *eventProcessor) HandleMessage(msg *redis.Message) {
	log.Printf("收到Sentinel事件: Channel=%s, Payload=%s", msg.Channel, msg.Payload)

	if msg.Channel != "+switch-master" {
		return
	}

	event, err := sentinel.ParseSwitchMaster(msg.Payload)
	if err != nil {
		log.Printf("无效的switch-master消息格式: %v", err)
		return
	}
	if !sentinel.ShouldHandleMaster(p.masterFilter, event.MasterName) {
		log.Printf("忽略未配置的master事件: master=%s", event.MasterName)
		return
	}

	log.Printf("接收master切换事件: master=%s, newIP=%s", event.MasterName, event.NewIP)
	p.getWorker(event.MasterName).Enqueue(event)
}

func (p *eventProcessor) getWorker(masterName string) *masterWorker {
	p.mu.Lock()
	defer p.mu.Unlock()

	worker, ok := p.workers[masterName]
	if ok {
		return worker
	}

	worker = &masterWorker{
		processor: p,
		master:    masterName,
		wakeCh:    make(chan struct{}, 1),
	}
	p.workers[masterName] = worker
	go worker.run()
	return worker
}

func (w *masterWorker) Enqueue(event *sentinel.SwitchMasterEvent) {
	w.mu.Lock()
	w.queue = append(w.queue, event)
	w.mu.Unlock()

	select {
	case w.wakeCh <- struct{}{}:
	default:
	}
}

func (w *masterWorker) run() {
	for {
		event, ok := w.next()
		if !ok {
			return
		}

		if err := w.processor.processEvent(event); err != nil {
			log.Printf("处理master事件失败: master=%s event=%s err=%v", event.MasterName, event.Key(), err)
		}
	}
}

func (w *masterWorker) next() (*sentinel.SwitchMasterEvent, bool) {
	for {
		w.mu.Lock()
		if len(w.queue) > 0 {
			event := w.queue[0]
			w.queue = w.queue[1:]
			w.mu.Unlock()
			return event, true
		}
		w.mu.Unlock()

		select {
		case <-w.processor.ctx.Done():
			return nil, false
		case <-w.wakeCh:
		}
	}
}

func (p *eventProcessor) processEvent(event *sentinel.SwitchMasterEvent) error {
	log.Printf("开始处理master切换事件: master=%s, newIP=%s", event.MasterName, event.NewIP)

	if p.coordinator == nil {
		return p.applyProviders(p.ctx, event)
	}

	return p.processClusterEvent(event)
}

func (p *eventProcessor) processClusterEvent(event *sentinel.SwitchMasterEvent) error {
	for {
		if err := p.ctx.Err(); err != nil {
			return err
		}

		processed, err := p.coordinator.IsProcessed(p.ctx, event.Key())
		if err != nil {
			log.Printf("检查事件去重状态失败: event=%s err=%v", event.Key(), err)
			if !waitForRetry(p.ctx, p.retryDelay) {
				return p.ctx.Err()
			}
			continue
		}
		if processed {
			log.Printf("事件已处理，跳过: event=%s", event.Key())
			return nil
		}

		lock, locked, err := p.coordinator.TryLock(p.ctx, event.MasterName)
		if err != nil {
			log.Printf("获取集群锁失败: master=%s err=%v", event.MasterName, err)
			if !waitForRetry(p.ctx, p.retryDelay) {
				return p.ctx.Err()
			}
			continue
		}
		if !locked {
			if !waitForRetry(p.ctx, p.retryDelay) {
				return p.ctx.Err()
			}
			continue
		}

		return p.executeWithLock(event, lock)
	}
}

func (p *eventProcessor) executeWithLock(event *sentinel.SwitchMasterEvent, lock cluster.Lock) error {
	lockCtx, cancelRenew := context.WithCancel(p.ctx)
	lock.StartAutoRenew(lockCtx)
	defer cancelRenew()
	defer func() {
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), releaseTimeout)
		defer releaseCancel()
		if err := lock.Release(releaseCtx); err != nil {
			log.Printf("释放集群锁失败: master=%s err=%v", event.MasterName, err)
		}
	}()

	processed, err := p.coordinator.IsProcessed(p.ctx, event.Key())
	if err != nil {
		return err
	}
	if processed {
		log.Printf("事件已由其他节点处理: event=%s", event.Key())
		return nil
	}

	if err := p.applyProviders(p.ctx, event); err != nil {
		return err
	}
	if err := p.coordinator.MarkProcessed(p.ctx, event.Key()); err != nil {
		return err
	}

	log.Printf("事件处理完成: event=%s", event.Key())
	return nil
}

func (p *eventProcessor) applyProviders(ctx context.Context, event *sentinel.SwitchMasterEvent) error {
	var failureCount int
	for _, dnsProvider := range p.providers {
		if err := ctx.Err(); err != nil {
			return err
		}

		log.Printf("使用 Provider[%s] 更新DNS记录", dnsProvider.GetName())
		changed, err := dnsProvider.EnsureDNS(ctx, event.MasterName, event.NewIP)
		if err != nil {
			failureCount++
			log.Printf("Provider[%s] 更新DNS记录失败: %v", dnsProvider.GetName(), err)
			continue
		}
		if changed {
			log.Printf("Provider[%s] DNS记录已更新", dnsProvider.GetName())
			continue
		}
		log.Printf("Provider[%s] DNS记录已是目标状态", dnsProvider.GetName())
	}

	if failureCount > 0 {
		return fmt.Errorf("%d 个DNSProvider更新失败", failureCount)
	}
	return nil
}
