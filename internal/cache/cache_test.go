package cache

import "testing"

func keyCache(scope Scope) *Cache {
	return &Cache{scope: scope, ttl: 3600, maxBytes: 1 << 20, enabled: true}
}

func TestKeyStableAcrossFieldOrderAndVolatileFields(t *testing.T) {
	c := keyCache(ScopeGlobal)

	// Same semantic request: reordered keys + different volatile fields (stream, user).
	a := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"stream":false,"user":"alice"}`)
	b := []byte(`{"messages":[{"role":"user","content":"hi"}],"user":"bob","stream":true,"model":"gpt-4o"}`)

	ka, ok := c.Key("", "openai", a)
	if !ok {
		t.Fatal("Key(a) not ok")
	}
	kb, _ := c.Key("", "openai", b)
	if ka != kb {
		t.Errorf("keys should match after normalization:\n a=%s\n b=%s", ka, kb)
	}
}

func TestKeyDiffersOnOutputAffectingFields(t *testing.T) {
	c := keyCache(ScopeGlobal)
	base := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"temperature":0.2}`)
	hot := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"temperature":0.9}`)

	kb, _ := c.Key("", "openai", base)
	kh, _ := c.Key("", "openai", hot)
	if kb == kh {
		t.Error("different temperature must produce different keys")
	}
}

func TestKeyScopeIsolation(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)

	// Per-key scope: different key ids → different cache keys.
	pk := keyCache(ScopeKey)
	k1, _ := pk.Key("key-1", "openai", body)
	k2, _ := pk.Key("key-2", "openai", body)
	if k1 == k2 {
		t.Error("ScopeKey must namespace by api key id")
	}

	// Global scope: key id is ignored → same cache key.
	gl := keyCache(ScopeGlobal)
	g1, _ := gl.Key("key-1", "openai", body)
	g2, _ := gl.Key("key-2", "openai", body)
	if g1 != g2 {
		t.Error("ScopeGlobal must ignore api key id")
	}
}

func TestKeyRejectsNonJSON(t *testing.T) {
	c := keyCache(ScopeGlobal)
	if _, ok := c.Key("", "openai", []byte("not json")); ok {
		t.Error("Key should report ok=false for non-JSON bodies")
	}
}
