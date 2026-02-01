package arena

import "testing"

func TestSmartReset(t *testing.T) {
	arena := NewArena(1, 2)

	for i := 0; i < 100; i++ {
		_ = New[byte](arena)
	}

	if len(arena.chunks) != 100 {
		t.Fatalf("expected 100 chunks, got %d", len(arena.chunks))
	}

	arena.Reset()

	if len(arena.chunks) > 5 {
		t.Errorf("smart reset failed: still have %d chunks, expected <= 5", len(arena.chunks))
	}

	_ = New[byte](arena)
}
