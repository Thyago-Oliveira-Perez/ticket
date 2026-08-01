// Package webhook delivers outbound webhook events to caller-supplied
// URLs, simulating the unreliable delivery of a real payment provider:
// injected latency and duplicate deliveries of the same event. Combined
// with independent per-delivery latency across concurrent payments, this
// also produces out-of-order arrival at the receiver without any special
// handling — event order isn't guaranteed, by design.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Config controls webhook delivery chaos. Zero value delivers immediately,
// exactly once, with no injected delay.
type Config struct {
	// LatencyMin and LatencyMax bound a random delay before each delivery
	// attempt (including duplicates). Disabled when LatencyMax <= 0.
	LatencyMin time.Duration
	LatencyMax time.Duration

	// DuplicateRate is the probability, in [0, 1], that an event is
	// delivered a second time with the identical id and body, simulating a
	// provider retry. Disabled when DuplicateRate <= 0.
	DuplicateRate float64
}

type envelope struct {
	ID         string    `json:"id"`
	Sequence   uint64    `json:"sequence"`
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurred_at"`
	Data       any       `json:"data"`
}

// Sender delivers webhook events over HTTP.
type Sender struct {
	cfg    Config
	client *http.Client
	seq    atomic.Uint64
}

func NewSender(cfg Config) *Sender {
	return &Sender{cfg: cfg, client: &http.Client{Timeout: 10 * time.Second}}
}

// Send delivers an eventType/data event to url as a JSON webhook, signed
// with secret (Stripe-style: a "t=<unix>,v1=<hmac>" header over
// "<unix>.<body>"), or unsigned if secret is empty. It blocks for the
// simulated latency and any duplicate delivery, so callers wanting
// fire-and-forget semantics should invoke it in a goroutine. Delivery
// failures (network errors, non-2xx responses) are logged, not returned —
// nubrank doesn't retry on failure beyond the configured duplicate rate.
func (s *Sender) Send(ctx context.Context, url, secret, eventType string, data any) error {
	id := uuid.NewString()

	body, err := json.Marshal(envelope{
		ID:         id,
		Sequence:   s.seq.Add(1),
		Type:       eventType,
		OccurredAt: time.Now(),
		Data:       data,
	})
	if err != nil {
		return fmt.Errorf("marshal webhook event: %w", err)
	}

	s.deliver(ctx, url, secret, id, body)

	if s.cfg.DuplicateRate > 0 && rand.Float64() < s.cfg.DuplicateRate {
		s.deliver(ctx, url, secret, id, body)
	}

	return nil
}

func (s *Sender) deliver(ctx context.Context, url, secret, eventID string, body []byte) {
	s.sleepLatency()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook: build request for %s: %v", url, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event-Id", eventID)
	if secret != "" {
		req.Header.Set("X-Webhook-Signature", sign(secret, body))
	}

	resp, err := s.client.Do(req)
	if err != nil {
		log.Printf("webhook: deliver event %s to %s: %v", eventID, url, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Printf("webhook: event %s to %s got status %d", eventID, url, resp.StatusCode)
	}
}

// sign produces a Stripe-style signature header: a timestamp plus an
// HMAC-SHA256 of "<timestamp>.<body>" keyed by secret, so a receiver can
// verify both authenticity and (via the timestamp) freshness.
func sign(secret string, body []byte) string {
	ts := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", ts)
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func (s *Sender) sleepLatency() {
	if s.cfg.LatencyMax <= 0 {
		return
	}
	d := s.cfg.LatencyMin
	if s.cfg.LatencyMax > s.cfg.LatencyMin {
		d += time.Duration(rand.Int64N(int64(s.cfg.LatencyMax - s.cfg.LatencyMin)))
	}
	time.Sleep(d)
}
