package hardware

import "testing"

const (
	gib      = uint64(1) << 30
	phiFile  = int64(2491874272)
	phiKV    = int64(131072) // 128 KiB per token (32 layers x 8 KV heads x 128 x 2 x 2)
	qwenFile = int64(1117320736)
	qwenKV   = int64(28672)
	bigKV    = int64(57344)
)

func TestTheContextCacheIsCountedOnlyBeyondTheBaseline(t *testing.T) {
	base := MinRAMBytes(phiFile)
	for tokens, want := range map[int]uint64{
		0:     base, // nothing asked: the rule as it was
		2048:  base, // a smaller context does not give memory back
		4096:  base, // exactly the baseline the rule's quarter allows for
		4097:  base + uint64(phiKV),
		8192:  base + uint64(phiKV)*4096,
		16384: base + uint64(phiKV)*(16384-4096),
	} {
		if got := RequiredBytes(phiFile, phiKV, tokens); got != want {
			t.Errorf("tokens %d: %d, want %d", tokens, got, want)
		}
	}
	// the real numbers the design quotes: Phi's cache is 0.5 GiB at 4K, 2 GiB at 16K, 4 GiB at 32K and 16 GiB at 128K
	for tokens, cacheGiB := range map[int]uint64{16384: 2, 32768: 4, 131072: 16} {
		extra := RequiredBytes(phiFile, phiKV, tokens) - RequiredBytes(phiFile, phiKV, BaselineContextTokens)
		if want := cacheGiB*gib - uint64(phiKV)*BaselineContextTokens; extra != want {
			t.Errorf("Phi at %d tokens adds %d bytes beyond the baseline, want %d", tokens, extra, want)
		}
	}
	// an unknown cache size is not counted, and not guessed
	if RequiredBytes(phiFile, 0, 131072) != base || RequiredBytes(phiFile, -5, 131072) != base {
		t.Error("an unknown cache size must not change the estimate")
	}
	if RequiredBytes(0, phiKV, 131072) != uint64(phiKV)*(131072-4096) {
		t.Error("a missing file size gives no base, only the cache")
	}
}

func TestTheLargestContextThatFitsIsTheLargestOfferedSize(t *testing.T) {
	sizes := []int{2048, 4096, 8192, 16384, 32768, 65536, 131072}
	for _, c := range []struct {
		name  string
		file  int64
		kv    int64
		ram   uint64
		want  int
		known bool
	}{
		{"phi on a 16 GiB machine", phiFile, phiKV, 16 * gib, 65536, true},
		{"phi on an 8 GiB machine", phiFile, phiKV, 8 * gib, 16384, true},
		{"phi on 6 GiB: barely the baseline", phiFile, phiKV, 6 * gib, 4096, true},
		{"phi on 5 GiB: not even the model", phiFile, phiKV, 5 * gib, 0, true},
		{"qwen 1.5B on 8 GiB: everything", qwenFile, qwenKV, 8 * gib, 131072, true},
		{"unknown memory", phiFile, phiKV, 0, 0, false},
		{"unknown cache size", phiFile, 0, 16 * gib, 0, false},
	} {
		got, known := LargestContext(c.file, c.kv, c.ram, sizes)
		if got != c.want || known != c.known {
			t.Errorf("%s: %d known %v, want %d known %v", c.name, got, known, c.want, c.known)
		}
	}
	// exactly the memory a size needs fits; one byte less does not
	need := RequiredBytes(phiFile, phiKV, 16384)
	if got, _ := LargestContext(phiFile, phiKV, need, sizes); got != 16384 {
		t.Errorf("exactly enough must fit: %d", got)
	}
	if got, _ := LargestContext(phiFile, phiKV, need-1, sizes); got != 8192 {
		t.Errorf("one byte less must not: %d", got)
	}
}

func TestSizeAndNumberWriting(t *testing.T) {
	for n, want := range map[int]string{0: "0", 7: "7", 999: "999", 1000: "1,000", 4096: "4,096", 16384: "16,384", 131072: "131,072", 1234567: "1,234,567", -4096: "-4,096", -999: "-999"} {
		if got := Thousands(n); got != want {
			t.Errorf("Thousands(%d) = %q, want %q", n, got, want)
		}
	}
	for bytes, want := range map[uint64]string{0: "0.0 GiB", 1 << 30: "1.0 GiB", 6336068312: "5.9 GiB", 16 << 30: "16.0 GiB"} {
		if got := GiB(bytes); got != want {
			t.Errorf("GiB(%d) = %q, want %q", bytes, got, want)
		}
	}
}
