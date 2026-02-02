# Go Arena: High-Performance Zero-GC Allocator

![Go Version](https://img.shields.io/badge/Go-1.18+-00ADD8?logo=go&logoColor=white)
[![Go Report Card](https://goreportcard.com/badge/github.com/VoolFI71/go-arena)](https://goreportcard.com/report/github.com/VoolFI71/go-arena)
Go Arena is a memory allocator for Go designed for systems with extreme latency requirements (C10M, HFT, GameDev).

The idea is simple: instead of millions of tiny heap allocations and GC pauses, we use a bump-pointer arena that resets in O(1). The garbage collector stops seeing the "small trash," and latency becomes flat.

## Benchmarks

Comparisons performed on Intel Core i5-12450H (Windows/amd64). Lower is better.

### 1. Stress Test (Real World Scenario)
Simulation of a loaded HTTP server (log parsing, DTO creation).
* Load: 1,000,000 requests.
* Concurrency: 100 workers.
* Heap Pressure: 5 million live objects in the background.

| Metric | Standard Heap | Go Arena | Result |
|---|---|---|---|
| Throughput | 53,251 RPS | 104,454 RPS | 2x faster |
| Max Latency | 378 ms | 72 ms | 5x more stable |
| p99 Latency | 5.6 ms | 2.5 ms | Lower tail latency |
| GC Pause | Stop-The-World | Zero | No GC pressure |

### 2. Performance Benchmarks (1,000,000 ops)

| Operation (1M ops) | Standard Go (Runtime) | Go Arena | Speedup | Allocs/Op (Arena) |
|---|---|---|---|---|
| Struct Allocation | 35.3 ns/op | 7.5 ns/op | ~4.7x | 0 |
| String Copy | 17.1 ns/op | 6.5 ns/op | ~2.6x | 0 |
| Make Slice (1M len) | 4,467,774 ns/op | 7.4 ns/op | ~600,000x* | 0 |

> Note: MakeSlice in Arena is O(1) (pointer bump), while Runtime make is O(N) due to memory zeroing. This provides massive performance gains for large temporary buffers.

<details>
<summary>Full raw benchmark output</summary>

BenchmarkNewObject/Runtime/1000000-12         34          35328644 ns/op        64000062 B/op    1000000 allocs/op
BenchmarkNewObject/Arena/1000000-12          138           7549980 ns/op               0 B/op          0 allocs/op
BenchmarkAllocString/Runtime/1000000-12       78          17134946 ns/op        32000003 B/op    1000000 allocs/op
BenchmarkAllocString/Arena/1000000-12        202           6577214 ns/op               0 B/op          0 allocs/op
BenchmarkMakeSlice/Runtime/1000000-12        241           4467774 ns/op        56000512 B/op          1 allocs/op
BenchmarkMakeSlice/Arena/1000000-12      157475995                7.408 ns/op           0 B/op          0 allocs/op

</details>

---

## Installation

```bash
go get github.com/VoolFI71/go-arena
```

## Examples
### 1. HTTP / TCP server (Arena Pool pattern)
This is the primary scenario. Use ArenaPool to reuse memory between requests.

```go
package main

import (
    "fmt"
    "github.com/VoolFI71/go-arena"
)

// Global pool. 64KB is enough for most HTTP requests.
var pool = arena.NewArenaPool(64*1024, 0)

func HandleLog(ctx *fasthttp.RequestCtx) {
    // 1. Get an arena from the pool
    mem := pool.Get()
    defer pool.Put(mem)

    // 2. Allocate a struct inside the arena
    u := arena.New[UserData](mem)

    // 3. Strings without heap allocations
    u.Name = mem.AllocBytesToString(ctx.PostBody())

    ctx.SetStatusCode(200)
}
```

### 2. Slices (MakeSlice + Append)
Shows the correct `append` replacement that requires the arena argument.

```go
// 4. Work with dynamic data
func DynamicWork(mem *arena.Arena) {
    // Create an empty slice in the arena
    items := arena.MakeSlice[int](mem, 0, 10)

    // Use arena.Append instead of built-in append
    for i := 0; i < 100; i++ {
        items = arena.Append(mem, items, i)
    }
    // Even if the slice grows 10x, allocations stay inside the arena
}
```

### 3. Reset in a loop
Useful for long loops in a single goroutine without a pool.

```go
func Worker() {
    mem := arena.NewArena(1024*1024, 0) // 1MB arena

    for i := 0; i < 1000; i++ {
        processIteration(mem)
        mem.Reset() // Clear everything from this iteration in O(1)
    }
}
```

## The Safety Contract
Manual memory management requires discipline. In short:
- Scope Limit: do not return pointers to arena objects outside their lifetime (after pool.Put or Reset).
- Concurrency: Arena is not thread-safe. Use ArenaPool for parallel use.
- Data Independence: if you need long-lived data, make a copy (serialize).

### Anti-patterns (do not do this)
```go
// ERROR: Returning a pointer to arena memory
func BadFunction() *User {
    mem := pool.Get()
    defer pool.Put(mem) // Memory is cleared here!

    u := arena.New[User](mem)
    return u // <--- DANGER! Caller gets a dangling pointer.
}

// ERROR: Using one arena from multiple goroutines
func AsyncLogic(mem *arena.Arena) {
    go func() {
        // Arena is not thread-safe (except Pool).
        // This will cause a data race.
        _ = arena.New[int](mem)
    }()
}
```

### Correct approach
```go
// If you need to return data, serialize it
func GoodFunction() []byte {
    mem := pool.Get()
    defer pool.Put(mem)

    u := arena.New[User](mem)
    // ... заполняем u ...

    return json.Marshal(u) // Return a copy independent from the arena
}
```

## API Reference
### Allocators
- `New[T](a *Arena) *T` — allocates an object of type T in the arena.
- `MakeSlice[T](a *Arena, len, cap int) []T` — creates a slice.
- `AllocString(s string) string` — copies a string/bytes into the arena.
- `AllocBytesToString(b []byte) string` — copies []byte into the arena and returns string.

### Helper functions
- `Append(a *Arena, slice []T, items ...T) []T` — append equivalent that stays inside the arena.

### Memory management
- `NewArenaPool(chunkSize, maxRetained int) *ArenaPool` — thread-safe pool (recommended).
- `Reset()` — instant arena cleanup (cursor -> 0).

## Under the hood (Architecture)
Go Arena uses a linked list of memory chunks ([][]byte).
- On creation, you get a single chunk (e.g., 64KB).
- New[T] advances the allocation cursor inside the current chunk.
- When the chunk is full, a new chunk is allocated and used.
- Reset does not free chunks, it just resets indices for instant reuse.
> Tip: choose chunkSize in ArenaPool so it covers ~90% of typical requests. This minimizes new chunk allocations.


## License
MIT
