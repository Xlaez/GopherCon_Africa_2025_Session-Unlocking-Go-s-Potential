package main

import "fmt"

type User struct {
	Name string
 }

func makeUser(name string) *User {
    u := User{Name: name} 
    return &u  
}

func main() {
    // 'user' points to memory from the 'makeUser' function.
    // If 'u' had been on makeUser's stack, it would be gone now,
    // and 'user' would be a dangerous, invalid pointer.
    user := makeUser("Alice")
    
    // To prevent this, the compiler allocates 'u' on the heap.
    fmt.Println(user.Name)
}
