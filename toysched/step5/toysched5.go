// On running this code we get a bug as seen in the logs below
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
	// May block inside!
	g.Func() 
	if g.blockChan != nil {
		fmt.Println("  G: Waiting for unblock signal...")
		<-g.blockChan // Block here.
		fmt.Println("  G: Resumed after unblock!")
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

	// To signal available Ps.
	handoff chan *P 
}

// Where M represents a Machine (OS thread)
type M struct {
	ID int

	// P bound to the Machine (if any)
	P *P

	// Current G being run (if any)
	G *G

	// To stop M's goroutine.
	stop    chan struct{} 
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
	// For work-stealing if local empty.
	globalQ  []*G
	 // Wait for all Ms.
	wg       sync.WaitGroup
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

		// Buffered for non-blocking send
		handoff: make(chan *P, 1),
	}
	s.Ps = append(s.Ps, p)
	return p
}

//  Schedules until stop
func (m *M) run(s *Scheduler) {
	defer s.wg.Done()
	for {
		select {
		case <-m.stop:
			return
		default:
			m.scheduleOnce(s)
		}
		time.Sleep(10 * time.Millisecond)
	}
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
		stop: make(chan struct{}),
	}

	s.Ms = append(s.Ms, m)
	s.wg.Add(1)
	go m.run(s)

	return m
}

// Enqueue adds a G to a P's run queue (FIFO).
func (s *Scheduler) Enqueue(p *P, g *G) {
	p.RunQ = append(p.RunQ, g)
	p.NumG++
	// Ensure that it is ready to be used.
	g.Status = "runnable"
}

