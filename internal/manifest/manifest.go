package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"maps"
	"slices"
)

type Manifest map[string]string

func (m Manifest) Hash() string {
	h := sha256.New()
	for _, p := range slices.Sorted(maps.Keys(m)) {
		h.Write([]byte(p))
		h.Write([]byte{0})
		h.Write([]byte(m[p]))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
