package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"
)

func makeRecord(id, prevHash string) ChainRecord {
	return ChainRecord{
		ID:           id,
		UserID:       "user-1",
		Action:       "create",
		ResourceType: "clusters",
		ResourceID:   "res-1",
		Detail:       map[string]any{"name": "test"},
		IP:           "127.0.0.1",
		PrevHash:     prevHash,
		CreatedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestGenesisHash(t *testing.T) {
	t.Parallel()
	gh := GenesisHash()
	expected := sha256.Sum256([]byte("GENESIS"))
	expectedHex := hex.EncodeToString(expected[:])
	if gh != expectedHex {
		t.Errorf("GenesisHash mismatch: expected %s, got %s", expectedHex, gh)
	}
}

func TestNormalize_Deterministic(t *testing.T) {
	t.Parallel()
	r1 := makeRecord("a", GenesisHash())
	r2 := makeRecord("a", GenesisHash())
	n1 := Normalize(r1)
	n2 := Normalize(r2)
	if n1 != n2 {
		t.Errorf("Normalize should be deterministic for identical records")
	}
}

func TestNormalize_DifferentRecords(t *testing.T) {
	t.Parallel()
	r1 := makeRecord("a", GenesisHash())
	r2 := makeRecord("b", GenesisHash())
	n1 := Normalize(r1)
	n2 := Normalize(r2)
	if n1 == n2 {
		t.Errorf("Normalize should differ for different records")
	}
}

func TestNormalize_MapKeyOrdering(t *testing.T) {
	t.Parallel()
	r1 := ChainRecord{
		ID:           "x",
		UserID:       "u",
		Action:       "create",
		ResourceType: "clusters",
		ResourceID:   "1",
		Detail:       map[string]any{"z": "last", "a": "first", "m": "middle"},
		IP:           "1.1.1.1",
		PrevHash:     GenesisHash(),
		CreatedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	r2 := ChainRecord{
		ID:           "x",
		UserID:       "u",
		Action:       "create",
		ResourceType: "clusters",
		ResourceID:   "1",
		Detail:       map[string]any{"a": "first", "m": "middle", "z": "last"},
		IP:           "1.1.1.1",
		PrevHash:     GenesisHash(),
		CreatedAt:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	// Both should produce the same normalized string despite different map insertion order
	if Normalize(r1) != Normalize(r2) {
		t.Errorf("Normalize should produce same output regardless of map key order")
	}
}

func TestComputeHash_Deterministic(t *testing.T) {
	t.Parallel()
	r := makeRecord("a", GenesisHash())
	h1 := ComputeHash(r)
	h2 := ComputeHash(r)
	if h1 != h2 {
		t.Errorf("ComputeHash should be deterministic")
	}
}

func TestComputeHash_DifferentInputs(t *testing.T) {
	t.Parallel()
	r1 := makeRecord("a", GenesisHash())
	r2 := makeRecord("b", GenesisHash())
	h1 := ComputeHash(r1)
	h2 := ComputeHash(r2)
	if h1 == h2 {
		t.Errorf("ComputeHash should differ for different records")
	}
}

func TestComputeHash_Format(t *testing.T) {
	t.Parallel()
	r := makeRecord("a", GenesisHash())
	h := ComputeHash(r)
	// SHA256 hex digest = 64 chars
	if len(h) != 64 {
		t.Errorf("ComputeHash should return 64-char hex string, got %d chars", len(h))
	}
}

func TestVerifyChain_Empty(t *testing.T) {
	t.Parallel()
	valid, gaps := VerifyChain(nil)
	if !valid {
		t.Errorf("empty chain should be valid")
	}
	if len(gaps) != 0 {
		t.Errorf("empty chain should have no gaps")
	}
}

func TestVerifyChain_SingleEntry_Valid(t *testing.T) {
	t.Parallel()
	r := makeRecord("r1", GenesisHash())
	valid, gaps := VerifyChain([]ChainRecord{r})
	if !valid {
		t.Errorf("chain with correct genesis hash should be valid, gaps: %v", gaps)
	}
}

func TestVerifyChain_SingleEntry_Invalid(t *testing.T) {
	t.Parallel()
	r := makeRecord("r1", "bogus_hash")
	valid, gaps := VerifyChain([]ChainRecord{r})
	if valid {
		t.Errorf("chain with wrong genesis hash should be invalid")
	}
	if len(gaps) != 1 {
		t.Fatalf("expected 1 gap, got %d", len(gaps))
	}
	if gaps[0].Index != 0 {
		t.Errorf("gap should be at index 0, got %d", gaps[0].Index)
	}
}

func TestVerifyChain_MultipleEntries_Valid(t *testing.T) {
	t.Parallel()
	r1 := makeRecord("r1", GenesisHash())
	r2 := makeRecord("r2", ComputeHash(r1))
	r3 := makeRecord("r3", ComputeHash(r2))

	valid, gaps := VerifyChain([]ChainRecord{r1, r2, r3})
	if !valid {
		t.Errorf("chain with correct links should be valid, gaps: %v", gaps)
	}
}

func TestVerifyChain_MultipleEntries_TamperedMiddle(t *testing.T) {
	t.Parallel()
	r1 := makeRecord("r1", GenesisHash())
	r2 := makeRecord("r2", ComputeHash(r1))
	r3 := makeRecord("r3", ComputeHash(r2))

	// Tamper r2's action
	r2Tampered := r2
	r2Tampered.Action = "delete"
	// r3's prev_hash still points to original r2
	r3Copy := r3

	valid, gaps := VerifyChain([]ChainRecord{r1, r2Tampered, r3Copy})
	// r2 is tampered, so when we get to r3, the expected hash (computed from tampered r2)
	// won't match r3.prev_hash (computed from original r2)
	if valid {
		t.Errorf("tampered chain should be invalid")
	}
	if len(gaps) == 0 {
		t.Fatalf("expected at least 1 gap")
	}
	// The gap should be at index 2 (r3) because r3.prev_hash was computed from the original r2
	foundAt2 := false
	for _, g := range gaps {
		if g.Index == 2 {
			foundAt2 = true
		}
	}
	if !foundAt2 {
		t.Errorf("expected a gap at index 2, gaps: %v", gaps)
	}
}

func TestVerifyChain_MultipleEntries_TamperedFirst(t *testing.T) {
	t.Parallel()
	r1 := makeRecord("r1", GenesisHash())
	r2 := makeRecord("r2", ComputeHash(r1))

	// Tamper r1 prev_hash
	r1.PrevHash = "wrong_genesis"

	valid, gaps := VerifyChain([]ChainRecord{r1, r2})
	if valid {
		t.Errorf("tampered chain should be invalid")
	}
	if len(gaps) != 2 {
		t.Fatalf("expected 2 gaps (tamper at 0 propagates to 1), got %d: %v", len(gaps), gaps)
	}
	if gaps[0].Index != 0 {
		t.Errorf("first gap should be at index 0")
	}
	if gaps[1].Index != 1 {
		t.Errorf("second gap should be at index 1 (propagation)")
	}
}

func TestVerifyChain_GapPropagates(t *testing.T) {
	t.Parallel()
	r1 := makeRecord("r1", GenesisHash())
	r2 := makeRecord("r2", ComputeHash(r1))
	r3 := makeRecord("r3", ComputeHash(r2))

	// Break r2's prev_hash
	r2.PrevHash = "broken"

	valid, gaps := VerifyChain([]ChainRecord{r1, r2, r3})
	if valid {
		t.Errorf("broken chain should be invalid")
	}
	// Should detect gap at index 1 (r2)
	// r3 may or may not also be flagged depending on propagation
	foundAt1 := false
	for _, g := range gaps {
		if g.Index == 1 {
			foundAt1 = true
		}
	}
	if !foundAt1 {
		t.Errorf("expected a gap at index 1, gaps: %v", gaps)
	}
}
