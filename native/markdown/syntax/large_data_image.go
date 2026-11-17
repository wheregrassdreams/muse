package syntax

import (
	"encoding/base64"
	"runtime"
	"strings"
	"sync"
)

// LargeDataImageThreshold is the minimum destination size for the opaque
// Base64 image projection used by the editor. Smaller images go through the
// normal incremental Markdown parser.
const LargeDataImageThreshold = 64 << 10

const maxBase64ValidationScratch = 45 << 20

var strictBase64 = base64.StdEncoding.Strict()

// LargeDataImageInlines recognizes a standalone paragraph containing one
// large Base64 image. Returned text and destination strings are source slices;
// callers must keep src alive for as long as they retain the tokens.
func LargeDataImageInlines(src string, start, end int) ([]IncrementalInline, bool) {
	if start < 0 || end > len(src) || end <= start || end-start <= LargeDataImageThreshold {
		return nil, false
	}
	return largeDataImageInlineRange(src, start, end, false)
}

// LargeDataImageOnlySource recognizes a whole document containing only one
// standalone large Base64 image and an optional final line ending. The strict
// validation path is used during source initialization so an invalid payload
// always falls back to the normal parser.
func LargeDataImageOnlySource(src string) ([]IncrementalInline, bool) {
	if len(src) <= LargeDataImageThreshold || !strings.HasPrefix(src, "![") {
		return nil, false
	}
	end := len(src)
	if end > 0 && src[end-1] == '\n' {
		end--
		if end > 0 && src[end-1] == '\r' {
			end--
		}
	}
	if end <= 0 {
		return nil, false
	}
	inlines, ok := largeDataImageInlineRange(src, 0, end, true)
	if !ok || len(inlines) == 0 || inlines[0].Start != 0 || inlines[0].End != end {
		return nil, false
	}
	for _, inline := range inlines[1:] {
		if inline.Kind != IncrementalText {
			continue
		}
		for i := 0; i < len(inline.Text); i++ {
			if inline.Text[i] >= 0x80 {
				return nil, false
			}
		}
	}
	return inlines, true
}

// IsLargeDataImageDestination reports whether dest is a sufficiently large
// image data URL with a Base64 header. It deliberately does not decode bytes.
func IsLargeDataImageDestination(dest string) bool {
	const prefix = "data:image/"
	if len(dest) <= LargeDataImageThreshold || len(dest) < len(prefix) ||
		!strings.EqualFold(dest[:len(prefix)], prefix) {
		return false
	}
	headerEnd := strings.IndexByte(dest[:min(len(dest), 256)], ',')
	if headerEnd < 0 {
		return false
	}
	return strings.Contains(strings.ToLower(dest[:headerEnd]), ";base64")
}

func largeDataImageInlineRange(src string, start, end int, decodePayload bool) ([]IncrementalInline, bool) {
	line := src[start:end]
	lead := 0
	for lead < len(line) && (line[lead] == ' ' || line[lead] == '\t') {
		lead++
	}
	if !strings.HasPrefix(line[lead:], "![") {
		return nil, false
	}
	labelRel := strings.Index(line[lead+2:], "](")
	if labelRel < 0 {
		return nil, false
	}
	labelEnd := lead + 2 + labelRel
	alt := line[lead+2 : labelEnd]
	if strings.ContainsAny(alt, "\r\n") || strings.Contains(alt, "%%") ||
		alt != "" && strings.IndexAny(alt, "[]\\`*_~&<>!|") >= 0 {
		return nil, false
	}
	imageStart := start + lead
	destStart := labelEnd + 2
	if len(line)-destStart < len("data:image/") || !strings.EqualFold(line[destStart:destStart+len("data:image/")], "data:image/") {
		return nil, false
	}
	commaRel := strings.IndexByte(line[destStart:], ',')
	if commaRel < 0 || commaRel > 256 {
		return nil, false
	}
	comma := destStart + commaRel
	if !strings.Contains(strings.ToLower(line[destStart:comma]), ";base64") {
		return nil, false
	}
	for i := destStart; i < comma; i++ {
		if invalidBareDestinationByte(line[i]) || line[i] == ')' ||
			line[i] == '%' && i+1 < comma && line[i+1] == '%' {
			return nil, false
		}
	}
	closeRel := strings.IndexByte(line[comma+1:], ')')
	if closeRel < 0 {
		return nil, false
	}
	close := comma + 1 + closeRel
	if close-destStart <= LargeDataImageThreshold || close+1 != len(line) {
		return nil, false
	}
	// In a bare Markdown destination these bytes change how the parser finds
	// its closing delimiter. Base64 payloads contain none of them.
	payload := line[comma+1 : close]
	decodedLen := base64.StdEncoding.DecodedLen(len(payload))
	if decodePayload && decodedLen <= maxBase64ValidationScratch {
		// Go's Base64 decoder ignores line breaks. They change Markdown
		// destination parsing, and %% starts an Obsidian comment.
		if strings.ContainsAny(payload, "\r\n%") {
			return nil, false
		}
		if _, err := strictBase64.DecodeString(payload); err != nil {
			return nil, false
		}
	} else if invalidBase64DestinationPayload(payload) {
		return nil, false
	}
	dest := line[destStart:close]
	if !IsLargeDataImageDestination(dest) {
		return nil, false
	}
	imageEnd := start + close + 1
	altStart := imageStart + 2
	altEnd := altStart + len(alt)
	parent := IncrementalInline{Kind: IncrementalImage, Dest: dest, Start: imageStart, End: imageEnd, Next: 1}
	if alt != "" {
		parent.Next = 2
		return []IncrementalInline{
			parent,
			{Kind: IncrementalText, Text: alt, Start: altStart, End: altEnd, Next: 2},
		}, true
	}
	return []IncrementalInline{parent}, true
}

