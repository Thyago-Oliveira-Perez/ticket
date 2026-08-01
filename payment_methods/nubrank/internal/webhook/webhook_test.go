package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type receivedRequest struct {
	eventID   string
	signature string
	body      envelope
}

func newCapturingServer(t *testing.T, mu *sync.Mutex, received *[]receivedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env envelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Errorf("decode webhook body: %v", err)
		}
		mu.Lock()
		*received = append(*received, receivedRequest{
			eventID:   r.Header.Get("X-Webhook-Event-Id"),
			signature: r.Header.Get("X-Webhook-Signature"),
			body:      env,
		})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
}

func TestSend_DeliversOnceByDefault(t *testing.T) {
	var mu sync.Mutex
	var received []receivedRequest
	srv := newCapturingServer(t, &mu, &received)
	defer srv.Close()

	sender := NewSender(Config{})
	if err := sender.Send(context.Background(), srv.URL, "", "payment.approved", map[string]string{"id": "p1"}); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected exactly 1 delivery, got %d", len(received))
	}
	if received[0].body.Type != "payment.approved" {
		t.Fatalf("expected event type payment.approved, got %s", received[0].body.Type)
	}
	if received[0].eventID == "" || received[0].eventID != received[0].body.ID {
		t.Fatalf("expected header event id to match body id, got header=%q body=%q", received[0].eventID, received[0].body.ID)
	}
	if received[0].signature != "" {
		t.Fatalf("expected no signature header without a secret, got %q", received[0].signature)
	}
}

func TestSend_WithSecret_SignsRequest(t *testing.T) {
	var mu sync.Mutex
	var received []receivedRequest
	srv := newCapturingServer(t, &mu, &received)
	defer srv.Close()

	sender := NewSender(Config{})
	if err := sender.Send(context.Background(), srv.URL, "whsec_test", "payment.approved", map[string]string{"id": "p1"}); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected exactly 1 delivery, got %d", len(received))
	}
	if !strings.HasPrefix(received[0].signature, "t=") || !strings.Contains(received[0].signature, ",v1=") {
		t.Fatalf("expected a Stripe-style signature header, got %q", received[0].signature)
	}
}

func TestSend_DuplicateRateOneSendsTwiceWithSameID(t *testing.T) {
	var mu sync.Mutex
	var received []receivedRequest
	srv := newCapturingServer(t, &mu, &received)
	defer srv.Close()

	sender := NewSender(Config{DuplicateRate: 1})
	if err := sender.Send(context.Background(), srv.URL, "", "payment.approved", map[string]string{"id": "p1"}); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected exactly 2 deliveries with DuplicateRate=1, got %d", len(received))
	}
	if received[0].body.ID != received[1].body.ID {
		t.Fatalf("expected duplicate deliveries to share the same event id, got %q and %q", received[0].body.ID, received[1].body.ID)
	}
}

func TestSend_SequenceIncreasesAcrossCalls(t *testing.T) {
	var mu sync.Mutex
	var received []receivedRequest
	srv := newCapturingServer(t, &mu, &received)
	defer srv.Close()

	sender := NewSender(Config{})
	for i := 0; i < 3; i++ {
		if err := sender.Send(context.Background(), srv.URL, "", "payment.approved", nil); err != nil {
			t.Fatalf("Send returned error: %v", err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 3 {
		t.Fatalf("expected 3 deliveries, got %d", len(received))
	}
	for i := 1; i < len(received); i++ {
		if received[i].body.Sequence <= received[i-1].body.Sequence {
			t.Fatalf("expected strictly increasing sequence numbers, got %d then %d", received[i-1].body.Sequence, received[i].body.Sequence)
		}
	}
}
