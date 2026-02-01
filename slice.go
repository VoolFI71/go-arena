package arena

import "unsafe"

// MakeSlice создает слайс в арене с заданной длиной и емкостью.
func MakeSlice[T any](a *Arena, length int, capacity int) []T {
	if length < 0 || capacity < 0 {
		panic("slice length and capacity must be non-negative")
	}
	if capacity < length {
		panic("cap must be >= len")
	}
	if capacity == 0 {
		return nil
	}

	elemSize := int(unsafe.Sizeof(*new(T)))
	elemAlign := int(unsafe.Alignof(*new(T)))
	if elemSize == 0 {
		return make([]T, length, capacity)
	}

	total := elemSize * capacity
	if total/elemSize != capacity {
		panic("slice size overflow")
	}

	buf := a.allocAligned(total, elemAlign)
	ptr := unsafe.Pointer(&buf[0])
	return unsafe.Slice((*T)(ptr), capacity)[:length]
}

// Append добавляет элементы в слайс, выделяя память в арене при нехватке cap.
func Append[T any](a *Arena, slice []T, items ...T) []T {
	if len(items) == 0 {
		return slice
	}
	if len(slice)+len(items) <= cap(slice) {
		return append(slice, items...)
	}

	newCap := cap(slice) * 2
	needed := len(slice) + len(items)
	if newCap < needed {
		newCap = needed
	}

	newSlice := MakeSlice[T](a, len(slice), newCap)
	copy(newSlice, slice)
	newSlice = append(newSlice, items...)
	return newSlice
}
