package arena

import (
	"strings"
	"sync"
	"testing"
	"unsafe"
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

// TestArenaPoolConcurrentGetPut verifies that concurrent Get/Put cycles do not
// produce data races and that pool metrics stay consistent.
// Run with: go test -race -run TestArenaPoolConcurrentGetPut
func TestArenaPoolConcurrentGetPut(t *testing.T) {
	const (
		goroutines    = 64
		opsPerRoutine = 200
		chunkSize     = 16 * 1024
	)

	p := NewArenaPool(chunkSize, 0)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerRoutine; j++ {
				mem := p.Get()

				// Typical per-request work: a struct + a string.
				u := New[User](mem)
				u.ID = id*opsPerRoutine + j
				u.Name = mem.AllocString("concurrent-test-payload")

				// A small slice to exercise MakeSlice path.
				sl := MakeSlice[int](mem, 4, 8)
				for k := range sl {
					sl[k] = k
				}

				p.Put(mem)
			}
		}(i)
	}

	wg.Wait()

	snap := p.MetricsSnapshot()
	totalOps := uint64(goroutines * opsPerRoutine)

	if snap.GetCount != totalOps {
		t.Errorf("GetCount: got %d, want %d", snap.GetCount, totalOps)
	}
	if snap.ActiveArenas != 0 {
		t.Errorf("ActiveArenas after all Put: got %d, want 0", snap.ActiveArenas)
	}
	if snap.TotalCapacityBytes == 0 {
		t.Error("TotalCapacityBytes must be > 0 after pool use")
	}
}

// aligned8 has 8-byte alignment (float64 field).
type aligned8 struct {
	v float64
}

// aligned1 has 1-byte alignment.
type aligned1 struct {
	b byte
}

// aligned16 is a manually aligned-16 struct via array of uint64.
// On amd64 unsafe.Alignof returns 8, but the test still checks the general rule.
type aligned16 struct {
	a, b uint64
}

// TestAllocAlignment verifies that New[T] and MakeSlice[T] return pointers that
// satisfy the alignment requirement of the allocated type.
func TestAllocAlignment(t *testing.T) {
	a := NewArena(4096, 0)

	t.Run("New[byte]", func(t *testing.T) {
		for i := 0; i < 16; i++ {
			p := New[byte](a)
			addr := uintptr(unsafe.Pointer(p))
			want := unsafe.Alignof(*p)
			if addr%uintptr(want) != 0 {
				t.Fatalf("iter %d: address 0x%x not aligned to %d", i, addr, want)
			}
		}
	})

	t.Run("New[aligned8]", func(t *testing.T) {
		// Interleave byte allocations to stress padding logic.
		for i := 0; i < 16; i++ {
			_ = New[byte](a)
			p := New[aligned8](a)
			addr := uintptr(unsafe.Pointer(p))
			want := unsafe.Alignof(*p)
			if addr%uintptr(want) != 0 {
				t.Fatalf("iter %d: address 0x%x not aligned to %d", i, addr, want)
			}
		}
	})

	t.Run("New[aligned1]", func(t *testing.T) {
		for i := 0; i < 32; i++ {
			p := New[aligned1](a)
			addr := uintptr(unsafe.Pointer(p))
			want := unsafe.Alignof(*p)
			if addr%uintptr(want) != 0 {
				t.Fatalf("iter %d: address 0x%x not aligned to %d", i, addr, want)
			}
		}
	})

	t.Run("New[uint64]", func(t *testing.T) {
		for i := 0; i < 16; i++ {
			// Introduce misalignment pressure with a byte alloc.
			_ = New[byte](a)
			p := New[uint64](a)
			addr := uintptr(unsafe.Pointer(p))
			want := unsafe.Alignof(*p)
			if addr%uintptr(want) != 0 {
				t.Fatalf("iter %d: address 0x%x not aligned to %d", i, addr, want)
			}
		}
	})

	t.Run("MakeSlice[float64]", func(t *testing.T) {
		for i := 0; i < 8; i++ {
			_ = New[byte](a) // misalign intentionally
			sl := MakeSlice[float64](a, 4, 4)
			if len(sl) == 0 {
				t.Fatal("expected non-empty slice")
			}
			addr := uintptr(unsafe.Pointer(&sl[0]))
			var dummy float64
			want := unsafe.Alignof(dummy)
			if addr%uintptr(want) != 0 {
				t.Fatalf("iter %d: slice base address 0x%x not aligned to %d", i, addr, want)
			}
		}
	})

	t.Run("AlignmentAcrossChunkBoundary", func(t *testing.T) {
		// Small arena to force chunk allocation and verify alignment holds
		// even for the first object in a fresh chunk.
		small := NewArena(64, 0)
		for i := 0; i < 10; i++ {
			_ = New[byte](small)           // nudge offset
			p := New[aligned8](small)
			addr := uintptr(unsafe.Pointer(p))
			want := unsafe.Alignof(*p)
			if addr%uintptr(want) != 0 {
				t.Fatalf("chunk boundary iter %d: address 0x%x not aligned to %d", i, addr, want)
			}
		}
	})
}

