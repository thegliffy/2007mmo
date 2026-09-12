package hub

import "testing"

func TestTokenBucketAllowsBurstThenBlocks(t *testing.T) {
	b := newBucket(1, 2)
	if !b.allow() || !b.allow() {
		t.Fatal("burst of 2 should pass")
	}
	if b.allow() {
		t.Fatal("third token should be refused")
	}
}

func TestKeyedLimiterSeparatesKeys(t *testing.T) {
	k := newKeyedLimiter(1, 1)
	if !k.allow("a") {
		t.Fatal("first a")
	}
	if k.allow("a") {
		t.Fatal("second a should block")
	}
	if !k.allow("b") {
		t.Fatal("b should have its own bucket")
	}
}
