package arena

import (
	"sync"
	"sync/atomic"
)

type PoolMetrics struct {
	TotalCapacityBytes atomic.Uint64
	_                  [56]byte // Protect against False Sharing
	ActiveArenas       atomic.Int64
	_                  [56]byte
	GetCount           atomic.Uint64
	_                  [56]byte
	TotalUsedBytes     atomic.Uint64
	_                  [56]byte
}

// PoolMetricsSnapshot is a plain copy of metrics values. / PoolMetricsSnapshot — снепшот значений метрик.
type PoolMetricsSnapshot struct {
	TotalCapacityBytes uint64
	ActiveArenas       int64
	GetCount           uint64
	TotalUsedBytes     uint64
}

// ArenaPool stores and reuses arenas. / ArenaPool хранит и переиспользует арены.
type ArenaPool struct {
	pool        sync.Pool
	chunkSize   int
	maxRetained int
	Metrics     PoolMetrics
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
		p.Metrics.TotalCapacityBytes.Add(uint64(chunkSize))
		return NewArena(p.chunkSize, p.maxRetained)
	}
	return p
}

// Get returns an arena from the pool. / Get возвращает арену из пула.
func (p *ArenaPool) Get() *Arena {
	p.Metrics.GetCount.Add(1)
	p.Metrics.ActiveArenas.Add(1)
	return p.pool.Get().(*Arena)
}

// Put returns an arena to the pool. / Put возвращает арену в пул.
func (p *ArenaPool) Put(a *Arena) {
	if a == nil {
		return
	}
	used := uint64(a.UsedBytes())
	p.Metrics.TotalUsedBytes.Add(used)
	p.Metrics.ActiveArenas.Add(-1)
	a.Reset()
	p.pool.Put(a)
}

// MetricsSnapshot returns a copy of current metrics values (atomic-safe read).
// MetricsSnapshot возвращает копию текущих значений метрик (без гонок и блокировок).
func (p *ArenaPool) MetricsSnapshot() PoolMetricsSnapshot {
	return PoolMetricsSnapshot{
		TotalCapacityBytes: p.Metrics.TotalCapacityBytes.Load(),
		ActiveArenas:       p.Metrics.ActiveArenas.Load(),
		GetCount:           p.Metrics.GetCount.Load(),
		TotalUsedBytes:     p.Metrics.TotalUsedBytes.Load(),
	}
}