// TestAllocBytesZeroReturnsNil checks that AllocBytes(0) returns nil.
func TestAllocBytesZeroReturnsNil(t *testing.T) {
	a := NewArena(64, 0)
	b := a.AllocBytes(0)
	if b != nil {
		t.Fatalf("expected nil for n=0, got len=%d cap=%d", len(b), cap(b))
	}
}

// TestAllocBytesPanicsOnNegative checks that AllocBytes panics for n < 0.
func TestAllocBytesPanicsOnNegative(t *testing.T) {
	mustPanic(t, "negative size", func() {
		a := NewArena(64, 0)
		_ = a.AllocBytes(-1)
	})
}

// TestAllocBytesLengthAndCapacity checks that the returned slice has the
// requested length and capacity (both must equal n).
func TestAllocBytesLengthAndCapacity(t *testing.T) {
	a := NewArena(256, 0)
	for _, n := range []int{1, 7, 16, 100, 255} {
		b := a.AllocBytes(n)
		if len(b) != n {
			t.Errorf("n=%d: len=%d, want %d", n, len(b), n)
		}
		if cap(b) != n {
			t.Errorf("n=%d: cap=%d, want %d", n, cap(b), n)
		}
	}
}

// TestAllocBytesWriteRead verifies that data written into the slice is
// readable back correctly.
func TestAllocBytesWriteRead(t *testing.T) {
	a := NewArena(256, 0)
	b := a.AllocBytes(5)
	copy(b, "hello")
	if string(b) != "hello" {
		t.Fatalf("expected %q, got %q", "hello", string(b))
	}
}

// TestAllocBytesIsolation checks that two consecutive AllocBytes calls return
// non-overlapping slices.
func TestAllocBytesIsolation(t *testing.T) {
	a := NewArena(256, 0)
	x := a.AllocBytes(8)
	y := a.AllocBytes(8)

	for i := range x {
		x[i] = 0xAA
	}
	for i := range y {
		y[i] = 0xBB
	}

	for i, v := range x {
		if v != 0xAA {
			t.Fatalf("x[%d] corrupted: got 0x%x, want 0xAA", i, v)
		}
	}
}

// TestAllocBytesAccountedInUsedBytes checks that AllocBytes contributes to UsedBytes.
func TestAllocBytesAccountedInUsedBytes(t *testing.T) {
	a := NewArena(256, 0)
	before := a.UsedBytes()
	const n = 64
	_ = a.AllocBytes(n)
	after := a.UsedBytes()
	if after-before < n {
		t.Fatalf("UsedBytes delta too small: before=%d after=%d, want delta>=%d", before, after, n)
	}
}

// TestAllocBytesAcrossChunkBoundary checks that AllocBytes works correctly
// when it triggers a chunk growth.
func TestAllocBytesAcrossChunkBoundary(t *testing.T) {
	const chunkSize = 32
	a := NewArena(chunkSize, 0)

	// Fill first chunk almost full.
	_ = a.AllocBytes(chunkSize - 1)

	// This allocation must cross into a new chunk.
	const bigN = 64
	b := a.AllocBytes(bigN)
	if len(b) != bigN {
		t.Fatalf("expected len=%d after chunk growth, got %d", bigN, len(b))
	}
	copy(b, strings.Repeat("z", bigN))
	for i, v := range b {
		if v != 'z' {
			t.Fatalf("b[%d] = %q after chunk growth, want 'z'", i, v)
		}
	}
}

