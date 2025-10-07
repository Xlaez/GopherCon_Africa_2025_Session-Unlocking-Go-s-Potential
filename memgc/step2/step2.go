package main

import (
	"fmt"
	"runtime"
	"sort"
	"time"
)

func printGCStats(prefix string) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	totalPause := uint64(0)
	for _, ns := range stats.PauseNs {
		totalPause += ns
	}
	fmt.Printf("%s: Heap %d bytes (~%.1f MB), GC runs: %d, Total pause: %v\n",
		prefix, stats.HeapAlloc, float64(stats.HeapAlloc)/1e6, stats.NumGC, time.Duration(totalPause))

	// Rough quantiles from PauseNs (last 256 pauses, most recent first; many may be 0)
	if len(stats.PauseNs) > 0 {
		pauses := make([]uint64, len(stats.PauseNs))
		copy(pauses, stats.PauseNs[:])
		sort.Slice(pauses, func(i, j int) bool { return pauses[i] < pauses[j] })
		n := len(pauses)
		fmt.Printf("  Pause quantiles (from %d samples): 50%%=%v, 95%%=%v, 99%%=%v\n",
			n, time.Duration(pauses[n/2]), time.Duration(pauses[int(float64(n)*0.95)]), time.Duration(pauses[int(float64(n)*0.99)]))
	}
}

func main() {
	printGCStats("Initial")

	// This slice will hold pointers to our allocations (forces heap growth)
	var retained []*[]byte 
	start := time.Now()

	// Allocate in smaller bursts over time; append to retained to keep live
	for epoch := 0; epoch < 5; epoch++ {
		burstStart := time.Now()
		// ~12.8MB per burst
		for i := 0; i < 200_000; i++ {
			slice := make([]byte, 64)
			retained = append(retained, &slice)
		}
		burstDur := time.Since(burstStart)
		fmt.Printf("Burst %d done in %v (retained now: %d items)\n", epoch+1, burstDur, len(retained))
		// Brief pause to let GC breathe
		time.Sleep(100 * time.Millisecond)
	}
	totalDur := time.Since(start)

	printGCStats("Final")
	fmt.Printf("Total run: %v\n", totalDur)
	// runtime.GC()
	printGCStats("After final forced GC")
}

/**
Take away:

Heap growth: ~110MB live (your 1M slices at ~72B each, plus overhead—Go rounds up allocations). The final forced GC doesn't reclaim because everything's still rooted in retained.
Default GOGC=100: 7 GCs (triggers ~every 100% growth), total pause ~0.37ms (super low—modern Go's concurrent marking shines). Bursts speed up later (27-41ms) as allocator warms up.
GOGC=10: 41 GCs (every ~10% growth), total pause ~1.9ms (higher count, but still tiny per-GC; 99th quantile ~100µs max). Tradeoff: More frequent but shorter pauses—great for latency-critical apps (e.g., games), but more CPU overhead.
No reclaim on forced GC: Heap stays ~104-110MB since live set is fixed.

Bursts vary due to append resizes (slice doubling) and occasional STW pauses mid-burst. Total run ~0.7-0.8s feels snappy, but imagine this in a loop—pauses could jitter latencies.
*/