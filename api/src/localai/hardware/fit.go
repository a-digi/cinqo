package hardware

import (
	"fmt"
	"strconv"
)

// Our own rule of thumb for "does this model fit this machine", not a figure from a model's authors: the model file
// plus a quarter for the context cache and the runtime, plus a fixed reserve for the operating system and Cinqo itself.
// It is meant to be measured on real runs (the spike and the live proof of the plan) and adjusted then.
const (
	overheadNumerator   = 5 // the file size times 5/4
	overheadDenominator = 4
	reservedBytes       = 3 << 30 // 3 GiB left for the system and the app
)

// MinRAMBytes is the total memory a machine should have to run a model file of this size.
func MinRAMBytes(modelSizeBytes int64) uint64 {
	if modelSizeBytes <= 0 {
		return 0
	}
	size := uint64(modelSizeBytes)
	return size*overheadNumerator/overheadDenominator + reservedBytes
}

// Fits reports whether the machine has the memory for a model file of this size: nil when the memory is unknown.
func (i Info) Fits(modelSizeBytes int64) *bool {
	if i.RAMBytes == 0 {
		return nil
	}
	fits := i.RAMBytes >= MinRAMBytes(modelSizeBytes)
	return &fits
}

// BaselineContextTokens is the context the fit rule's quarter already allows for (the runtime and a cache of this many tokens).
// A larger context adds its own cache on top; a smaller one does not give memory back (the rule stays on the safe side).
const BaselineContextTokens = 4096

// RequiredBytes is the memory a machine should have to run a model file of this size with a context of tokens: the fit rule
// plus the cache of every token beyond the baseline. kvBytesPerToken is what the model's cache holds per token (its layers x
// KV heads x head size x 2 x 2 bytes, f16); 0 or less means unknown and the cache is not counted.
func RequiredBytes(modelSizeBytes, kvBytesPerToken int64, tokens int) uint64 {
	required := MinRAMBytes(modelSizeBytes)
	if kvBytesPerToken > 0 && tokens > BaselineContextTokens {
		required += uint64(kvBytesPerToken) * uint64(tokens-BaselineContextTokens)
	}
	return required
}

// LargestContext is the largest of candidates (in ascending order) the machine has the memory for. known is false when the
// machine's memory or the model's cache size is not known: nothing can be said then, and nothing is refused.
func LargestContext(modelSizeBytes, kvBytesPerToken int64, ramBytes uint64, candidates []int) (largest int, known bool) {
	if ramBytes == 0 || kvBytesPerToken <= 0 {
		return 0, false
	}
	for _, c := range candidates {
		if RequiredBytes(modelSizeBytes, kvBytesPerToken, c) <= ramBytes {
			largest = c
		}
	}
	return largest, true
}

// GiB writes a size in bytes the way the pages do: "5.9 GiB".
func GiB(bytes uint64) string { return fmt.Sprintf("%.1f GiB", float64(bytes)/(1<<30)) }

// Thousands writes a number with thousands separators: 16384 becomes "16,384".
func Thousands(n int) string {
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return sign + s
}
