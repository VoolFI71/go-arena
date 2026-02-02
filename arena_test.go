package arena

import (
	"fmt"
	"testing"
	"unsafe"
)

// sinkUser prevents compiler optimizations. / sinkUser предотвращает оптимизации компилятора.
var sinkUser *User

// sinkString prevents compiler optimizations. / sinkString предотвращает оптимизации компилятора.
var sinkString string

// sinkSlice prevents compiler optimizations. / sinkSlice предотвращает оптимизации компилятора.
var sinkSlice []User

type User struct {
	ID    int
	Age   int
	Name  string
	Email string
	Rate  float64
}

// counts is the size set for sub-benchmarks. / counts — набор размеров для суб-бенчмарков.
var counts = []int{100, 1_000, 10_000, 100_000, 1_000_000}

// BenchmarkNewObject compares per-object allocation. / BenchmarkNewObject сравнивает одиночные аллокации.
func BenchmarkNewObject(b *testing.B) {
	b.Run("Runtime", func(b *testing.B) {
		for _, count := range counts {
			b.Run(fmt.Sprintf("%d", count), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					for j := 0; j < count; j++ {
						u := new(User)
						u.ID = j
						sinkUser = u
					}
				}
			})
		}
	})

	b.Run("Arena", func(b *testing.B) {
		for _, count := range counts {
			b.Run(fmt.Sprintf("%d", count), func(b *testing.B) {
				// Preallocate arena to measure raw allocation speed. / Предвыделяем арену для чистого измерения.
				a := NewArena(count*int(unsafe.Sizeof(User{}))+1024, 0)
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					// Reset per iteration to avoid unbounded growth. / Сбрасываем на каждой итерации.
					a.Reset()
					for j := 0; j < count; j++ {
						u := New[User](a)
						u.ID = j
						sinkUser = u
					}
				}
			})
		}
	})
}

// BenchmarkAllocString compares string allocation paths. / BenchmarkAllocString сравнивает аллокации строк.
func BenchmarkAllocString(b *testing.B) {
	str := "Hello, Go Arena Performance!"

	b.Run("Runtime", func(b *testing.B) {
		for _, count := range counts {
			b.Run(fmt.Sprintf("%d", count), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					for j := 0; j < count; j++ {
						s := string([]byte(str))
						sinkString = s
					}
				}
			})
		}
	})

	b.Run("Arena", func(b *testing.B) {
		for _, count := range counts {
			b.Run(fmt.Sprintf("%d", count), func(b *testing.B) {
				a := NewArena(count*len(str)*2, 0)
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					a.Reset()
					for j := 0; j < count; j++ {
						s := a.AllocString(str)
						sinkString = s
					}
				}
			})
		}
	})
}

// BenchmarkMakeSlice compares slice allocation. / BenchmarkMakeSlice сравнивает аллокацию слайсов.
func BenchmarkMakeSlice(b *testing.B) {
	b.Run("Runtime", func(b *testing.B) {
		for _, count := range counts {
			b.Run(fmt.Sprintf("%d", count), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					sl := make([]User, count)
					sinkSlice = sl
				}
			})
		}
	})

	b.Run("Arena", func(b *testing.B) {
		for _, count := range counts {
			b.Run(fmt.Sprintf("%d", count), func(b *testing.B) {
				a := NewArena(count*int(unsafe.Sizeof(User{}))*2, 0)
				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; i < b.N; i++ {
					a.Reset()
					sl := MakeSlice[User](a, count, count)
					sinkSlice = sl
				}
			})
		}
	})
}
