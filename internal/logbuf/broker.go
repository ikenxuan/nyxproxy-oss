package logbuf

import (
	"bytes"
	"sync"
	"time"
)

type Record struct {
	Seq  uint64    `json:"seq"`
	TS   time.Time `json:"ts"`
	Line string    `json:"line"`
}

type Broker struct {
	mu      sync.Mutex
	ring    []Record
	max     int
	nextSeq uint64
	partial []byte
	subs    map[chan Record]struct{}
	maxSubs int
	closed  bool
}

func New(size, maxSubs int) *Broker {
	if size <= 0 {
		size = 2000
	}
	if maxSubs <= 0 {
		maxSubs = 8
	}
	return &Broker{
		ring:    make([]Record, 0, size),
		max:     size,
		subs:    make(map[chan Record]struct{}),
		maxSubs: maxSubs,
	}
}

func (b *Broker) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return len(p), nil
	}

	b.partial = append(b.partial, p...)
	for {
		idx := bytes.IndexByte(b.partial, '\n')
		if idx < 0 {
			break
		}
		line := string(b.partial[:idx])
		b.partial = b.partial[idx+1:]
		b.appendLocked(line)
	}
	if len(b.partial) > 64*1024 {
		b.appendLocked(string(b.partial))
		b.partial = nil
	}
	return len(p), nil
}

func (b *Broker) appendLocked(line string) {
	b.nextSeq++
	record := Record{Seq: b.nextSeq, TS: time.Now(), Line: line}
	if len(b.ring) < b.max {
		b.ring = append(b.ring, record)
	} else {
		copy(b.ring, b.ring[1:])
		b.ring[len(b.ring)-1] = record
	}
	for ch := range b.subs {
		select {
		case ch <- record:
		default:
			delete(b.subs, ch)
			close(ch)
		}
	}
}

func (b *Broker) Subscribe() (<-chan Record, func(), bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || len(b.subs) >= b.maxSubs {
		return nil, nil, false
	}
	ch := make(chan Record, 256)
	b.subs[ch] = struct{}{}
	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
	return ch, cancel, true
}

// Replay returns records newer than after. When after falls outside the ring,
// dropped reports how many sequence numbers were lost.
func (b *Broker) Replay(after uint64) ([]Record, uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.ring) == 0 {
		return nil, 0
	}
	last := b.ring[len(b.ring)-1].Seq
	if after >= last {
		return nil, 0
	}
	start := 0
	for start < len(b.ring) && b.ring[start].Seq <= after {
		start++
	}
	var dropped uint64
	if after != 0 && b.ring[0].Seq > after+1 {
		dropped = b.ring[0].Seq - after - 1
	}
	out := make([]Record, len(b.ring)-start)
	copy(out, b.ring[start:])
	return out, dropped
}

// LastSeq returns the newest sequence number, or 0 when nothing was logged.
func (b *Broker) LastSeq() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.ring) == 0 {
		return 0
	}
	return b.ring[len(b.ring)-1].Seq
}

func (b *Broker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}
