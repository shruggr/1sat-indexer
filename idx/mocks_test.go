package idx

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/b-open-io/overlay/pubsub"
	"github.com/b-open-io/overlay/queue"
)

// MockQueueStorage is an in-memory implementation of queue.QueueStorage for testing
type MockQueueStorage struct {
	mu         sync.RWMutex
	sets       map[string]map[string]struct{}
	hashes     map[string]map[string]string
	sortedSets map[string][]queue.ScoredMember
}

func NewMockQueueStorage() *MockQueueStorage {
	return &MockQueueStorage{
		sets:       make(map[string]map[string]struct{}),
		hashes:     make(map[string]map[string]string),
		sortedSets: make(map[string][]queue.ScoredMember),
	}
}

// Set Operations
func (m *MockQueueStorage) SAdd(ctx context.Context, key string, members ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sets[key] == nil {
		m.sets[key] = make(map[string]struct{})
	}
	for _, member := range members {
		m.sets[key][member] = struct{}{}
	}
	return nil
}

func (m *MockQueueStorage) SMembers(ctx context.Context, key string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.sets[key] == nil {
		return nil, nil
	}
	result := make([]string, 0, len(m.sets[key]))
	for member := range m.sets[key] {
		result = append(result, member)
	}
	return result, nil
}

func (m *MockQueueStorage) SRem(ctx context.Context, key string, members ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sets[key] == nil {
		return nil
	}
	for _, member := range members {
		delete(m.sets[key], member)
	}
	return nil
}

func (m *MockQueueStorage) SIsMember(ctx context.Context, key, member string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.sets[key] == nil {
		return false, nil
	}
	_, ok := m.sets[key][member]
	return ok, nil
}

// Hash Operations
func (m *MockQueueStorage) HSet(ctx context.Context, key, field, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.hashes[key] == nil {
		m.hashes[key] = make(map[string]string)
	}
	m.hashes[key][field] = value
	return nil
}

func (m *MockQueueStorage) HGet(ctx context.Context, key, field string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.hashes[key] == nil {
		return "", nil
	}
	return m.hashes[key][field], nil
}

func (m *MockQueueStorage) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.hashes[key] == nil {
		return nil, nil
	}
	result := make(map[string]string, len(m.hashes[key]))
	for k, v := range m.hashes[key] {
		result[k] = v
	}
	return result, nil
}

func (m *MockQueueStorage) HDel(ctx context.Context, key string, fields ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.hashes[key] == nil {
		return nil
	}
	for _, field := range fields {
		delete(m.hashes[key], field)
	}
	return nil
}

func (m *MockQueueStorage) HMSet(ctx context.Context, key string, fields map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.hashes[key] == nil {
		m.hashes[key] = make(map[string]string)
	}
	for k, v := range fields {
		m.hashes[key][k] = v
	}
	return nil
}

func (m *MockQueueStorage) HMGet(ctx context.Context, key string, fields ...string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]string, len(fields))
	if m.hashes[key] == nil {
		return result, nil
	}
	for i, field := range fields {
		result[i] = m.hashes[key][field]
	}
	return result, nil
}

// Sorted Set Operations
func (m *MockQueueStorage) ZAdd(ctx context.Context, key string, members ...queue.ScoredMember) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sortedSets[key] == nil {
		m.sortedSets[key] = []queue.ScoredMember{}
	}

	for _, newMember := range members {
		found := false
		for i, existing := range m.sortedSets[key] {
			if existing.Member == newMember.Member {
				m.sortedSets[key][i].Score = newMember.Score
				found = true
				break
			}
		}
		if !found {
			m.sortedSets[key] = append(m.sortedSets[key], newMember)
		}
	}

	// Sort by score ascending
	sort.Slice(m.sortedSets[key], func(i, j int) bool {
		return m.sortedSets[key][i].Score < m.sortedSets[key][j].Score
	})

	return nil
}

func (m *MockQueueStorage) ZRem(ctx context.Context, key string, members ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sortedSets[key] == nil {
		return nil
	}

	memberSet := make(map[string]struct{}, len(members))
	for _, member := range members {
		memberSet[member] = struct{}{}
	}

	filtered := make([]queue.ScoredMember, 0, len(m.sortedSets[key]))
	for _, sm := range m.sortedSets[key] {
		if _, ok := memberSet[sm.Member]; !ok {
			filtered = append(filtered, sm)
		}
	}
	m.sortedSets[key] = filtered
	return nil
}

