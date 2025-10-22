package main

import (
	"fmt"
	"runtime"
)

func main() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("Initial heap: %d bytes\n", m.HeapAlloc)

	// Burst allocate 1M small objects (e.g., 64-byte structs)
	for i := 0; i < 1_000_000; i++ {
		 // Small allocation to simulate object creation
		_ = make([]byte, 64)
	}

	// Force a GC for demo
	runtime.GC()

	runtime.ReadMemStats(&m)
	fmt.Printf("After allocation + forced GC: heap %d bytes, GC runs: %d\n", m.HeapAlloc, m.NumGC)
}

/*
What this does:

Reads initial heap size.
Allocates 1M tiny slices (~64MB total) – this grows the heap and likely triggers GC.
Forces a GC
Prints final stats.
*/

/*
What did you notice?
The heap size was not actually affected because the GC is so smart and it performs an escape analysis
*/