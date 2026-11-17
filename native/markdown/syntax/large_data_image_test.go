package syntax

import "testing"

func invalidBareDestinationByteBranch(c byte) bool {
	return c <= ' ' || c >= 0x7f || c == '(' || c == '\\'
}

func TestInvalidBareDestinationByteBitsetMatchesPredicate(t *testing.T) {
	for i := 0; i < 256; i++ {
		got, want := invalidBareDestinationByte(byte(i)), invalidBareDestinationByteBranch(byte(i))
		if got != want {
			t.Fatalf("byte %#02x: bitset=%v predicate=%v", i, got, want)
		}
	}
}

func TestParallelBase64DestinationValidationChecksEveryChunkBoundary(t *testing.T) {
	const valid = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	for _, workers := range []int{2, 3, 4} {
		if invalidBase64DestinationParallel(valid, workers) {
			t.Fatalf("parallel validator rejected valid payload with %d workers", workers)
		}
		positions := map[int]bool{0: true, len(valid) - 1: true}
		for worker := 1; worker < workers; worker++ {
			boundary := worker * len(valid) / workers
			for _, at := range []int{boundary - 1, boundary, boundary + 1} {
				if at >= 0 && at < len(valid) {
					positions[at] = true
				}
			}
		}
		for at := range positions {
			for _, bad := range []byte{'%', '\n', '\r', ' ', '(', '\\', 0x7f, 0x80} {
				payload := []byte(valid)
				payload[at] = bad
				if !invalidBase64DestinationParallel(string(payload), workers) {
					t.Fatalf("validator accepted byte %#02x at %d with %d workers", bad, at, workers)
				}
			}
		}
	}
}
