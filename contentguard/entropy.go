package contentguard

import (
	"math"
)

// entropyThreshold is the bits/byte threshold above which content is considered
// potentially encoded. Base64 typically has entropy around 5.0-5.5, so we use a
// lower threshold to catch it.
const entropyThreshold = 4.8

// shannonEntropy calculates the Shannon entropy of a byte slice in bits per byte.
// Higher entropy indicates more randomness/compression/encoding.
//
// Typical values:
//   - English text: 3.0 - 4.5 bits/byte
//   - Source code: 4.0 - 5.0 bits/byte
//   - Base64 encoded: 5.5 - 6.0 bits/byte
//   - Compressed/random: 7.5 - 8.0 bits/byte
func shannonEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0
	}

	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}

	length := float64(len(data))
	var entropy float64

	for _, count := range freq {
		if count == 0 {
			continue
		}
		p := float64(count) / length
		entropy -= p * math.Log2(p)
	}

	return entropy
}
