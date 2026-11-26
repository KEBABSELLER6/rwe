package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"fmt"
	"maps"
	"os"
	"regexp"
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

func Load(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	m := Manifest{}
	err = json.Unmarshal(raw, &m)
	if err != nil {
		return nil, fmt.Errorf("corrupt manifest %s: %w", path, err)
	}

	return m, nil
}

var Sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)
