package idx

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHeightScore_Block(t *testing.T) {
	// Block transaction: score = height * 1e9 + idx
	score := HeightScore(800000, 5)
	expected := float64(800000)*1000000000 + 5
	assert.Equal(t, expected, score)
}

func TestHeightScore_BlockZeroIdx(t *testing.T) {
	score := HeightScore(800000, 0)
	expected := float64(800000) * 1000000000
	assert.Equal(t, expected, score)
}

func TestHeightScore_Mempool(t *testing.T) {
	// Mempool transaction (height=0): score = current timestamp in nanoseconds
	before := time.Now().UnixNano()
	score := HeightScore(0, 0)
	after := time.Now().UnixNano()

	assert.GreaterOrEqual(t, score, float64(before))
	assert.LessOrEqual(t, score, float64(after))
}

func TestHeightScore_LargeHeight(t *testing.T) {
	// Test with large height values
	score := HeightScore(1000000, 999999)
	expected := float64(1000000)*1000000000 + 999999
	assert.Equal(t, expected, score)
}

func TestHeightScore_Ordering(t *testing.T) {
	// Earlier blocks should have lower scores
	score1 := HeightScore(800000, 0)
	score2 := HeightScore(800001, 0)
	score3 := HeightScore(800000, 1)

	assert.Less(t, score1, score2) // Different heights
	assert.Less(t, score1, score3) // Same height, different idx
	assert.Less(t, score3, score2) // idx difference vs height difference
}

func TestTxo_AddOwner(t *testing.T) {
	txo := &Txo{
		Owners: []string{},
	}

	txo.AddOwner("owner1")
	assert.Equal(t, []string{"owner1"}, txo.Owners)

	// Adding same owner again should not duplicate
	txo.AddOwner("owner1")
	assert.Equal(t, []string{"owner1"}, txo.Owners)

	// Adding different owner
	txo.AddOwner("owner2")
	assert.Equal(t, []string{"owner1", "owner2"}, txo.Owners)
}

func TestTxo_AddOwner_Empty(t *testing.T) {
	txo := &Txo{
		Owners: nil,
	}

	txo.AddOwner("owner1")
	assert.Equal(t, []string{"owner1"}, txo.Owners)
}
