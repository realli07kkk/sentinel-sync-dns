package cluster

import (
	"context"
	"fmt"
	"log"
	"time"
)

type Coordinator struct {
	store         Store
	nodeName      string
	lockTTL       time.Duration
	eventTTL      time.Duration
	renewInterval time.Duration
	keyPrefix     string
}

type Lock interface {
	StartAutoRenew(ctx context.Context)
	Release(ctx context.Context) error
}

type RedisLock struct {
	store         Store
	key           string
	token         string
	lockTTL       time.Duration
	renewInterval time.Duration
}

func NewCoordinator(store Store, namespace, nodeName string, lockTTL, eventTTL, renewInterval time.Duration) *Coordinator {
	if namespace == "" {
		namespace = defaultKeyPrefix
	}

	return &Coordinator{
		store:         store,
		nodeName:      nodeName,
		lockTTL:       lockTTL,
		eventTTL:      eventTTL,
		renewInterval: renewInterval,
		keyPrefix:     namespace,
	}
}

func (c *Coordinator) TryLock(ctx context.Context, masterName string) (Lock, bool, error) {
	key := c.masterLockKey(masterName)
	token := fmt.Sprintf("%s:%d", c.nodeName, time.Now().UnixNano())

	locked, err := c.store.SetNX(ctx, key, token, c.lockTTL)
	if err != nil {
		return nil, false, err
	}
	if !locked {
		return nil, false, nil
	}

	return &RedisLock{
		store:         c.store,
		key:           key,
		token:         token,
		lockTTL:       c.lockTTL,
		renewInterval: c.renewInterval,
	}, true, nil
}

func (c *Coordinator) IsProcessed(ctx context.Context, eventKey string) (bool, error) {
	return c.store.Exists(ctx, c.processedEventKey(eventKey))
}

func (c *Coordinator) MarkProcessed(ctx context.Context, eventKey string) error {
	return c.store.Set(ctx, c.processedEventKey(eventKey), c.nodeName, c.eventTTL)
}

func (c *Coordinator) masterLockKey(masterName string) string {
	return fmt.Sprintf("lock:%s:master:%s", c.keyPrefix, masterName)
}

func (c *Coordinator) processedEventKey(eventKey string) string {
	return fmt.Sprintf("done:%s:event:%s", c.keyPrefix, eventKey)
}

func (l *RedisLock) StartAutoRenew(ctx context.Context) {
	if l.renewInterval <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(l.renewInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				renewed, err := l.store.CompareAndExpire(ctx, l.key, l.token, l.lockTTL)
				if err != nil {
					log.Printf("续租集群锁失败: key=%s err=%v", l.key, err)
					continue
				}
				if !renewed {
					log.Printf("集群锁已丢失: key=%s", l.key)
					return
				}
			}
		}
	}()
}

func (l *RedisLock) Release(ctx context.Context) error {
	_, err := l.store.CompareAndDelete(ctx, l.key, l.token)
	return err
}
