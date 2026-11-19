package shell

import "testing"

func TestResultPumpDrainsAtMostTheRequestedCount(t *testing.T) {
	results := make(chan int, 4)
	for range 4 {
		results <- 7
	}
	pump := NewResultPump((<-chan int)(results))
	seen := 0
	drained, open := pump.Drain(2, func(int) { seen++ })
	if drained != 2 || seen != 2 || !open {
		t.Fatalf("first drain = %d results, seen=%d, open=%v", drained, seen, open)
	}
	drained, open = pump.Drain(8, func(int) { seen++ })
	if drained != 2 || seen != 4 || !open {
		t.Fatalf("second drain = %d results, seen=%d, open=%v", drained, seen, open)
	}
}

func TestResultPumpReportsClosedResultChannel(t *testing.T) {
	results := make(chan int)
	close(results)
	pump := NewResultPump((<-chan int)(results))
	if drained, open := pump.Drain(1, nil); drained != 0 || open {
		t.Fatalf("closed drain = %d, open=%v", drained, open)
	}
}
