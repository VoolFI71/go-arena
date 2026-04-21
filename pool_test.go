package arena

import (
	"bytes"
	"sync"
	"testing"

	"github.com/valyala/bytebufferpool"
)

// benchmarkWork simulates per-request work on an arena. / benchmarkWork эмулирует работу одного запроса c ареной.
func benchmarkWork(mem *Arena, objectsPerIter int, payload string) {
	for i := 0; i < objectsPerIter; i++ {
		u := New[User](mem)
		u.ID = i
		u.Name = mem.AllocString(payload)
	}
}

// BenchmarkArenaPoolThroughput measures Get/Put + typical work pattern under high parallelism.
// BenchmarkArenaPoolThroughput измеряет Get/Put и типичную работу с ареной при высокой конкуренции.
func BenchmarkArenaPoolThroughput(b *testing.B) {
	const (
		chunkSize      = 64 * 1024 // size of arena chunk in bytes / размер чанка арены в байтах
		objectsPerIter = 256       // how many objects we allocate per request / сколько объектов аллоцируем за один запрос
		strPayload     = "payload" // example string payload / пример строкового payload
	)

	p := NewArenaPool(chunkSize, 0)
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mem := p.Get()
			benchmarkWork(mem, objectsPerIter, strPayload)
			p.Put(mem)
		}
	})

	// Log pool metrics for additional insight. / Логируем метрики пула для анализа.
	gets := p.Metrics.GetCount.Load()
	totalCap := p.Metrics.TotalCapacityBytes.Load()
	used := p.Metrics.TotalUsedBytes.Load()
	active := p.Metrics.ActiveArenas.Load()
	var avgUsed float64
	if gets > 0 {
		avgUsed = float64(used) / float64(gets)
	}
	b.Logf("ArenaPoolThroughput: gets=%d active=%d totalCap=%dB avgUsedPerGet=%.1fB",
		gets, active, totalCap, avgUsed)
}

// BenchmarkArenaPoolVsRaw compares three strategies:
//  1. Using ArenaPool (Get/Put per request)
//  2. Reusing a single Arena with Reset()
//  3. Allocating a new Arena per request without reuse (worst case)
//
// BenchmarkArenaPoolVsRaw сравнивает три стратегии:
//  1. ArenaPool (Get/Put на запрос)
//  2. Одна арена с повторным использованием через Reset()
//  3. Новая арена на каждый запрос (худший случай)
func BenchmarkArenaPoolVsRaw(b *testing.B) {
	const (
		chunkSize      = 64 * 1024
		objectsPerIter = 256
		strPayload     = "payload"
	)

	b.Run("Pool", func(b *testing.B) {
		p := NewArenaPool(chunkSize, 0)
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			mem := p.Get()
			benchmarkWork(mem, objectsPerIter, strPayload)
			p.Put(mem)
		}

		gets := p.Metrics.GetCount.Load()
		totalCap := p.Metrics.TotalCapacityBytes.Load()
		used := p.Metrics.TotalUsedBytes.Load()
		var avgUsed float64
		if gets > 0 {
			avgUsed = float64(used) / float64(gets)
		}
		b.Logf("Pool: gets=%d totalCap=%dB avgUsedPerGet=%.1fB", gets, totalCap, avgUsed)
	})

	b.Run("SingleArena", func(b *testing.B) {
		mem := NewArena(chunkSize, 0)
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			mem.Reset()
			benchmarkWork(mem, objectsPerIter, strPayload)
		}
	})

	b.Run("NoPoolNoReset", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			mem := NewArena(chunkSize, 0)
			benchmarkWork(mem, objectsPerIter, strPayload)
			// No Reset and no reuse. / Без Reset и без переиспользования.
		}
	})
}

var sinkLen int

func BenchmarkAllocationCompetitors(b *testing.B) {
	const objectsPerIter = 256

	b.Run("RuntimeHeap", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for j := 0; j < objectsPerIter; j++ {
				u := &User{ID: j, Name: "payload"}
				sinkUser = u
			}
		}
	})

	b.Run("sync.Pool", func(b *testing.B) {
		var userPool sync.Pool
		userPool.New = func() any { return new(User) }

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			for j := 0; j < objectsPerIter; j++ {
				u := userPool.Get().(*User)
				u.ID = j
				u.Name = "payload"
				sinkUser = u
				*u = User{}
				userPool.Put(u)
			}
		}
	})

	b.Run("GoArenaPool", func(b *testing.B) {
		p := NewArenaPool(64*1024, 0)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mem := p.Get()
			benchmarkWork(mem, objectsPerIter, "payload")
			p.Put(mem)
		}
	})
}

func BenchmarkBufferCompetitors(b *testing.B) {
	payload := bytes.Repeat([]byte("x"), 4096)

	b.Run("RuntimeBytesToString", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s := string(payload)
			sinkString = s
			sinkLen = len(s)
		}
	})

	b.Run("bytebufferpool", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buf := bytebufferpool.Get()
			buf.Write(payload)
			s := string(buf.B)
			sinkString = s
			sinkLen = len(s)
			bytebufferpool.Put(buf)
		}
	})

	b.Run("GoArena", func(b *testing.B) {
		mem := NewArena(64*1024, 0)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mem.Reset()
			s := mem.AllocBytesToString(payload)
			sinkString = s
			sinkLen = len(s)
		}
	})
}
