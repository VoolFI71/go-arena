package arena

import "testing"

// TestPublicAPIContracts keeps compile-time checks for exported API signatures.
// If any signature changes, this test fails to compile and signals a breaking change.
func TestPublicAPIContracts(t *testing.T) {
	// Constructors and generic helpers.
	var _ func(int, int) *Arena = NewArena
	var _ func(*Arena) *int = New[int]
	var _ func(*Arena, int, int) []int = MakeSlice[int]
	var _ func(*Arena, []int, ...int) []int = Append[int]
	var _ func(int, int) *ArenaPool = NewArenaPool

	// Arena methods.
	var _ func(*Arena) = (*Arena).Reset
	var _ func(*Arena, string) string = (*Arena).AllocString
	var _ func(*Arena, []byte) string = (*Arena).AllocBytesToString
	var _ func(*Arena) int = (*Arena).UsedBytes

	// Pool methods.
	var _ func(*ArenaPool) *Arena = (*ArenaPool).Get
	var _ func(*ArenaPool, *Arena) = (*ArenaPool).Put
	var _ func(*ArenaPool) PoolMetricsSnapshot = (*ArenaPool).MetricsSnapshot

	// Exported types presence.
	var _ *PoolMetrics
	var _ *PoolMetricsSnapshot
}
