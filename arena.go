package arena

import "unsafe"

// Arena holds memory chunks and allocation cursor. / Arena хранит набор чанков памяти и курсор выделения.
//
// Field order is intentional: the three hot fields (offset, curStart, curEnd)
// are placed first in the struct body so they occupy the very start of the
// first active cache line (right after the leading false-sharing guard).
// Cold fields (chunks, chunkSize, maxRetain) follow and may spill to CL2,
// but they are only touched during chunk growth and Reset — not on every alloc.
type Arena struct {
	_ [64]byte // false-sharing guard / защита от false sharing

	// --- hot path (touched on every allocation) ---
	offset   int            // Cursor inside current chunk. / Курсор внутри текущего чанка.
	curStart unsafe.Pointer // Pointer to current chunk start. / Указатель на начало текущего чанка.
	curEnd   int            // Cached cap() of current chunk. / Кэшированный cap() текущего чанка.

	// --- warm path (touched on chunk switch) ---
	chunkIndex int // Current chunk index. / Индекс текущего чанка.

	// --- cold path (touched only during growth / Reset) ---
	chunkSize int      // Base chunk size. / Базовый размер чанка.
	maxRetain int      // Retained memory after Reset. / Сколько памяти оставляем после Reset.
	chunks    [][]byte // Chunk storage. / Набор чанков памяти.

	_ [64]byte // false-sharing guard / защита от false sharing
}

// NewArena creates an arena with fixed chunk size. / NewArena создает арену фиксированного размера чанка.
func NewArena(size int, maxRetained int) *Arena {
	if size <= 0 {
		panic("Arena size must be positive")
	}
	if maxRetained <= 0 {
		maxRetained = size * 10
	}

	firstChunk := make([]byte, size)
	chunks := make([][]byte, 0, 64)
	chunks = append(chunks, firstChunk)

	return &Arena{
		chunks:    chunks,
		chunkSize: size,
		curStart:  unsafe.Pointer(&firstChunk[0]),
		curEnd:    cap(firstChunk),
		maxRetain: maxRetained,
	}
}

// Reset resets cursors and trims memory by limit. / Reset сбрасывает курсоры и подрезает память по лимиту.
func (a *Arena) Reset() {
	a.chunkIndex = 0
	a.offset = 0

	if len(a.chunks) == 0 {
		firstChunk := make([]byte, a.chunkSize)
		a.chunks = [][]byte{firstChunk}
		a.curStart = unsafe.Pointer(&firstChunk[0])
		a.curEnd = cap(firstChunk)
		return
	}

	if cap(a.chunks[0]) > a.maxRetain {
		firstChunk := make([]byte, a.chunkSize)
		a.chunks = [][]byte{firstChunk}
		a.curStart = unsafe.Pointer(&firstChunk[0])
		a.curEnd = cap(firstChunk)
		return
	}

	a.curStart = unsafe.Pointer(&a.chunks[0][0])
	a.curEnd = cap(a.chunks[0])

	total := 0
	keepIndex := len(a.chunks)
	for i, chunk := range a.chunks {
		total += cap(chunk)
		if total > a.maxRetain {
			keepIndex = i + 1
			break
		}
	}

	if keepIndex < len(a.chunks) {
		a.chunks = a.chunks[:keepIndex]
	}
}

// AllocString copies string bytes into arena. / AllocString копирует байты строки внутрь арены.
func (a *Arena) AllocString(s string) string {
	length := len(s)
	if length == 0 {
		return ""
	}

	ptr := a.allocRaw(length, 1)
	buf := unsafe.Slice((*byte)(ptr), length)
	copy(buf, s)
	return unsafe.String((*byte)(ptr), length)
}

// AllocBytesToString copies []byte into arena and returns string. / AllocBytesToString копирует []byte в арену и возвращает как string.
func (a *Arena) AllocBytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	tmp := unsafe.String(unsafe.SliceData(b), len(b))
	return a.AllocString(tmp)
}

// AllocBytes reserves n bytes in the arena and returns them as a []byte.
// The returned slice is valid until the next Reset or pool.Put call.
//
// WARNING: Reset does NOT zero memory. The slice may contain residual data
// from previous allocations. Zero it yourself if the content is security-sensitive
// (e.g. passwords, PII) before passing it outside the request scope.
//
// Panics if n < 0. Returns nil for n == 0.
func (a *Arena) AllocBytes(n int) []byte {
	if n < 0 {
		panic("arena: AllocBytes called with negative size")
	}
	if n == 0 {
		return nil
	}
	return a.allocBytes(n)
}

func (a *Arena) allocBytes(size int) []byte {
	ptr := a.allocRaw(size, 1)
	if ptr == nil {
		return nil
	}
	return unsafe.Slice((*byte)(ptr), size)
}

func (a *Arena) allocRaw(size int, align int) unsafe.Pointer {
	if size <= 0 {
		return nil
	}
	if align <= 0 {
		align = 1
	}

	// align from unsafe.Alignof is power-of-two, so bit trick is safe. / align from unsafe.Alignof is a power of two, so bit trick is safe.
	padding := (-a.offset) & (align - 1)
	newOffset := a.offset + padding + size
	if newOffset <= a.curEnd {
		ptr := unsafe.Add(a.curStart, a.offset+padding)
		a.offset = newOffset
		return ptr
	}

	return a.growAndAlloc(size, align)
}

//go:noinline
func (a *Arena) growAndAlloc(size int, align int) unsafe.Pointer {
	a.ensure(size + align)
	return a.allocRaw(size, align)
}

func (a *Arena) ensure(size int) {
	if size <= cap(a.chunks[a.chunkIndex])-a.offset {
		return
	}

	if a.chunkIndex+1 < len(a.chunks) {
		nextChunk := a.chunks[a.chunkIndex+1]
		if size <= cap(nextChunk) {
			a.chunkIndex++
			a.offset = 0
			a.curStart = unsafe.Pointer(&nextChunk[0])
			a.curEnd = cap(nextChunk)
			return
		}
	}

	newSize := a.chunkSize
	if size > newSize {
		newSize = size
	}
	newChunk := make([]byte, newSize)
	a.chunks = append(a.chunks, newChunk)
	a.chunkIndex++
	a.offset = 0
	a.curStart = unsafe.Pointer(&newChunk[0])
	a.curEnd = cap(newChunk)
}

// New allocates object of type T inside arena. / New выделяет объект типа T внутри арены.
func New[T any](a *Arena) *T {
	size := int(unsafe.Sizeof(*new(T)))
	if size == 0 {
		var zero T
		return &zero
	}

	align := int(unsafe.Alignof(*new(T)))
	ptr := a.allocRaw(size, align)
	return (*T)(ptr)
}

func (a *Arena) UsedBytes() int {
	total := 0
	for i := 0; i < a.chunkIndex; i++ {
		total += cap(a.chunks[i])
	}
	total += a.offset
	return total
}
