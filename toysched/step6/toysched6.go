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
		time.Sleep(10 * time.Millisecond)
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
	}

	s.Ms = append(s.Ms, m)
	s.wg.Add(1)
	go m.run(s)

	return m
}

// Enqueue adds a G to a P's run queue (FIFO).
func (s *Scheduler) Enqueue(p *P, g *G) {
if p.NumG > 10 { 
		s.mu.Lock()
		s.globalQ = append(s.globalQ, g)
		s.mu.Unlock()
		fmt.Printf("Enqueued G%d to globalQ (P%d full)\n", g.ID, p.ID)
		return
	}
	p.RunQ = append(p.RunQ, g)
	p.NumG++
	g.Status = "runnable"
}

// Like old Schedule, but async + steal
func (m *M) scheduleOnce(s *Scheduler) {
	
	if m.P == nil {
		// No P: Non-block grab from central pool
		select {
		case p := <-s.availPs:
			m.P = p
			fmt.Printf("M%d: Grabbed available P%d\n", m.ID, m.P.ID)
		default:
			// No available P: Stay idle
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
			// No work: Handoff P to central pool (park M).
			fmt.Printf("M%d: Parking, handing off P%d\n", m.ID, m.P.ID)
			s.availPs <- m.P
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
		// Still blocked? Handoff P (G stays on M, waiting).
		fmt.Printf("M%d on P%d: G%d blocked, handing off P\n", m.ID, m.P.ID, g.ID)
		s.availPs <- m.P
		m.P = nil
		// M stays with G, will resume when unblocked.
		return
	}
}

//Starts Ms, waits for wg, signals unblocks.
func (s *Scheduler) Run() {
	fmt.Println("=== Starting Toy Schedule ===")

	go func() {
			// Simulate async unblock after 500ms.
			time.Sleep(500 * time.Millisecond)
			s.mu.Lock()
			// Find any blocked G on running Ms.
			for _, m := range s.Ms {
				if m.G != nil && m.G.blockChan != nil {
					m.G.blockChan <- struct{}{}
					fmt.Println("Scheduler: Signaled unblock for G", m.G.ID)
					break // One at a time.
				}
			}
			s.mu.Unlock()
		}()

		 // Wait for all Ms to stop (but we don't stop yet).
	s.wg.Wait()
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
	}

	g0 := sched.NewG(sampleWork, false)
	g1 := sched.NewG(sampleWork2, false)
	// Manual for blockChan.
	g2 := sched.NewG(sampleWork3, true)

	sched.Enqueue(p0, g0)
	sched.Enqueue(p1, g1)
	sched.Enqueue(p0, g2)

	sched.Run()

	// Stop Ms after a bit.
	time.Sleep(1 * time.Second)
	for _, m := range sched.Ms {
		close(m.stop)
	}
}



/**
What went wrong and what is the fix

Mutex Panic ("unlock of unlocked mutex"):

In scheduleOnce()'s if m.P == nil block: We s.mu.Lock() once, then inside the for loop (if a P is grabbed), we s.mu.Unlock() and break. But then the code falls through to the second s.mu.Unlock() outside the loop—boom, unlocking an already-unlocked mutex.
If no P grabbed, it unlocks once (fine), but the inner unlock never fires.


Unending Loop (when commenting unlocks):

After G0/G1 finish, M1's P1 empties. M1 enters if m.P.NumG == 0, steals nothing (globalQ empty), prints "Parking, handing off P1", sends to p1.handoff (buffered, so non-blocking), sets m.P = nil, returns.
Next tick (~10ms): M1 sees m.P == nil, locks mu, loops Ps, sees len(p1.handoff) > 0 (the P we just sent is buffered there), grabs it (<-handoff empties chan), unlocks, prints "Grabbed handed-off P1".
Continues to if m.P.NumG == 0 again (still empty), hands off immediately (refills buffer), sets nil, returns.
Repeat forever: Grab → handoff → grab... The 10ms sleep makes it a tight loop of spam. (G2 starts on M0, but M1 is thrashing P1 independently.)
Bonus: Dupe prints (e.g., "Work step 1" twice) are just concurrent output from G0/G1—harmless interleaving.


Bonus Issue: Unblock Never Fires:

Our unblock goroutine scans p.RunQ (queued Gs), but blocked G2 is dequeued and bound to M0 (m.G = g2, status="running"). So it never finds/signals g2.blockChan, G2 waits forever, M0 stays blocked, wg.Wait() hangs.
In log: Reaches "Waiting for unblock signal..." but no resume.


Other Nits:

idleMs declared but unused (leftover).
In main: g2 := sched.NewG(sampleWork3, true) sets ID=2 (after g0=0, g1=1), then sched.nextGID = 3—ok, but unnecessary.
bindIdleM defined but unused.
No global enqueue (all local), so no stealing demo yet.
After sched.Run(), time.Sleep(2s) then close stops—but if blocked, wg hangs, program exits uncleanly.



The Fixes (Layered for Learning)

Mutex: Use defer s.mu.Unlock() after Lock()—unlocks exactly once, even on early return/break. Restructure loop to avoid inner unlock.
Loop: Per-P handoff chans + buffer cause the ping-pong. Switch to a central availPs chan *P in Scheduler (buffered, cap=num Ps).

Park: s.availPs <- m.P; m.P = nil (non-block send).
Grab: select { case p := <-s.availPs: m.P = p; ... } default: { return } (non-block receive; idle if none).
This prevents self-grab: Send to central pool, only other/idle Ms compete for it. No more thrash!


Unblock: Scan s.Ms for m.G.blockChan != nil (running/blocked Gs), not queues. Lock mu for safety (racy access to Ms slice/G).
Polish: Remove unused idleMs/ bindIdleM/ done. Add global enqueue option. In main, after Run(), wait wg explicitly? No—Run() already wg.Wait(), but add timeout or ensure unblock. Increase sleep in run() to 50ms for less spam. Make unblock find any blocked G.
Realism: This now better mimics Go's "P list" (availPs) and "Ms steal Ps" without per-P mess.

Run the updated code below—it should finish cleanly: G0/G1 concurrent, G2 blocks ~500ms (M0 parks P0 to pool, M1 may grab/park P1 once but idles after), unblock signals, G2 resumes on M0, all done, "Schedule Complete", then stops Ms.
*/