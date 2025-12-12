package idx

import (
	"context"
	"fmt"
	"testing"

	"github.com/b-open-io/overlay/queue"
)

func BenchmarkSearch_100(b *testing.B) {
	benchmarkSearch(b, 100)
}

func BenchmarkSearch_1K(b *testing.B) {
	benchmarkSearch(b, 1000)
}

func BenchmarkSearch_10K(b *testing.B) {
	benchmarkSearch(b, 10000)
}

func benchmarkSearch(b *testing.B, count int) {
	mockQueue := NewMockQueueStorage()
	mockPubSub := NewMockPubSub()
	store := NewQueueStore(mockQueue, mockPubSub)
	ctx := context.Background()

	// Pre-populate sorted set
	for i := 0; i < count; i++ {
		mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{
			Member: fmt.Sprintf("out%d_0", i),
			Score:  float64(i * 1000),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Search(ctx, &SearchCfg{
			Keys:  []string{"test:key"},
			Limit: 100,
		})
	}
}

func BenchmarkSearch_FilterSpent_100(b *testing.B) {
	benchmarkSearchFilterSpent(b, 100, 50) // 50% spent
}

func BenchmarkSearch_FilterSpent_1K(b *testing.B) {
	benchmarkSearchFilterSpent(b, 1000, 500)
}

func benchmarkSearchFilterSpent(b *testing.B, count, spentCount int) {
	mockQueue := NewMockQueueStorage()
	mockPubSub := NewMockPubSub()
	store := NewQueueStore(mockQueue, mockPubSub)
	ctx := context.Background()

	// Pre-populate sorted set
	for i := 0; i < count; i++ {
		member := fmt.Sprintf("out%d_0", i)
		mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{
			Member: member,
			Score:  float64(i * 1000),
		})
		// Mark some as spent
		if i < spentCount {
			mockQueue.HSet(ctx, SpendsKey, member, "spendingtx")
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Search(ctx, &SearchCfg{
			Keys:        []string{"test:key"},
			Limit:       100,
			FilterSpent: true,
		})
	}
}

func BenchmarkSearch_MultiKey_OR(b *testing.B) {
	mockQueue := NewMockQueueStorage()
	mockPubSub := NewMockPubSub()
	store := NewQueueStore(mockQueue, mockPubSub)
	ctx := context.Background()

	// Pre-populate two sorted sets
	for i := 0; i < 500; i++ {
		mockQueue.ZAdd(ctx, "key1", queue.ScoredMember{
			Member: fmt.Sprintf("out%d_0", i),
			Score:  float64(i * 1000),
		})
		mockQueue.ZAdd(ctx, "key2", queue.ScoredMember{
			Member: fmt.Sprintf("out%d_1", i+500),
			Score:  float64((i + 500) * 1000),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Search(ctx, &SearchCfg{
			Keys:           []string{"key1", "key2"},
			ComparisonType: ComparisonOR,
			Limit:          100,
		})
	}
}

func BenchmarkSearch_MultiKey_AND(b *testing.B) {
	mockQueue := NewMockQueueStorage()
	mockPubSub := NewMockPubSub()
	store := NewQueueStore(mockQueue, mockPubSub)
	ctx := context.Background()

	// Pre-populate two sorted sets with overlapping members
	for i := 0; i < 500; i++ {
		member := fmt.Sprintf("out%d_0", i)
		score := float64(i * 1000)
		mockQueue.ZAdd(ctx, "key1", queue.ScoredMember{Member: member, Score: score})
		// Only 50% overlap
		if i%2 == 0 {
			mockQueue.ZAdd(ctx, "key2", queue.ScoredMember{Member: member, Score: score})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Search(ctx, &SearchCfg{
			Keys:           []string{"key1", "key2"},
			ComparisonType: ComparisonAND,
			Limit:          100,
		})
	}
}

func BenchmarkSearchBalance(b *testing.B) {
	mockQueue := NewMockQueueStorage()
	mockPubSub := NewMockPubSub()
	store := NewQueueStore(mockQueue, mockPubSub)
	ctx := context.Background()

	// Pre-populate sorted set and sats
	for i := 0; i < 100; i++ {
		member := fmt.Sprintf("out%d_0", i)
		mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{
			Member: member,
			Score:  float64(i * 1000),
		})
		mockQueue.HSet(ctx, SatsKey, member, fmt.Sprintf("%d", (i+1)*100))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.SearchBalance(ctx, &SearchCfg{
			Keys: []string{"test:key"},
		})
	}
}

func BenchmarkCountMembers(b *testing.B) {
	mockQueue := NewMockQueueStorage()
	mockPubSub := NewMockPubSub()
	store := NewQueueStore(mockQueue, mockPubSub)
	ctx := context.Background()

	// Pre-populate sorted set
	for i := 0; i < 10000; i++ {
		mockQueue.ZAdd(ctx, "test:key", queue.ScoredMember{
			Member: fmt.Sprintf("out%d_0", i),
			Score:  float64(i * 1000),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.CountMembers(ctx, "test:key")
	}
}
