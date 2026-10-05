package tex

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// AlgorithmVersion is mixed into every cache key; bump it whenever any
// derivation/packing algorithm changes so stale results fully invalidate.
const AlgorithmVersion = 1

// entryMagic identifies cache entry files and guards against decoding
// foreign or truncated data.
const entryMagic = "TXC1"

// Cache stores derived artifacts on disk, keyed by content+config hash.
// Keys depend only on input content, relevant config and the algorithm
// version, so a config or algorithm change invalidates everything derived
// from it while unchanged artifacts are reused.
type Cache struct {
	dir string
}

// NewCache opens (and creates if needed) the cache directory.
func NewCache(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cache: create %s: %w", dir, err)
	}
	return &Cache{dir: dir}, nil
}

// HashImage computes the content hash of an image: geometry, metadata and
// the exact float bits of every pixel.
func HashImage(img *Image) string {
	h := sha256.New()
	var hdr [8]byte
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(img.Width))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(img.Height))
	h.Write(hdr[:])
	binary.LittleEndian.PutUint32(hdr[0:4], uint32(img.Channels))
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(img.Space))
	h.Write(hdr[:])
	if img.Dir.NormalYUp {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}
	if img.Dir.OriginTopLeft {
		h.Write([]byte{1})
	} else {
		h.Write([]byte{0})
	}
	buf := make([]byte, 4*len(img.Data))
	for k, v := range img.Data {
		binary.LittleEndian.PutUint32(buf[4*k:], math.Float32bits(v))
	}
	h.Write(buf)
	return hex.EncodeToString(h.Sum(nil))
}

// Key computes the cache key for an operation over content hashes and a
// canonical config encoding. The algorithm version is always mixed in.
func Key(op string, contentHashes []string, configCanonical string) string {
	h := sha256.New()
	fmt.Fprintf(h, "v%d|%s|%s|%s", AlgorithmVersion, op,
		strings.Join(contentHashes, ","), configCanonical)
	return hex.EncodeToString(h.Sum(nil))
}

// CacheEntry is a stored artifact plus integrity metadata.
type CacheEntry struct {
	Key  string
	Data []byte
}

func (c *Cache) path(key string) string { return filepath.Join(c.dir, key+".bin") }

// Get returns the entry for key, or nil if absent, corrupted or
// incompletely written. A corrupted entry is removed so later runs do
// not keep re-reading it.
func (c *Cache) Get(key string) *CacheEntry {
	raw, err := os.ReadFile(c.path(key))
	if err != nil {
		return nil
	}
	// Layout: magic(4) | keyLen(4) | key | checksum(32) | payload.
	if len(raw) < 4+4 {
		c.remove(key)
		return nil
	}
	if string(raw[:4]) != entryMagic {
		c.remove(key)
		return nil
	}
	keyLen := int(binary.LittleEndian.Uint32(raw[4:8]))
	if keyLen <= 0 || len(raw) < 8+keyLen+32 {
		c.remove(key)
		return nil
	}
	storedKey := string(raw[8 : 8+keyLen])
	sum := raw[8+keyLen : 8+keyLen+32]
	payload := raw[8+keyLen+32:]
	got := sha256.Sum256(payload)
	if storedKey != key || !equalBytes(sum, got[:]) {
		c.remove(key)
		return nil
	}
	return &CacheEntry{Key: key, Data: payload}
}

// Put stores data under key atomically: the payload is written to a
// temporary file first and renamed into place, so a crash or concurrent
// reader never observes a partial entry.
func (c *Cache) Put(key string, data []byte) error {
	sum := sha256.Sum256(data)
	var hdr [8]byte
	copy(hdr[0:4], entryMagic)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(key)))
	tmp, err := os.CreateTemp(c.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("cache: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed
	for _, part := range [][]byte{hdr[:], []byte(key), sum[:], data} {
		if _, err := tmp.Write(part); err != nil {
			tmp.Close()
			return fmt.Errorf("cache: write temp: %w", err)
		}
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("cache: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cache: close temp: %w", err)
	}
	if err := os.Rename(tmpName, c.path(key)); err != nil {
		return fmt.Errorf("cache: rename: %w", err)
	}
	return nil
}

func (c *Cache) remove(key string) { os.Remove(c.path(key)) }

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
