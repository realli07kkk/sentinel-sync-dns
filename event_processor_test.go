package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/realli07kkk/sentinel-sync-dns/cluster"
	"github.com/realli07kkk/sentinel-sync-dns/provider"
	"github.com/realli07kkk/sentinel-sync-dns/sentinel"
)

type fakeProvider struct {
	name         string
	started      chan string
	releaseFirst chan struct{}
	mu           sync.Mutex
	calls        []string
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{
		name:         "fake",
		started:      make(chan string, 8),
		releaseFirst: make(chan struct{}),
	}
}

func (p *fakeProvider) GetName() string {
	return p.name
}

func (p *fakeProvider) EnsureDNS(ctx context.Context, masterName, newIP string) (bool, error) {
	p.mu.Lock()
	callIndex := len(p.calls)
	p.calls = append(p.calls, newIP)
	p.mu.Unlock()

	select {
	case p.started <- newIP:
	default:
	}

	if callIndex == 0 {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-p.releaseFirst:
		}
	}
	return true, nil
}

func (p *fakeProvider) Calls() []string {
	p.mu.Lock()
	defer p.mu.Unlock()

	copied := make([]string, len(p.calls))
	copy(copied, p.calls)
	return copied
}

type fakeLock struct{}

func (fakeLock) StartAutoRenew(context.Context) {}
func (fakeLock) Release(context.Context) error  { return nil }

type fakeCoordinator struct {
	mu          sync.Mutex
	lockResults []bool
	lockCalls   int
	processed   map[string]bool
}

func newFakeCoordinator(lockResults ...bool) *fakeCoordinator {
	return &fakeCoordinator{
		lockResults: lockResults,
		processed:   make(map[string]bool),
	}
}

func (c *fakeCoordinator) TryLock(_ context.Context, _ string) (cluster.Lock, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lockCalls++
	result := true
	if len(c.lockResults) >= c.lockCalls {
		result = c.lockResults[c.lockCalls-1]
	}

	if !result {
		return nil, false, nil
	}
	return fakeLock{}, true, nil
}

func (c *fakeCoordinator) IsProcessed(_ context.Context, eventKey string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.processed[eventKey], nil
}

func (c *fakeCoordinator) MarkProcessed(_ context.Context, eventKey string) error {
	c.mu.Lock()
	c.processed[eventKey] = true
	c.mu.Unlock()
	return nil
}

func TestEventProcessorKeepsSameMasterSequential(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dnsProvider := newFakeProvider()
	processor := newEventProcessor(ctx, []provider.Provider{dnsProvider}, nil, nil)
	worker := processor.getWorker("master-a")

	worker.Enqueue(&sentinel.SwitchMasterEvent{MasterName: "master-a", NewIP: "10.0.0.2"})
	if got := waitString(t, dnsProvider.started); got != "10.0.0.2" {
		t.Fatalf("unexpected first provider call: %s", got)
	}

	worker.Enqueue(&sentinel.SwitchMasterEvent{MasterName: "master-a", NewIP: "10.0.0.3"})

	select {
	case got := <-dnsProvider.started:
		t.Fatalf("second event started before first completed: %s", got)
	case <-time.After(200 * time.Millisecond):
	}

	close(dnsProvider.releaseFirst)

	if got := waitString(t, dnsProvider.started); got != "10.0.0.3" {
		t.Fatalf("unexpected second provider call: %s", got)
	}

	deadline := time.Now().Add(2 * time.Second)
	for len(dnsProvider.Calls()) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for provider calls, got %v", dnsProvider.Calls())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestProcessClusterEventRetriesOnLockContention(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dnsProvider := newFakeProvider()
	close(dnsProvider.releaseFirst)

	coordinator := newFakeCoordinator(false, false, true)
	processor := newEventProcessor(ctx, []provider.Provider{dnsProvider}, nil, coordinator)
	event := &sentinel.SwitchMasterEvent{MasterName: "master-a", NewIP: "10.0.0.2"}

	if err := processor.processClusterEvent(event); err != nil {
		t.Fatalf("processClusterEvent returned error: %v", err)
	}

	if len(dnsProvider.Calls()) != 1 {
		t.Fatalf("expected one provider call after lock retry, got %v", dnsProvider.Calls())
	}
	if coordinator.lockCalls < 3 {
		t.Fatalf("expected repeated lock attempts, got %d", coordinator.lockCalls)
	}
}

func TestProcessClusterEventSkipsProcessedEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dnsProvider := newFakeProvider()
	close(dnsProvider.releaseFirst)

	coordinator := newFakeCoordinator(true)
	event := &sentinel.SwitchMasterEvent{MasterName: "master-a", NewIP: "10.0.0.2"}
	coordinator.processed[event.Key()] = true

	processor := newEventProcessor(ctx, []provider.Provider{dnsProvider}, nil, coordinator)
	if err := processor.processClusterEvent(event); err != nil {
		t.Fatalf("processClusterEvent returned error: %v", err)
	}
	if got := len(dnsProvider.Calls()); got != 0 {
		t.Fatalf("expected no provider calls for processed event, got %d", got)
	}
}

func TestProcessClusterEventDoesNotSkipDifferentTransitionToSameNewIP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dnsProvider := newFakeProvider()
	close(dnsProvider.releaseFirst)

	coordinator := newFakeCoordinator(true)
	previousEvent := &sentinel.SwitchMasterEvent{
		MasterName: "master-a",
		OldIP:      "10.0.0.1",
		OldPort:    "6379",
		NewIP:      "10.0.0.2",
		NewPort:    "6379",
	}
	currentEvent := &sentinel.SwitchMasterEvent{
		MasterName: "master-a",
		OldIP:      "10.0.0.3",
		OldPort:    "6379",
		NewIP:      "10.0.0.2",
		NewPort:    "6379",
	}
	coordinator.processed[previousEvent.Key()] = true

	processor := newEventProcessor(ctx, []provider.Provider{dnsProvider}, nil, coordinator)
	if err := processor.processClusterEvent(currentEvent); err != nil {
		t.Fatalf("processClusterEvent returned error: %v", err)
	}
	if got := len(dnsProvider.Calls()); got != 1 {
		t.Fatalf("expected one provider call for a distinct transition, got %d", got)
	}
}

func waitString(t *testing.T, ch <-chan string) string {
	t.Helper()

	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal(errors.New("timed out waiting for channel value"))
		return ""
	}
}
