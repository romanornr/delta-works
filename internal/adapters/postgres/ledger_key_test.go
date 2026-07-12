package postgres

import "testing"

func TestInventoryLockKeyIsInjectiveAcrossSeparators(t *testing.T) {
	a := inventoryLockKey("a|b", "c", "base", "quote")
	b := inventoryLockKey("a", "b|c", "base", "quote")
	if a == b {
		t.Fatal("tuples differing only in field boundaries produced the same lock key")
	}
	if a != inventoryLockKey("a|b", "c", "base", "quote") {
		t.Fatal("lock key is not deterministic")
	}
}
