package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// GenesisSeed is the seed value for the first audit log entry's hash chain.
const GenesisSeed = "GENESIS"

// ChainRecord is a normalized representation of an audit log entry used for
// hash chain computation. It contains only the fields that participate in
// the hash — excluding the hash itself and the auto-generated timestamp
// is included for ordering but the hash is computed from the normalized
// representation of the *previous* record.
type ChainRecord struct {
	ID           string         `json:"id"`
	UserID       string         `json:"user_id"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Detail       map[string]any `json:"detail"`
	IP           string         `json:"ip"`
	PrevHash     string         `json:"prev_hash"`
	CreatedAt    time.Time      `json:"created_at"`
}

// ToChainRecord converts an Ent AuditLog entity to a ChainRecord.
func ToChainRecord(id, userID, action, resourceType, resourceID, ip, prevHash string,
	detail map[string]any, createdAt time.Time) ChainRecord {
	return ChainRecord{
		ID:           id,
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Detail:       detail,
		IP:           ip,
		PrevHash:     prevHash,
		CreatedAt:    createdAt,
	}
}

// Normalize serializes a ChainRecord into a deterministic JSON string.
// Map keys are sorted alphabetically by the encoder to ensure determinism.
func Normalize(r ChainRecord) string {
	// Use a struct→map approach to ensure deterministic ordering.
	// json.Marshal on a struct produces fields in struct definition order,
	// which is deterministic. For the Detail map, json.Marshal sorts keys
	// alphabetically by default.
	data, err := json.Marshal(r)
	if err != nil {
		// Fallback: should never happen for our well-typed struct
		return fmt.Sprintf("%v", r)
	}
	return string(data)
}

// ComputeHash computes the hash value that the *next* record should store
// as its prev_hash. Given the previous record, the hash is:
//
//	SHA256(prevRecord.prev_hash + normalize(prevRecord))
//
// For the genesis record (the very first entry), prev_hash is
// SHA256("GENESIS").
func ComputeHash(prevRecord ChainRecord) string {
	normalized := Normalize(prevRecord)
	payload := prevRecord.PrevHash + normalized
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// GenesisHash returns the initial prev_hash for the first audit log entry.
func GenesisHash() string {
	sum := sha256.Sum256([]byte(GenesisSeed))
	return hex.EncodeToString(sum[:])
}

// VerificationGap represents a break in the hash chain at a specific index.
type VerificationGap struct {
	Index   int    `json:"index"`
	ID      string `json:"id"`
	Reason  string `json:"reason"`
}

// VerifyChain verifies the integrity of a hash chain.
// logs must be ordered by created_at ascending (oldest first).
// Returns true if the entire chain is valid, along with a list of any gaps found.
func VerifyChain(logs []ChainRecord) (bool, []VerificationGap) {
	if len(logs) == 0 {
		return true, nil
	}

	var gaps []VerificationGap
	expectedPrevHash := GenesisHash()

	for i, log := range logs {
		if log.PrevHash != expectedPrevHash {
			gaps = append(gaps, VerificationGap{
				Index:  i,
				ID:     log.ID,
				Reason: fmt.Sprintf("prev_hash mismatch: expected %s, got %s", truncateHash(expectedPrevHash), truncateHash(log.PrevHash)),
			})
			// Recalculate expected from this record to continue checking downstream
			expectedPrevHash = ComputeHash(log)
			continue
		}
		// Hash matches — advance the chain
		expectedPrevHash = ComputeHash(log)
	}

	return len(gaps) == 0, gaps
}

// truncateHash shortens a hash for error message readability.
func truncateHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12] + "..."
}
