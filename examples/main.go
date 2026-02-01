package main

import (
	"fmt"
	"github.com/VoolFI71/go-arena"
)

type User struct {
	ID    int
	Age   int
	Name  string
	Email string
	Rate  float64
}

func main() {
	pool := arena.NewArenaPool(1024*1024, 0)
	mem := pool.Get()
	defer pool.Put(mem)

	users := arena.MakeSlice[User](mem, 0, 2)
	users = arena.Append(mem, users, User{ID: 1, Name: mem.AllocString("Ivan")})
	users = arena.Append(mem, users, User{ID: 2, Name: mem.AllocString("Petr")})
	users = arena.Append(mem, users, User{ID: 3, Name: mem.AllocString("Oleg")})

	fmt.Printf("Users count: %d, Cap: %d\n", len(users), cap(users))
	fmt.Printf("Last User: %+v\n", users[2])
}
