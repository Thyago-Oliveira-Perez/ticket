package idempotency

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

type fakeRepository struct {
	mu      sync.Mutex
	records map[string]*Record // key: merchantID+"|"+key
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{records: make(map[string]*Record)}
}

func (f *fakeRepository) Reserve(ctx context.Context, merchantID, key, requestHash string) (Record, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	k := merchantID + "|" + key
	if existing, ok := f.records[k]; ok {
		return *existing, false, nil
	}

	rec := &Record{ID: uuid.NewString(), MerchantID: merchantID, Key: key, RequestHash: requestHash}
	f.records[k] = rec
	return *rec, true, nil
}

func (f *fakeRepository) Complete(ctx context.Context, id string, status int, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, rec := range f.records {
		if rec.ID == id {
			rec.ResponseStatus = &status
			rec.ResponseBody = body
			return nil
		}
	}
	return ErrNotFound
}

func countingHandler(calls *int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ok":true}`))
	})
}

func TestMiddleware_NoKey_PassesThrough(t *testing.T) {
	var calls int
	repo := newFakeRepository()
	h := Middleware(repo)(countingHandler(&calls))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if calls != 1 {
		t.Fatalf("expected handler to be called once, got %d", calls)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}
}

func TestMiddleware_FirstRequest_RunsHandlerAndPersists(t *testing.T) {
	var calls int
	repo := newFakeRepository()
	h := Middleware(repo)(countingHandler(&calls))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":1}`))
	req.Header.Set("Idempotency-Key", "key-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if calls != 1 {
		t.Fatalf("expected handler to be called once, got %d", calls)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", rec.Code)
	}

	stored, ok := repo.records["|key-1"]
	if !ok {
		t.Fatal("expected a record to be persisted")
	}
	if stored.ResponseStatus == nil || *stored.ResponseStatus != http.StatusCreated {
		t.Fatalf("expected persisted response status 201, got %v", stored.ResponseStatus)
	}
}

func TestMiddleware_RepeatedKeySameBody_ReplaysWithoutRerunningHandler(t *testing.T) {
	var calls int
	repo := newFakeRepository()
	h := Middleware(repo)(countingHandler(&calls))

	body := `{"a":1}`
	req1 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req1.Header.Set("Idempotency-Key", "key-2")
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)

	req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req2.Header.Set("Idempotency-Key", "key-2")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if calls != 1 {
		t.Fatalf("expected handler to run exactly once across both requests, got %d", calls)
	}
	if rec2.Code != http.StatusCreated {
		t.Fatalf("expected replayed status 201, got %d", rec2.Code)
	}
	if rec2.Body.String() != `{"ok":true}` {
		t.Fatalf("expected replayed body, got %q", rec2.Body.String())
	}
}

func TestMiddleware_RepeatedKeyDifferentBody_ReturnsConflict(t *testing.T) {
	var calls int
	repo := newFakeRepository()
	h := Middleware(repo)(countingHandler(&calls))

	req1 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":1}`))
	req1.Header.Set("Idempotency-Key", "key-3")
	h.ServeHTTP(httptest.NewRecorder(), req1)

	req2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":2}`))
	req2.Header.Set("Idempotency-Key", "key-3")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)

	if calls != 1 {
		t.Fatalf("expected handler to run only for the first request, got %d calls", calls)
	}
	if rec2.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422, got %d", rec2.Code)
	}
}

func TestMiddleware_InFlightDuplicate_ReturnsConflict(t *testing.T) {
	repo := newFakeRepository()
	// Simulate a first request that's still being processed: reserved but
	// not yet completed.
	repo.records["|key-4"] = &Record{ID: uuid.NewString(), Key: "key-4", RequestHash: hashRequest([]byte(`{"a":1}`))}

	var calls int
	h := Middleware(repo)(countingHandler(&calls))

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":1}`))
	req.Header.Set("Idempotency-Key", "key-4")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if calls != 0 {
		t.Fatalf("expected handler not to run while a request is in flight, got %d calls", calls)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", rec.Code)
	}
}
