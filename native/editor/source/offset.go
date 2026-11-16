package source

// OffsetInNormalizedSource maps a byte offset in on-disk Markdown to the
// corresponding byte offset after CRLF line endings have been normalized to
// LF by the editor. Offsets are clamped to the source bounds before mapping.
func OffsetInNormalizedSource(onDisk string, diskOffset int) int {
	diskOffset = max(0, min(diskOffset, len(onDisk)))
	return diskOffset - countCRLFPairs(onDisk[:diskOffset])
}

func countCRLFPairs(source string) int {
	count := 0
	for i := 1; i < len(source); i++ {
		if source[i-1] == '\r' && source[i] == '\n' {
			count++
			i++
		}
	}
	return count
}
