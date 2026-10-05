package tex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheRoundTrip(t *testing.T) {
	c, err := NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := Key("op", []string{"hashA"}, "cfg=1")
	if c.Get(key) != nil {
		t.Fatal("unexpected hit on empty cache")
	}
	if err := c.Put(key, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	e := c.Get(key)
	if e == nil || string(e.Data) != "payload" {
		t.Fatalf("roundtrip failed: %+v", e)
	}
}

func TestCacheKeyDependsOnContentConfigAndVersion(t *testing.T) {
	base := Key("op", []string{"a"}, "cfg")
	if Key("op", []string{"b"}, "cfg") == base {
		t.Fatal("key ignores content hash")
	}
	if Key("op", []string{"a"}, "cfg2") == base {
		t.Fatal("key ignores config")
	}
	if Key("op2", []string{"a"}, "cfg") == base {
		t.Fatal("key ignores operation")
	}
	// Keys are deterministic: identical inputs give identical keys.
	if Key("op", []string{"a"}, "cfg") != base {
		t.Fatal("key not deterministic")
	}
}

func TestCacheRejectsCorruptedEntries(t *testing.T) {
	dir := t.TempDir()
	c, err := NewCache(dir)
	if err != nil {
		t.Fatal(err)
	}
	key := Key("op", []string{"a"}, "cfg")
	if err := c.Put(key, []byte("payload")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, key+".bin")

	corrupt := func(mutate func([]byte) []byte) {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, mutate(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		if c.Get(key) != nil {
			t.Fatal("corrupted entry was returned as valid")
		}
		// Corrupted entry must be evicted from disk.
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("corrupted entry was not evicted")
		}
		if err := c.Put(key, []byte("payload")); err != nil {
			t.Fatal(err)
		}
	}
	corrupt(func(b []byte) []byte { b[len(b)-1] ^= 0xff; return b }) // bit flip in payload
	corrupt(func(b []byte) []byte { return b[:len(b)/2] })           // truncated write
	corrupt(func(b []byte) []byte { copy(b, "XXXX"); return b })     // bad magic
}

func TestHashImageStableAndSensitive(t *testing.T) {
	a := NewImage(4, 4, 1, Linear, Direction{OriginTopLeft: true})
	a.Data[3] = 0.5
	b := a.Clone()
	if HashImage(a) != HashImage(b) {
		t.Fatal("same content hashed differently")
	}
	b.Data[3] = 0.500001
	if HashImage(a) == HashImage(b) {
		t.Fatal("pixel change not reflected in hash")
	}
	c := a.Clone()
	c.Dir.NormalYUp = true
	if HashImage(a) == HashImage(c) {
		t.Fatal("metadata change not reflected in hash")
	}
}
