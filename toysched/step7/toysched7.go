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
}

// Where M represents a Machine (OS thread)
// Add cooldown for anti-thrash
type M struct {
	ID int

	// P bound to the Machine (if any)
	P *P

	// Current G being run (if any)
	G *G

	// To stop M's goroutine.
	stop    chan struct{} 
	parkTime time.Time
}

type Scheduler struct {
	Ps []*P
	Ms []*M
	// For safe ID allocation
	mu sync.Mutex 
	// Global counter for G IDs
	nextGID int   
	// For work-stealing if local empty.
	globalQ  []*G
	 // Wait for all Ms.
	wg       sync.WaitGroup
	// Central pool for available Ps (buffered to avoid send blocks).
	availPs chan *P
}

// NewG creates a new runnable G with auto-ID.
func (s *Scheduler) NewG(f func(), block bool) *G {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextGID
	s.nextGID++
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
		// Slower tick to reduce spam.
		time.Sleep(100 * time.Millisecond)
	}
}

// AddM creates an M and binds it to a P (by index).
func (s *Scheduler) AddM(id, pIndex int) *M {
	if pIndex >= len(s.Ps) {
		panic("P index out of bounds")
	}

	// Init central availPs if first M.
	if s.availPs == nil {
		s.availPs = make(chan *P, len(s.Ps))
	}

	m := &M{
		ID: id,
		P:  s.Ps[pIndex],
		stop: make(chan struct{}),
		// Initialise time as zero
		parkTime: time.Time{},
	}

	s.Ms = append(s.Ms, m)
	s.wg.Add(1)
	go m.run(s)

	return m
}

// Enqueue adds a G to a P's run queue (FIFO).
// Add simple overflow to globalQ for stealing demo
func (s *Scheduler) Enqueue(p *P, g *G) {
if p.NumG > 5 { 
		s.mu.Lock()
		s.globalQ = append(s.globalQ, g)
		s.mu.Unlock()
		fmt.Printf("Overflow: Enqueued G%d to globalQ (P%d full)\n", g.ID, p.ID)
		return
	}
	p.RunQ = append(p.RunQ, g)
	p.NumG++
	g.Status = "runnable"
}

// Like old Schedule, but async + steal
// Add cooldown skip + stealing from global
func (m *M) scheduleOnce(s *Scheduler) {
	
	if m.P == nil {
			// Cooldown: Skip grab right after park
			if !m.parkTime.IsZero() && time.Since(m.parkTime) < 200*time.Millisecond {
				return
			}

			// Reset cooldown
			m.parkTime = time.Time{}

			// Non-block grab
			select {
			case p := <-s.availPs:
				m.P = p
				fmt.Printf("M%d: Grabbed available P%d\n", m.ID, m.P.ID)
			default:
				return
			}
		}


	if m.P.NumG == 0 {
		// Local empty: Steal from global
		s.mu.Lock()
		if len(s.globalQ) > 0 {
			g := s.globalQ[0]
			s.globalQ = s.globalQ[1:]
			m.P.RunQ = append(m.P.RunQ, g)
			m.P.NumG++
			fmt.Printf("M%d: Stole G%d from global to P%d\n", m.ID, g.ID, m.P.ID)
			s.mu.Unlock()
		}else{ 
			s.mu.Unlock()
			fmt.Printf("M%d: Parking, handing off P%d\n", m.ID, m.P.ID)
			s.availPs <- m.P
			m.P = nil
			// Start cool down
			m.parkTime = time.Now()
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
		fmt.Printf("M%d on P%d: G%d blocked, handing off P\n", m.ID, m.P.ID, g.ID)
		s.availPs <- m.P
		m.P = nil
		m.parkTime = time.Now()
		// M stays with G, will resume when unblocked
		return
	}
}

//Starts Ms, waits for wg, signals unblocks.
func (s *Scheduler) Run() {
	fmt.Println("=== Starting Toy Schedule ===")

	// go func() {
	// 		// Simulate async unblock after 500ms.
	// 		time.Sleep(500 * time.Millisecond)
	// 		s.mu.Lock()
	// 		// Find any blocked G on running Ms.
	// 		for _, m := range s.Ms {
	// 			if m.G != nil && m.G.blockChan != nil {
	// 				m.G.blockChan <- struct{}{}
	// 				fmt.Println("Scheduler: Signaled unblock for G", m.G.ID)
	// 				break // One at a time.
	// 			}
	// 		}
	// 		s.mu.Unlock()
	// 	}()

	// 	 // Wait for all Ms to stop (but we don't stop yet).
	// s.wg.Wait()
	// fmt.Println("=== Schedule Complete ===")
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
	}

	g0 := sched.NewG(sampleWork, false)
	g1 := sched.NewG(sampleWork2, false)
	g2 := sched.NewG(sampleWork3, true)

	sched.Enqueue(p0, g0)
	sched.Enqueue(p1, g1)
	sched.Enqueue(p0, g2)

	// Manual unblock: No race
	go func() {
		// Tune to hit during/after G2 work.
		time.Sleep(800 * time.Millisecond)
		g2.blockChan <- struct{}{}
		fmt.Println("Manual: Signaled unblock for G2")
	}()

	sched.Run()

	// Stop Ms after a bit.
	time.Sleep(3 * time.Second)
	for _, m := range sched.Ms {
		close(m.stop)
	}
	
	// sched.wg.Wait()
	fmt.Println("=== Schedule Complete ===")
}



