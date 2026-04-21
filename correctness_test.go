package arena

import (
	"strings"
	"testing"
)

func mustPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("%s: expected panic", name)
		}
	}()
	fn()
}

func TestNewArenaPanicsOnInvalidSize(t *testing.T) {
	mustPanic(t, "zero size", func() {
		_ = NewArena(0, 0)
	})
	mustPanic(t, "negative size", func() {
		_ = NewArena(-1, 0)
	})
}

func TestNewArenaUsesDefaultMaxRetained(t *testing.T) {
	a := NewArena(128, 0)
	if a.maxRetain != 1280 {
		t.Fatalf("unexpected maxRetain: got %d, want %d", a.maxRetain, 1280)
	}
}

func TestAllocStringCopiesInput(t *testing.T) {
	a := NewArena(256, 0)
	src := []byte("hello")
	out := a.AllocBytesToString(src)
	src[0] = 'j'

	if out != "hello" {
		t.Fatalf("arena string should not change with source mutation: got %q", out)
	}
}

func TestAllocBytesToStringCopiesInput(t *testing.T) {
	a := NewArena(256, 0)
	src := []byte("payload")
	out := a.AllocBytesToString(src)
	src[1] = 'X'

	if out != "payload" {
		t.Fatalf("expected copied string, got %q", out)
	}
}

func TestResetResetsUsedBytesAndCursor(t *testing.T) {
	a := NewArena(64, 128)
	_ = a.AllocString(strings.Repeat("x", 80)) // force growth
	if a.UsedBytes() == 0 {
		t.Fatal("expected used bytes before reset")
	}

	a.Reset()
	if a.UsedBytes() != 0 {
		t.Fatalf("expected used bytes to be zero after reset, got %d", a.UsedBytes())
	}
	if a.chunkIndex != 0 || a.offset != 0 {
		t.Fatalf("expected cursor reset, got chunkIndex=%d offset=%d", a.chunkIndex, a.offset)
	}
}

func TestResetTrimsRetainedMemory(t *testing.T) {
	a := NewArena(64, 96)
	_ = a.AllocString(strings.Repeat("a", 60))
	_ = a.AllocString(strings.Repeat("b", 60))
	_ = a.AllocString(strings.Repeat("c", 60))
	if len(a.chunks) < 2 {
		t.Fatalf("expected multiple chunks before trim, got %d", len(a.chunks))
	}

	a.Reset()
	if len(a.chunks) > 2 {
		t.Fatalf("expected trimmed chunks by maxRetain, got %d", len(a.chunks))
	}
}

func TestMakeSliceAndAppendPreserveData(t *testing.T) {
	a := NewArena(256, 0)
	items := MakeSlice[int](a, 0, 2)
	items = Append(a, items, 1, 2, 3, 4)
	want := []int{1, 2, 3, 4}
	if len(items) != len(want) {
		t.Fatalf("unexpected len: got %d, want %d", len(items), len(want))
	}
	for i := range want {
		if items[i] != want[i] {
			t.Fatalf("unexpected value at %d: got %d, want %d", i, items[i], want[i])
		}
	}
}

func TestMakeSliceZeroCapacityReturnsNil(t *testing.T) {
	a := NewArena(128, 0)
	s := MakeSlice[int](a, 0, 0)
	if s != nil {
		t.Fatalf("expected nil slice for zero capacity, got len=%d cap=%d", len(s), cap(s))
	}
}

func TestMakeSlicePanicsOnInvalidArgs(t *testing.T) {
	a := NewArena(128, 0)
	mustPanic(t, "negative len", func() {
		_ = MakeSlice[int](a, -1, 0)
	})
	mustPanic(t, "cap smaller than len", func() {
		_ = MakeSlice[int](a, 2, 1)
	})
}

func TestNewZeroSizedType(t *testing.T) {
	type zero struct{}

	a := NewArena(64, 0)
	v := New[zero](a)
	if v == nil {
		t.Fatal("expected non-nil pointer for zero-sized type")
	}
	if a.UsedBytes() != 0 {
		t.Fatalf("zero-sized allocation should not consume arena bytes, got %d", a.UsedBytes())
	}
}

func TestArenaPoolMetricsSnapshot(t *testing.T) {
	p := NewArenaPool(256, 0)
	mem := p.Get()
	_ = New[User](mem)
	p.Put(mem)

	s := p.MetricsSnapshot()
	if s.GetCount == 0 {
		t.Fatal("expected GetCount > 0")
	}
	if s.ActiveArenas != 0 {
		t.Fatalf("expected no active arenas, got %d", s.ActiveArenas)
	}
	if s.TotalCapacityBytes == 0 {
		t.Fatal("expected capacity metrics to be tracked")
	}
}