func invalidBase64DestinationByte(c byte) bool {
	return c == '%' || invalidBareDestinationByte(c)
}

const parallelBase64ValidationThreshold = 32 << 20
const base64ValidationProbeBytes = 64 << 10

func invalidBase64DestinationPayload(payload string) bool {
	if len(payload) < parallelBase64ValidationThreshold {
		return invalidBase64DestinationBytes(payload)
	}
	if invalidBase64DestinationBytes(payload[:min(len(payload), base64ValidationProbeBytes)]) {
		return true
	}
	workers := min(4, runtime.GOMAXPROCS(0))
	if workers < 2 {
		return invalidBase64DestinationBytes(payload)
	}
	return invalidBase64DestinationParallel(payload, workers)
}

func invalidBase64DestinationParallel(payload string, workers int) bool {
	if workers < 2 || workers > 4 {
		return invalidBase64DestinationBytes(payload)
	}
	var invalid [4]bool
	var wait sync.WaitGroup
	for i := 0; i < workers; i++ {
		from, to := i*len(payload)/workers, (i+1)*len(payload)/workers
		wait.Add(1)
		go func(i, from, to int) {
			defer wait.Done()
			invalid[i] = invalidBase64DestinationBytes(payload[from:to])
		}(i, from, to)
	}
	wait.Wait()
	for i := 0; i < workers; i++ {
		if invalid[i] {
			return true
		}
	}
	return false
}

func invalidBase64DestinationBytes(payload string) bool {
	i := 0
	for ; i+8 <= len(payload); i += 8 {
		if invalidBase64DestinationByte(payload[i]) || invalidBase64DestinationByte(payload[i+1]) ||
			invalidBase64DestinationByte(payload[i+2]) || invalidBase64DestinationByte(payload[i+3]) ||
			invalidBase64DestinationByte(payload[i+4]) || invalidBase64DestinationByte(payload[i+5]) ||
			invalidBase64DestinationByte(payload[i+6]) || invalidBase64DestinationByte(payload[i+7]) {
			return true
		}
	}
	for ; i < len(payload); i++ {
		if invalidBase64DestinationByte(payload[i]) {
			return true
		}
	}
	return false
}

// invalidBareDestinationMasks marks bytes that can change Markdown's bare
// destination parsing. The lookup keeps the large Base64 validation loop
// branch-light while preserving the exact byte predicate.
var invalidBareDestinationMasks = [...]uint64{
	0x101ffffffff,      // NUL through space, and '('
	0x8000000010000000, // '\\' and DEL
	^uint64(0),
	^uint64(0),
}

func invalidBareDestinationByte(c byte) bool {
	return invalidBareDestinationMasks[c>>6]&(uint64(1)<<(c&63)) != 0
}