/**
Error and fixes:

The Thrash Loop (M1 Grabbing/Parking P1 Forever): After G1 finishes, P1 empties. M1 parks it to the central availPs buffer. Next tick, M1 (idle) grabs the same P1 back from the buffer (since it's the only available), sees empty again, parks it... repeat. The buffer (cap=2) enables this ping-pong. It's harmless (M1 idles effectively), but spammy. Real Go avoids this via work-stealing (Ms try to steal Gs from other Ps before parking) and smarter P assignment.
The Panic (Nil Pointer in Unblock Goroutine): The scan in the unblock go func() races with scheduleOnce() (concurrent sets m.G = nil after g.Run() finishes). Even with checks, accessing m.G.ID (or the send) can hit a half-nil state mid-race. Plus, no "Signaled unblock" print means it panics right at/after the send (e.g., fmt on a racing m.G). Go's runtime hates racy reads.
Hidden Hang (wg.Wait() Deadlock): Run() calls s.wg.Wait(), waiting for Ms to exit via <-m.stop. But closes happen after Run() in main—so it hangs forever! The panic interrupts, but it'd block otherwise.

Fixes in This Bit (Learning Focus):

Thrash: Add a "cooldown" per M after parking (skip grabbing for 200ms). This spaces out the loop (less spam) and gives time for other events (e.g., M0 parking P0 during block—M1 might grab P0 instead).
Panic/Race: Ditch the scanning unblock (racy). Use manual signal for G2 in main (known ID, no scan). Easy for toy; scales to a "blocked Gs" list later.
Hang: Run() now just prints "start" (Ms already running from AddM). main signals unblock, sleeps total time, closes stops, then wg.Wait(). Finite run!
Bonus Polish: Bump tick sleep to 100ms (less output). Add overflow to globalQ in Enqueue (demo stealing if you add more Gs). In blocking case, see M0 park P0, M1 grab it (empty, park with cooldown), unblock resumes G2 on M0 (re-grabs post-cooldown).
*/


/**
If it works, you should get an output similar to this:

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
Goroutine is done with task!
M0 on P0: Finished G0
    Work step 3
Goroutine is done with task!
M1 on P1: Finished G1
M1: Parking, handing off P1
M0 on P0: Starting G2
  G3 doing some work...
    Work step 1
M1: Grabbed available P1
M1: Parking, handing off P1
    Work step 2
    Work step 3
  G3: Entering block... (syscall sim)...
  G: Waiting for unblock signal...
Manual: Signaled unblock for G2
  G: Resumed after unblock!
Goroutine is done with task!
M0 on P0: Finished G2
M1: Grabbed available P1
M1: Parking, handing off P1
M0: Parking, handing off P0
M1: Grabbed available P1
M1: Parking, handing off P1
M0: Grabbed available P0
M0: Parking, handing off P0
M1: Grabbed available P1
M1: Parking, handing off P1
M0: Grabbed available P0
M0: Parking, handing off P0
M1: Grabbed available P1
M1: Parking, handing off P1
M0: Grabbed available P0
M0: Parking, handing off P0
M1: Grabbed available P1
M1: Parking, handing off P1
M0: Grabbed available P0
M0: Parking, handing off P0
M1: Grabbed available P1
M1: Parking, handing off P1
M0: Grabbed available P0
M0: Parking, handing off P0
=== Schedule Complete ===
*/