package main

import (
	"fmt"
	"sync"
	"time"
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

type Scheduler struct {
	Ps []*P
	Ms []*M
	// For safe ID allocation
	mu sync.Mutex
	// Global counter for G IDs
	nextGID int
}

// NewG creates a new runnable G with auto-ID.
func (s *Scheduler) NewG(f func()) *G {
	s.mu.Lock()
	id := s.nextGID
	s.nextGID++
	s.mu.Unlock()

	return &G{
		ID:     id,
		Func:   f,
		Status: "runnable", // In the real-world we'll favour using enums over direct strings for status
	}
}

// AddP creates and adds a P to the scheduler.
func (s *Scheduler) AddP(id int) *P {
	p := &P{
		ID:   id,
		RunQ: make([]*G, 0),
		NumG: 0,
	}
	s.Ps = append(s.Ps, p)
	return p
}

// AddM creates an M and binds it to a P (by index).

func (s *Scheduler) AddM(id, pIndex int) *M {
	if pIndex >= len(s.Ps) {
		panic("P index out of bounds")
	}

	m := &M{
		ID: id,
		P:  s.Ps[pIndex],
		G:  nil,
	}

	s.Ms = append(s.Ms, m)

	return m
}

// Enqueue adds a G to a P's run queue (FIFO).
func (s *Scheduler) Enqueue(p *P, g *G) {
	p.RunQ = append(p.RunQ, g)
	p.NumG++
	// Ensure that it is ready to be used.
	g.Status = "runnable"
}

// Schedule runs one G from the M's P queue (simple poll).
func (m *M) Schedule() {
	if m.P == nil || m.P.NumG == 0 {
		fmt.Printf("M%d: No P or no Gs to run.\n", m.ID)
		return
	}

	// Pop front (FIFO): Get first G.
	g := m.P.RunQ[0]
	m.P.RunQ = m.P.RunQ[1:]
	m.P.NumG--

	m.G = g
	g.Status = "running"
	fmt.Printf("M%d on P%d: Starting G%d\n", m.ID, m.P.ID, g.ID)

	// Execute the function
	g.Run()

	// Unbind
	m.G = nil
	g.Status = "done"
	fmt.Printf("M%d on P%d: Finished G%d\n", m.ID, m.P.ID, g.ID)
}

func main() {

	sched := &Scheduler{}

	// Create 1 P and 1 M bound to it.
	p0 := sched.AddP(0)
	m0 := sched.AddM(0, 0)

	// Create a sample G: Simulate work with a sleep and print.
	sampleWork := func() {
		fmt.Println("  G doing some work...")
		// Simulate CPU-bound work (no sleep for pure compute sim, but sleep for demo visibility).
		for i := 0; i < 3; i++ {
			time.Sleep(100 * time.Millisecond)
			fmt.Printf("    Work step %d\n", i+1)
		}
	}
	
	sampleWork2 := func() {
		fmt.Println("  G2 doing some work...")
		// Simulate CPU-bound work (no sleep for pure compute sim, but sleep for demo visibility).
		for i := 0; i < 3; i++ {
			time.Sleep(100 * time.Millisecond)
			fmt.Printf("    Work step %d\n", i+1)
		}
	}

	sampleWork3 := func() {
		fmt.Println("  G3 doing some work...")
		// Simulate CPU-bound work (no sleep for pure compute sim, but sleep for demo visibility).
		for i := 0; i < 3; i++ {
			time.Sleep(100 * time.Millisecond)
			fmt.Printf("    Work step %d\n", i+1)
		}
	}

	g0 := sched.NewG(sampleWork)
	g1 := sched.NewG(sampleWork2)
	g2 := sched.NewG(sampleWork3)

	// Enqueue to P0.
	sched.Enqueue(p0, g0)
	sched.Enqueue(p0, g1)
	sched.Enqueue(p0, g2)

	// Run the scheduler: Just call Schedule on M0 once.
	fmt.Println("=== Starting Toy Schedule ===")

	for i := 0; i < 2; i++ {
		m0.Schedule()
	}

	fmt.Println("=== Schedule Complete ===")
}

/*
Question:  Why did the logs not contain all the expected results?
- 
*/