package arena

import "unsafe"

// Arena хранит набор чанков памяти и курсор выделения.
type Arena struct {
	chunks     [][]byte // Набор чанков памяти (Тетради)
	chunkSize  int      // Базовый размер чанка
	chunkIndex int      // Индекс текущего чанка
	offset     int      // Курсор внутри чанка
	maxRetain  int      // Сколько памяти оставляем после Reset
}

// NewArena создает арену фиксированного размера чанка.
func NewArena(size int, maxRetained int) *Arena {
	if size <= 0 {
		panic("Arena size must be positive")
	}
	if maxRetained <= 0 {
		maxRetained = size * 10
	}

	chunks := make([][]byte, 0, 64)
	chunks = append(chunks, make([]byte, size))

	return &Arena{
		chunks:    chunks,
		chunkSize: size,
		maxRetain: maxRetained,
	}
}

// Reset сбрасывает курсоры и подрезает память по лимиту.
func (a *Arena) Reset() {
	a.chunkIndex = 0
	a.offset = 0

	if len(a.chunks) == 0 {
		a.chunks = [][]byte{make([]byte, a.chunkSize)}
		return
	}

	if cap(a.chunks[0]) > a.maxRetain {
		a.chunks = [][]byte{make([]byte, a.chunkSize)}
		return
	}

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

// AllocString копирует байты строки внутрь арены.
func (a *Arena) AllocString(s string) string {
	length := len(s)
	if length == 0 {
		return ""
	}

	buf := a.allocAligned(length, 1)
	ptr := unsafe.Pointer(&buf[0])
	copy(buf, s)

	return unsafe.String((*byte)(ptr), length)
}

func (a *Arena) allocBytes(size int) []byte {
	return a.allocAligned(size, 1)
}

func (a *Arena) allocAligned(size int, align int) []byte {
	if size <= 0 {
		return nil
	}
	if align <= 0 {
		align = 1
	}

	padding := (align - (a.offset % align)) % align
	if a.offset+padding+size > cap(a.chunks[a.chunkIndex]) {
		a.ensure(size)
		padding = 0
	}
	a.offset += padding

	a.ensure(size)
	chunk := a.chunks[a.chunkIndex]
	start := a.offset
	a.offset += size
	return chunk[start:a.offset]
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
			return
		}
	}

	newSize := a.chunkSize
	if size > newSize {
		newSize = size
	}
	a.chunks = append(a.chunks, make([]byte, newSize))
	a.chunkIndex++
	a.offset = 0
}

// New выделяет объект типа T внутри арены.
func New[T any](a *Arena) *T {
	size := int(unsafe.Sizeof(*new(T)))
	align := int(unsafe.Alignof(*new(T)))
	if size == 0 {
		var zero T
		return &zero
	}

	buf := a.allocAligned(size, align)
	itemPtr := unsafe.Pointer(&buf[0])
	return (*T)(itemPtr)
}