// Like old Schedule, but async + steal.
func (m *M) scheduleOnce(s *Scheduler) {
	if m.P == nil {
		// No P: Try to grab one via handoff or idle pool (simplified: check Ps).
		s.mu.Lock()
		for _, p := range s.Ps {
			if len(p.handoff) > 0 {
				m.P = <-p.handoff
				fmt.Printf("M%d: Grabbed handed-off P%d\n", m.ID, m.P.ID)
				s.mu.Unlock()
				break
			}
		}

		s.mu.Unlock()
		if m.P == nil {
			return
		}
	}

	if m.P.NumG == 0 {
		// Local empty: Steal from global.
		s.mu.Lock()
		if len(s.globalQ) > 0 {
			g := s.globalQ[0]
			s.globalQ = s.globalQ[1:]
			m.P.RunQ = append(m.P.RunQ, g)
			m.P.NumG++
			fmt.Printf("M%d: Stole G%d from global to P%d\n", m.ID, g.ID, m.P.ID)
		}
		s.mu.Unlock()
		if m.P.NumG == 0 {
			// No work: Handoff P back (park M).
			fmt.Printf("M%d: Parking, handing off P%d\n", m.ID, m.P.ID)
			m.P.handoff <- m.P
			m.P = nil
			return
		}
	}

	// Run a G.
	g := m.P.RunQ[0]
	m.P.RunQ = m.P.RunQ[1:]
	m.P.NumG--

	m.G = g
	g.Status = "running"
	fmt.Printf("M%d on P%d: Starting G%d\n", m.ID, m.P.ID, g.ID)

	// This may block if chan wait!
	g.Run()

	m.G = nil
	if g.Status == "done" {
		fmt.Printf("M%d on P%d: Finished G%d\n", m.ID, m.P.ID, g.ID)
	} else {
		// Still blocked? Handoff P now (G stays on M, waiting).
		fmt.Printf("M%d on P%d: G%d blocked, handing off P\n", m.ID, m.P.ID, g.ID)
		m.P.handoff <- m.P
		m.P = nil
		// M stays with G, will resume when unblocked.
		return
	}
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

//Starts Ms, waits for wg, signals unblocks.
func (s *Scheduler) Run() {
	fmt.Println("=== Starting Toy Schedule ===")
	go func() {
		// Simulate async unblock after 500ms.
		time.Sleep(500 * time.Millisecond)
		s.mu.Lock()
		// Find blocked G (hacky: assume G2).
		for _, p := range s.Ps {
			// But queued won't block.
			for _, g := range p.RunQ { 
				if g.blockChan != nil {
					g.blockChan <- struct{}{}
					fmt.Println("Scheduler: Signaled unblock for G", g.ID)
					break
				}
			}
		}
		s.mu.Unlock()
	}()

	s.wg.Wait() // Wait for all Ms to stop (but we don't stop yet).
	fmt.Println("=== Schedule Complete ===")
}

func main() {
	sched := &Scheduler{}

	// 2 Ps, 2 Ms.
	p0 := sched.AddP(0)
	_ = sched.AddM(0, 0)
	p1 := sched.AddP(1)
	_ = sched.AddM(1, 1)


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

	sampleWork3 := func() {
		fmt.Println("  G3 doing some work...")
		for i := 0; i < 3; i++ {
			time.Sleep(100 * time.Millisecond)
			fmt.Printf("    Work step %d\n", i+1)
		}
		// After work, "block" on channel (simulates syscall after compute).
		fmt.Println("  G3: Entering block... (syscall sim)...")

		// Block here: Wait for signal.
		// Short pre-block.
		time.Sleep(200 * time.Millisecond)
		// But real block in Run()'s <-chan.
	}

	g0 := sched.NewG(sampleWork, false)
	g1 := sched.NewG(sampleWork2, false)

	// Manual for blockChan.
	g2 := sched.NewG(sampleWork3, true)

	sched.nextGID = 3

	sched.Enqueue(p0, g0)
	sched.Enqueue(p1, g1)
	sched.Enqueue(p0, g2)

	sched.Run()

	// Stop Ms after a bit.
	time.Sleep(2 * time.Second)
	for _, m := range sched.Ms {
		close(m.stop)
	}
}

/**
=== Starting Toy Schedule ===
M0 on P0: Starting G0
  G doing some work...
M1 on P1: Starting G1
  G2 doing some work...
    Work step 1
    Work step 1
    Work step 2
    Work step 2
    Work step 3
    Work step 3
Goroutine is done with task!
M1 on P1: Finished G1
Goroutine is done with task!
M0 on P0: Finished G0
M0 on P0: Starting G2
  G3 doing some work...
M1: Parking, handing off P1
M1: Grabbed handed-off P1
fatal error: sync: unlock of unlocked mutex

goroutine 19 [running]:
internal/sync.fatal({0x10aaa5e1?, 0xc000104000?})
	/usr/local/go/src/runtime/panic.go:1068 +0x18
internal/sync.(*Mutex).unlockSlow(0xc0000b4030, 0xffffffff)
	/usr/local/go/src/internal/sync/mutex.go:204 +0x35
internal/sync.(*Mutex).Unlock(...)
	/usr/local/go/src/internal/sync/mutex.go:198
sync.(*Mutex).Unlock(...)
	/usr/local/go/src/sync/mutex.go:65
main.(*M).scheduleOnce(0xc0000ac060, 0xc0000b4000)
	/Users/utee/Documents/gophercon_projects/toysched/step5/toysched5.go:171 +0x249
main.(*M).run(0xc0000ac060, 0xc0000b4000)
	/Users/utee/Documents/gophercon_projects/toysched/step5/toysched5.go:123 +0x5f
created by main.(*Scheduler).AddM in goroutine 1
	/Users/utee/Documents/gophercon_projects/toysched/step5/toysched5.go:144 +0x187

goroutine 1 [sync.WaitGroup.Wait]:
sync.runtime_SemacquireWaitGroup(0xc0000b2040?)
	/usr/local/go/src/runtime/sema.go:110 +0x25
sync.(*WaitGroup).Wait(0x10af3e88?)
	/usr/local/go/src/sync/waitgroup.go:118 +0x48
main.(*Scheduler).Run(0xc0000b4000)
	/Users/utee/Documents/gophercon_projects/toysched/step5/toysched5.go:258 +0x9b
main.main()
	/Users/utee/Documents/gophercon_projects/toysched/step5/toysched5.go:315 +0x4c6

*/