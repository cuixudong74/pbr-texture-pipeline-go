package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AlgorithmVersion is mixed into every cache key; bump it whenever any
// processing algorithm changes so stale results are invalidated wholesale.
const AlgorithmVersion = 1

// Cache stores processed artifacts on disk, keyed only by input content and
// relevant configuration. Reads verify integrity; writes are atomic, so a
// crashed or cancelled run never leaves a partial artifact visible.
type Cache struct {
	Dir string
}

// NewCache creates a cache rooted at dir, creating it if needed.
func NewCache(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	return &Cache{Dir: dir}, nil
}

// Key computes the cache key for a named operation over a config and the
// content hashes of the inputs. The key depends only on input content, the
// relevant configuration, and the algorithm version.
func Key(op string, cfg any, inputs ...[32]byte) [32]byte {
	h := sha256.New()
	fmt.Fprintf(h, "v%d\n%s\n", AlgorithmVersion, op)
	cfgJSON, err := json.Marshal(cfg) // struct field order is fixed; map keys sorted
	if err != nil {
		fmt.Fprintf(h, "cfg-error:%v\n", err)
	} else {
		h.Write(cfgJSON)
	}
	h.Write([]byte("\n"))
	for _, in := range inputs {
		h.Write(in[:])
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

func (c *Cache) path(key [32]byte) string {
	return filepath.Join(c.Dir, hex.EncodeToString(key[:])+".bin")
}

// Get returns the cached payload for key, or nil if absent or corrupt.
// A corrupt entry is removed so it is never served as a valid result.
func (c *Cache) Get(key [32]byte) []byte {
	b, err := os.ReadFile(c.path(key))
	if err != nil {
		return nil
	}
	if len(b) < sha256.Size {
		os.Remove(c.path(key))
		return nil
	}
	want := b[:sha256.Size]
	payload := b[sha256.Size:]
	if sha256.Sum256(payload) != [32]byte(want) {
		os.Remove(c.path(key))
		return nil
	}
	return payload
}

// Put stores payload under key atomically: the data is written to a temp
// file in the same directory and renamed into place, so concurrent or
// interrupted writers can never expose a partially written entry.
func (c *Cache) Put(key [32]byte, payload []byte) error {
	sum := sha256.Sum256(payload)
	buf := make([]byte, 0, sha256.Size+len(payload))
	buf = append(buf, sum[:]...)
	buf = append(buf, payload...)
	tmp, err := os.CreateTemp(c.Dir, "tmp-*")
	if err != nil {
		return fmt.Errorf("cache put: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(buf); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("cache put: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("cache put: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cache put: %w", err)
	}
	if err := os.Rename(tmpName, c.path(key)); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("cache put: %w", err)
	}
	return nil
}
