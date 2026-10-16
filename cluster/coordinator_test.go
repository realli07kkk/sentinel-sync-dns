package cluster

import (
	"context"
	"sync"
	"testing"
	"time"
)

type fakeStore struct {
	mu    sync.Mutex
	items map[string]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		items: make(map[string]string),
	}
}

func (s *fakeStore) SetNX(_ context.Context, key, value string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.items[key]; exists {
		return false, nil
	}
	s.items[key] = value
	return true, nil
}

func (s *fakeStore) Exists(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.items[key]
	return exists, nil
}

func (s *fakeStore) Set(_ context.Context, key, value string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.items[key] = value
	return nil
}

func (s *fakeStore) CompareAndDelete(_ context.Context, key, expectedValue string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if currentValue, ok := s.items[key]; ok && currentValue == expectedValue {
		delete(s.items, key)
		return true, nil
	}
	return false, nil
}

func (s *fakeStore) CompareAndExpire(_ context.Context, key, expectedValue string, _ time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	currentValue, ok := s.items[key]
	return ok && currentValue == expectedValue, nil
}

func TestCoordinatorTryLock(t *testing.T) {
	coordinator := NewCoordinator(newFakeStore(), "ns-a", "node-a", 15*time.Second, 60*time.Second, 5*time.Second)

	lock, ok, err := coordinator.TryLock(context.Background(), "master-a")
	if err != nil {
		t.Fatalf("TryLock returned error: %v", err)
	}
	if !ok || lock == nil {
		t.Fatalf("expected first lock attempt to succeed")
	}

	if _, ok, err := coordinator.TryLock(context.Background(), "master-a"); err != nil {
		t.Fatalf("second TryLock returned error: %v", err)
	} else if ok {
		t.Fatalf("expected second lock attempt to fail while first lock is held")
	}

	if err := lock.Release(context.Background()); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}

	if _, ok, err := coordinator.TryLock(context.Background(), "master-a"); err != nil {
		t.Fatalf("third TryLock returned error: %v", err)
	} else if !ok {
		t.Fatalf("expected lock attempt to succeed after release")
	}
}

func TestCoordinatorProcessedMarkers(t *testing.T) {
	coordinator := NewCoordinator(newFakeStore(), "ns-a", "node-a", 15*time.Second, 60*time.Second, 5*time.Second)
	ctx := context.Background()

	processed, err := coordinator.IsProcessed(ctx, "switch-master:master-a:10.0.0.2")
	if err != nil {
		t.Fatalf("IsProcessed returned error: %v", err)
	}
	if processed {
		t.Fatalf("expected event to be unprocessed initially")
	}

	if err := coordinator.MarkProcessed(ctx, "switch-master:master-a:10.0.0.2"); err != nil {
		t.Fatalf("MarkProcessed returned error: %v", err)
	}

	processed, err = coordinator.IsProcessed(ctx, "switch-master:master-a:10.0.0.2")
	if err != nil {
		t.Fatalf("IsProcessed returned error: %v", err)
	}
	if !processed {
		t.Fatalf("expected event to be marked as processed")
	}
}

func TestCoordinatorNamespaceIsolation(t *testing.T) {
	store := newFakeStore()
	coordinatorA := NewCoordinator(store, "ns-a", "node-a", 15*time.Second, 60*time.Second, 5*time.Second)
	coordinatorB := NewCoordinator(store, "ns-b", "node-b", 15*time.Second, 60*time.Second, 5*time.Second)

	lockA, ok, err := coordinatorA.TryLock(context.Background(), "master-a")
	if err != nil {
		t.Fatalf("TryLock returned error: %v", err)
	}
	if !ok || lockA == nil {
		t.Fatalf("expected namespace A to acquire lock")
	}
	defer lockA.Release(context.Background())

	lockB, ok, err := coordinatorB.TryLock(context.Background(), "master-a")
	if err != nil {
		t.Fatalf("TryLock returned error: %v", err)
	}
	if !ok || lockB == nil {
		t.Fatalf("expected namespace B to acquire independent lock")
	}
}
