package idempotency

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned by GetByMerchantAndKey when no record exists for
// the (merchant_id, key) pair.
var ErrNotFound = errors.New("idempotency key not found")

// Record is a reserved or completed idempotency key. ResponseStatus and
// ResponseBody are nil until the in-flight request that reserved the key
// finishes.
type Record struct {
	ID             string
	MerchantID     string
	Key            string
	RequestHash    string
	ResponseStatus *int
	ResponseBody   []byte
	CreatedAt      time.Time
}

type Repository interface {
	// Reserve atomically inserts a placeholder record for (merchantID,
	// key), or — if one already exists — returns it instead. reserved is
	// true only if this call created the record, meaning the caller now
	// owns completing it via Complete.
	Reserve(ctx context.Context, merchantID, key, requestHash string) (record Record, reserved bool, err error)
	// Complete fills in the final response for a record previously
	// reserved by this caller.
	Complete(ctx context.Context, id string, responseStatus int, responseBody []byte) error
}
