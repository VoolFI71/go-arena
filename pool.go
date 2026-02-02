package arena

import "sync"

// ArenaPool stores and reuses arenas. / ArenaPool хранит и переиспользует арены.
type ArenaPool struct {
	pool        sync.Pool
	chunkSize   int
	maxRetained int
}

// NewArenaPool creates an arena pool. / NewArenaPool создает пул арен.
func NewArenaPool(chunkSize int, maxRetained int) *ArenaPool {
	if chunkSize <= 0 {
		panic("ArenaPool chunk size must be positive")
	}
	if maxRetained <= 0 {
		maxRetained = chunkSize * 10
	}

	p := &ArenaPool{
		chunkSize:   chunkSize,
		maxRetained: maxRetained,
	}
	p.pool.New = func() any {
		return NewArena(p.chunkSize, p.maxRetained)
	}
	return p
}

// Get returns an arena from the pool. / Get возвращает арену из пула.
func (p *ArenaPool) Get() *Arena {
	a := p.pool.Get().(*Arena)
	a.Reset()
	return a
}

// Put returns an arena to the pool. / Put возвращает арену в пул.
func (p *ArenaPool) Put(a *Arena) {
	if a == nil {
		return
	}
	a.Reset()
	p.pool.Put(a)
}