func (m *MockQueueStorage) ZRange(ctx context.Context, key string, scoreRange queue.ScoreRange) ([]queue.ScoredMember, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.sortedSets[key] == nil {
		return nil, nil
	}

	var result []queue.ScoredMember
	for _, sm := range m.sortedSets[key] {
		if !m.inRange(sm.Score, scoreRange) {
			continue
		}
		result = append(result, sm)
	}

	if scoreRange.Offset > 0 && int(scoreRange.Offset) < len(result) {
		result = result[scoreRange.Offset:]
	} else if scoreRange.Offset >= int64(len(result)) {
		return nil, nil
	}

	if scoreRange.Count > 0 && int(scoreRange.Count) < len(result) {
		result = result[:scoreRange.Count]
	}

	return result, nil
}

func (m *MockQueueStorage) ZRevRange(ctx context.Context, key string, scoreRange queue.ScoreRange) ([]queue.ScoredMember, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.sortedSets[key] == nil {
		return nil, nil
	}

	// Filter and collect in reverse order
	var result []queue.ScoredMember
	for i := len(m.sortedSets[key]) - 1; i >= 0; i-- {
		sm := m.sortedSets[key][i]
		if !m.inRange(sm.Score, scoreRange) {
			continue
		}
		result = append(result, sm)
	}

	if scoreRange.Offset > 0 && int(scoreRange.Offset) < len(result) {
		result = result[scoreRange.Offset:]
	} else if scoreRange.Offset >= int64(len(result)) {
		return nil, nil
	}

	if scoreRange.Count > 0 && int(scoreRange.Count) < len(result) {
		result = result[:scoreRange.Count]
	}

	return result, nil
}

func (m *MockQueueStorage) inRange(score float64, r queue.ScoreRange) bool {
	if r.Min != nil {
		if r.MinExclusive {
			if score <= *r.Min {
				return false
			}
		} else {
			if score < *r.Min {
				return false
			}
		}
	}
	if r.Max != nil {
		if r.MaxExclusive {
			if score >= *r.Max {
				return false
			}
		} else {
			if score > *r.Max {
				return false
			}
		}
	}
	return true
}

func (m *MockQueueStorage) ZScore(ctx context.Context, key, member string) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.sortedSets[key] == nil {
		return 0, errors.New("redis: nil")
	}

	for _, sm := range m.sortedSets[key] {
		if sm.Member == member {
			return sm.Score, nil
		}
	}
	return 0, errors.New("redis: nil")
}

func (m *MockQueueStorage) ZCard(ctx context.Context, key string) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.sortedSets[key] == nil {
		return 0, nil
	}
	return int64(len(m.sortedSets[key])), nil
}

func (m *MockQueueStorage) ZIncrBy(ctx context.Context, key, member string, increment float64) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sortedSets[key] == nil {
		m.sortedSets[key] = []queue.ScoredMember{}
	}

	for i, sm := range m.sortedSets[key] {
		if sm.Member == member {
			m.sortedSets[key][i].Score += increment
			return m.sortedSets[key][i].Score, nil
		}
	}

	m.sortedSets[key] = append(m.sortedSets[key], queue.ScoredMember{
		Member: member,
		Score:  increment,
	})
	return increment, nil
}

func (m *MockQueueStorage) ZSum(ctx context.Context, key string) (float64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.sortedSets[key] == nil {
		return 0, nil
	}

	var sum float64
	for _, sm := range m.sortedSets[key] {
		sum += sm.Score
	}
	return sum, nil
}

func (m *MockQueueStorage) Close() error {
	return nil
}

// MockPubSub is an in-memory implementation of pubsub.PubSub for testing
type MockPubSub struct {
	mu        sync.RWMutex
	published []PublishedEvent
}

type PublishedEvent struct {
	Topic string
	Data  string
	Score float64
}

func NewMockPubSub() *MockPubSub {
	return &MockPubSub{
		published: []PublishedEvent{},
	}
}

func (m *MockPubSub) Publish(ctx context.Context, topic string, data string, score ...float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	event := PublishedEvent{Topic: topic, Data: data}
	if len(score) > 0 {
		event.Score = score[0]
	}
	m.published = append(m.published, event)
	return nil
}

func (m *MockPubSub) Subscribe(ctx context.Context, topics []string) (<-chan pubsub.Event, error) {
	ch := make(chan pubsub.Event)
	return ch, nil
}

func (m *MockPubSub) Unsubscribe(topics []string) error {
	return nil
}

func (m *MockPubSub) Stop() error {
	return nil
}

func (m *MockPubSub) Close() error {
	return nil
}

func (m *MockPubSub) GetPublished() []PublishedEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]PublishedEvent{}, m.published...)
}

func (m *MockPubSub) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = []PublishedEvent{}
}
