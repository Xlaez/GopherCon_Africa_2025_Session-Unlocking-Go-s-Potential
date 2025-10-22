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

	// If non-nil, signals block start/end.
	blockChan chan struct{}
}

func (g *G) Run() {
g.Func()
	if g.blockChan != nil {
		// For blocking Gs: After "work," wait for unblock signal.
		// (In real: Resume after syscall.)

		<-g.blockChan // Wait here.

		// Cleanup.
		close(g.blockChan) 
	}
	g.Status = "done"
	fmt.Println("Goroutine is done with task!")
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
	//For blocking handoff
	// Pool of unbound Ms
	idleMs []*M 
	// Flag to stop Run loop
	done   bool 
}

// NewG creates a new runnable G with auto-ID.
func (s *Scheduler) NewG(f func(), block bool) *G {
s.mu.Lock()
	id := s.nextGID
	s.nextGID++
	s.mu.Unlock()
	g := &G{
		ID:     id,
		Func:   f,
		Status: "runnable",
	}

	if block {
		g.blockChan = make(chan struct{})
	}
	return g
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

// Now detects blocking and hands off P.
func (m *M) Schedule(s *Scheduler) {
	if m.P == nil || m.P.NumG == 0 {
		// If no P, grab from idle or global (simple: mark as idle).
		s.mu.Lock()
		s.idleMs = append(s.idleMs, m)
		s.mu.Unlock()
		fmt.Printf("M%d: Idling (no P or Gs).\n", m.ID)
		return
	}

	// Pop front (FIFO).
	g := m.P.RunQ[0]
	m.P.RunQ = m.P.RunQ[1:]
	m.P.NumG--

	// Bind and run.
	m.G = g
	g.Status = "running"
	fmt.Printf("M%d on P%d: Starting G%d\n", m.ID, m.P.ID, g.ID)

	// Simulate blocking: If G has a blockChan, wait on it (mimics syscall/I/O).
	// (We'll set this in NewG for blocking Gs.)
	if g.blockChan != nil {
		fmt.Printf("  G%d: Blocking (e.g., I/O wait)...\n", g.ID)
		// M "blocks": Drop P to handoff, then wait.
		// Detach!
		m.P = nil 
		s.mu.Lock()
		 // Park this M
		s.idleMs = append(s.idleMs, m)
		s.mu.Unlock()

		// Simulate unblock after 1s (in real: syscall returns).
		time.Sleep(1 * time.Second)
		fmt.Printf("  G%d: Unblocked!\n", g.ID)

		// Re-grab a P? For now, we'll let the loop re-bind later.
		// (In real Go: M tries to steal a P after unblock.)

		// Unbind G (still "running" until full finish)
		m.G = nil 

		// Don't finish yet—G needs to resume.
		return 
	}

	// Normal run (no block).
	g.Run()

	// Unbind.
	m.G = nil
	g.Status = "done"
	fmt.Printf("M%d on P%d: Finished G%d\n", m.ID, m.P.ID, g.ID)
}

// Bind idle M to a needy P (called in Run loop).
func (s *Scheduler) bindIdleM(p *P) *M {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.idleMs) == 0 {
		return nil // No idle M
	}
	// Pop an idle M and bind.
	m := s.idleMs[0]
	s.idleMs = s.idleMs[1:]
	m.P = p
	fmt.Printf("Scheduler: Handed P%d to idle M%d\n", p.ID, m.ID)
	return m
}

// Run loop—simulates the scheduler heart: Poll Ms until all done.
func (s *Scheduler) Run() {
	fmt.Println("=== Starting Toy Schedule ===")
	for !s.done {
		// allIdle := true
		for _, m := range s.Ms {
			if m.P != nil && m.P.NumG > 0 {
				// allIdle = false
				 // Run one step
				m.Schedule(s)
			}
		}
		// Check if all queues empty.
		empty := true
		for _, p := range s.Ps {
			if p.NumG > 0 {
				empty = false
				// Try to bind an idle M to this P.
				s.bindIdleM(p)
				break
			}
		}
		if empty {
			s.done = true
		}
		// Simulate tick; prevents busy loop.
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Println("=== Schedule Complete ===")
}

func main() {
	sched := &Scheduler{}

	// Create 2 Ps and 2 Ms.
	p0 := sched.AddP(0)

	 // M0 on P0
	sched.AddM(0, 0)
	p1 := sched.AddP(1)

	// M1 on P1
	sched.AddM(1, 1)


	// The 3 Gs, but make G2 "blocking."

	sampleWork := func() {
		fmt.Println("  G doing some work...")
		for i := 0; i < 3; i++ {
			time.Sleep(100 * time.Millisecond)
			fmt.Printf("    Work step %d\n", i+1)
		}
	}

	sampleWork2 := func() {
		fmt.Println("  G2 doing some work...")
		for i := 0; i < 3; i++ {
			time.Sleep(100 * time.Millisecond)
			fmt.Printf("    Work step %d\n", i+1)
		}
	}

	// G2: Blocking version.
	sampleWork3 := func() {
		fmt.Println("  G3 doing some work...")
		for i := 0; i < 3; i++ {
			time.Sleep(100 * time.Millisecond)
			fmt.Printf("    Work step %d\n", i+1)
		}
		// After work, "block" on channel (simulates syscall after compute).
		fmt.Println("  G3: Entering block...")
	}

	g0 := sched.NewG(sampleWork, false)
	g1 := sched.NewG(sampleWork2, false)

	// Manual for blockChan.
	g2 := sched.NewG(sampleWork3, true)

	sched.nextGID = 3

	// Enqueue: Distribute for demo.
	sched.Enqueue(p0, g0)
	sched.Enqueue(p1, g1)

	 // G2 on P0, will block.
	sched.Enqueue(p0, g2)

	// Run the scheduler loop.
	sched.Run()

	// Manually unblock G2 after start (simulate syscall return).
	// In real: Happens async.

	 // Let blocking happen first.
	time.Sleep(500 * time.Millisecond)
	if g2.blockChan != nil {
		// Signal unblock.
		g2.blockChan <- struct{}{}
		fmt.Println("Scheduler: Signaled unblock for G3")
	}

	// Give time for resume (loop already runs, but wait for full finish).
	time.Sleep(1 * time.Second)
}

/*
Why was there a deadlock?

1. sched.Run() starts.

2. M0 runs G0. g0.Run() is called and completes.

3. M1 runs G1. g1.Run() is called and completes.

4. The scheduler loop (sched.Run()) continues. P0 still has G2. It binds an idle M (M0) to P0.

5. M0 calls Schedule() on G2.

6. It prints M0 on P0: Starting G2.

7. It enters the if g.blockChan != nil block.

8. It prints G2: Blocking (e.g., I/O wait)....

9. It detaches P, adds itself to idleMs, and sleeps for 1 second.

10. It wakes up, prints G2: Unblocked!, and returns from Schedule().

11. g2.Run() was never called. G2's function (sampleWork3) never ran.

12. The sched.Run() loop continues. All processor queues (P0, P1) are now empty.

13. s.done is set to true, the loop terminates, and sched.Run() prints === Schedule Complete === and returns.

14. Execution returns to main().

15. main() sleeps for 500ms.

16. main() tries to send on g2.blockChan at g2.blockChan <- struct{}{}.

17. Since g2.Run() was never called, no goroutine is waiting to receive from that channel.

18. Deadlock: main is blocked forever.
*/