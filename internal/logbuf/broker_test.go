package logbuf

import (
	"fmt"
	"strings"
	"testing"
)

func TestWriteSplitsLinesAndAssignsSequence(t *testing.T) {
	b := New(10, 1)
	if _, err := b.Write([]byte("first\nsecond\nthi")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("rd\n")); err != nil {
		t.Fatal(err)
	}

	got, dropped := b.Replay(0)
	if dropped != 0 {
		t.Fatalf("dropped = %d", dropped)
	}
	if len(got) != 3 {
		t.Fatalf("records = %d, want 3: %#v", len(got), got)
	}
	lines := []string{got[0].Line, got[1].Line, got[2].Line}
	if strings.Join(lines, ",") != "first,second,third" {
		t.Fatalf("lines = %v", lines)
	}
	if got[0].Seq != 1 || got[2].Seq != 3 {
		t.Fatalf("sequence = %d..%d", got[0].Seq, got[2].Seq)
	}
}

func TestRingBufferDropsOldestRecords(t *testing.T) {
	b := New(3, 1)
	for i := 0; i < 5; i++ {
		b.Write([]byte(fmt.Sprintf("line-%d\n", i)))
	}
	got, _ := b.Replay(0)
	if len(got) != 3 {
		t.Fatalf("records = %d, want 3", len(got))
	}
	if got[0].Line != "line-2" || got[2].Line != "line-4" {
		t.Fatalf("unexpected records: %#v", got)
	}
}

func TestReplayReportsGapForExpiredLastEventID(t *testing.T) {
	b := New(3, 1)
	for i := 0; i < 10; i++ {
		b.Write([]byte(fmt.Sprintf("line-%d\n", i)))
	}
	got, dropped := b.Replay(2)
	if dropped != 5 {
		t.Fatalf("dropped = %d, want 5", dropped)
	}
	if len(got) != 3 || got[0].Seq != 8 {
		t.Fatalf("records = %#v", got)
	}
}

func TestReplayAfterLatestReturnsNothing(t *testing.T) {
	b := New(3, 1)
	b.Write([]byte("one\n"))
	got, dropped := b.Replay(1)
	if len(got) != 0 || dropped != 0 {
		t.Fatalf("got=%#v dropped=%d", got, dropped)
	}
}

func TestSlowSubscriberIsRemovedWithoutBlocking(t *testing.T) {
	b := New(10, 1)
	ch, cancel, ok := b.Subscribe()
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer cancel()

	for i := 0; i < 300; i++ {
		if _, err := b.Write([]byte(fmt.Sprintf("line-%d\n", i))); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	b.mu.Lock()
	subs := len(b.subs)
	b.mu.Unlock()
	if subs != 0 {
		t.Fatalf("slow subscriber still registered: %d", subs)
	}
	select {
	case _, open := <-ch:
		if open {
			// Buffered records may still be readable; drain them.
			for range ch {
			}
		}
	default:
	}
}

func TestSubscribeRejectsBeyondLimit(t *testing.T) {
	b := New(10, 1)
	_, cancel, ok := b.Subscribe()
	if !ok {
		t.Fatal("first subscribe failed")
	}
	defer cancel()
	if _, _, ok := b.Subscribe(); ok {
		t.Fatal("second subscribe must be rejected")
	}
}

func TestLastSeqTracksNewestRecord(t *testing.T) {
	b := New(2, 1)
	if got := b.LastSeq(); got != 0 {
		t.Fatalf("empty LastSeq = %d, want 0", got)
	}
	_, _ = b.Write([]byte("one\ntwo\nthree\n"))
	if got := b.LastSeq(); got != 3 {
		t.Fatalf("LastSeq = %d, want 3", got)
	}
}
