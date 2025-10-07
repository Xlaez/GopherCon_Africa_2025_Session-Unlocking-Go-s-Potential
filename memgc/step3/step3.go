package main

import (
	"fmt"
	"runtime"
	"time"
)

// Version 1: High escapes - returns pointer to local slice, stores in heap slice
func processDataEscaping(ids []int) *[]string {
	 // Escapes: returned as ptr
	results := make([]string, len(ids))
	for i, id := range ids {
		// Strings escape via fmt
		results[i] = fmt.Sprintf("item-%d", id)
	}
	 // Local slice escapes to heap
	return &results
}

// Version 2: Low escapes - returns value (copied to caller stack), no pointers
func processDataStack(ids []int) []string {
	// Fixed-size array on stack (assume <100 items)
	var results [100]string 
	 // Bounds check, but stack-friendly
	for i, id := range ids[:len(ids)] {
		if i < len(results) {
			results[i] = fmt.Sprintf("item-%d", id)
		}
	}
	 // Copies to caller stack; no heap alloc for slice
	return results[:len(ids)]
}

func printMem(prefix string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Printf("%s: Heap %d bytes (~%.1f MB), Allocs %d\n", prefix, m.HeapAlloc, float64(m.HeapAlloc)/1e6, m.Mallocs)
}

func main() {
	// Prep input  - Big loop to stress
	inputs := make([][]int, 10_000)
	for i := range inputs {
		// Small slices, but many
		inputs[i] = []int{i * 10, i*10 + 1, i*10 + 2}
	}

	printMem("Initial")

	start := time.Now()

	// Test 1: Escaping version - expect heap growth
	var allResultsEscaping [][]string
	for _, ids := range inputs {
		res := processDataEscaping(ids)
		 // Heap appends galore
		allResultsEscaping = append(allResultsEscaping, *res)
	}
	fmt.Printf("Escaping done in %v (results len: %d)\n", time.Since(start), len(allResultsEscaping))

	printMem("After escaping")

	// Reset heap-ish (GC will help later)
	runtime.GC()

	// Test 2: Stack version - minimal growth
	start = time.Now()
	var allResultsStack [][]string
	for _, ids := range inputs {
		res := processDataStack(ids)
		 // Still appends, but inner slices copied (less escape)
		allResultsStack = append(allResultsStack, res)
	}
	fmt.Printf("Stack done in %v (results len: %d)\n", time.Since(start), len(allResultsStack))

	printMem("After stack")

	totalDur := time.Since(start)
	printMem("Final")
	fmt.Printf("Total run: %v\n", totalDur)
}