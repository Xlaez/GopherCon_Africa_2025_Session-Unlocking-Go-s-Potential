package main

import (
	"fmt"
)

// Where G represents a Goroutine
type G struct {
	// Unique ID
	ID int

	// Function to be ran
	Func func()

	// "runnable", "running", "done"
	Status string
}

func (g *G) Run() {
	g.Func()
	g.Status = "done"
	fmt.Println("Done!!")
}

// Where P represents a Processor (logical CPU)
type P struct {
	ID int

	// Local run queue of Gs
	RunQ []*G

	// Current number of Gs in the queue
	NumG int
}

// Where M represents a Machine (OS thread)
type M struct {
	ID int

	// P bound to the Machine (if any)
	P *P

	// Current G being run (if any)
	G *G
}

/**
What we are achieving with this:

We'll create a package called toysched with structs for G (goroutine), P (processor), and M (machine). Each G will have a simple function to "run" (we'll simulate work). Ps will have a run queue (slice of Gs). Ms will hold a reference to a P.

What this does:
G: Holds the work (a closure) and state.
P: Owns a FIFO queue for local Gs.
M: Tracks what's it's running and its P.

In Step2, we will add the next layer: A central Scheduler to orchestrate everything. This will:

Hold slices of Ps and Ms.
Bind Ms to Ps (one-to-one for now).
Create a simple NewG helper to spawn Gs with auto-ID.
Enqueue a G to a P's run queue.
Have a basic Schedule method on M to pick and run a G from its P's queue.
We'll keep it minimal—no parallelism yet (we'll simulate with loops). This lets us test one M/P pair running one G.

*/