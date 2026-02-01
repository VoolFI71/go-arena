package arena

import (
	"testing"
	"unsafe"
)

var (
	benchUserPtr *User
)

const BatchSize = 100_000

type User struct {
	ID    int
	Age   int
	Name  string
	Email string
	Rate  float64
}

func BenchmarkStandardNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	name := "Ivan"
	email := "ivan@example.com"

	for i := 0; i < b.N; i++ {
		u := new(User)
		u.ID = i
		u.Age = 30
		u.Name = name
		u.Email = email
		u.Rate = 99.9
		benchUserPtr = u
	}
}

func BenchmarkArenaNew(b *testing.B) {
	b.ReportAllocs()

	userSize := int(unsafe.Sizeof(User{}))
	if userSize == 0 {
		b.Fatal("user size is zero")
	}

	const stringDataSize = 20
	const padding = 12
	perItemSize := userSize + stringDataSize + padding
	if perItemSize <= 0 {
		b.Fatal("per item size is invalid")
	}

	totalSize := b.N * perItemSize
	if totalSize <= 0 {
		b.Fatal("total size is invalid")
	}
	arena := NewArena(totalSize, 0)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		u := New[User](arena)
		u.ID = i
		u.Age = 30
		u.Name = arena.AllocString("Ivan")
		u.Email = arena.AllocString("ivan@example.com")
		u.Rate = 99.9
		benchUserPtr = u
	}
}

func BenchmarkBatchStandard(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		users := make([]*User, BatchSize)
		for j := 0; j < BatchSize; j++ {
			u := &User{
				ID:   j,
				Age:  30,
				Name: "Ivan",
			}
			users[j] = u
		}
		benchUserPtr = users[len(users)-1]
	}
}

func BenchmarkBatchArena(b *testing.B) {
	b.ReportAllocs()

	mem := NewArena(BatchSize*100, 0)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mem.Reset()
		for j := 0; j < BatchSize; j++ {
			u := New[User](mem)
			u.ID = j
			u.Age = 30
			u.Name = mem.AllocString("Ivan")
		}
	}
}
