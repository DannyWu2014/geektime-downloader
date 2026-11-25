package geektime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestClientRateLimitSpacing(t *testing.T) {
	var mu sync.Mutex
	var timestamps []time.Time

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		timestamps = append(timestamps, time.Now())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"page":{"more":false},"list":[]}}`))
	}))
	defer srv.Close()

	client := NewClient(nil)
	client.SetBaseURL(srv.URL)
	interval := 80 * time.Millisecond
	client.SetRequestInterval(interval, 0)

	for i := 0; i < 3; i++ {
		if _, err := client.NoteList(100 + i); err != nil {
			t.Fatalf("NoteList(%d) failed: %v", i, err)
		}
	}

	if len(timestamps) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(timestamps))
	}

	for i := 1; i < len(timestamps); i++ {
		gap := timestamps[i].Sub(timestamps[i-1])
		if gap < interval-15*time.Millisecond {
			t.Errorf("gap between request %d and %d = %v, want >= %v", i-1, i, gap, interval)
		}
	}
}

func TestClientRateLimitConcurrent(t *testing.T) {
	var mu sync.Mutex
	var timestamps []time.Time

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		timestamps = append(timestamps, time.Now())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"page":{"more":false},"list":[]}}`))
	}))
	defer srv.Close()

	client := NewClient(nil)
	client.SetBaseURL(srv.URL)
	interval := 60 * time.Millisecond
	client.SetRequestInterval(interval, 20*time.Millisecond)

	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_, err := client.NoteList(id)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent NoteList failed: %v", err)
		}
	}

	if len(timestamps) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(timestamps))
	}
	totalSpan := timestamps[2].Sub(timestamps[0])
	if totalSpan < 2*interval-20*time.Millisecond {
		t.Errorf("total span = %v, want >= %v", totalSpan, 2*interval)
	}
}

func TestClientRateLimitContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	client := NewClient(nil)
	client.SetRequestInterval(5*time.Second, 0)

	var res struct {
		Code int `json:"code"`
	}
	r1 := client.newRequest("POST", srv.URL, "/first", nil, nil, &res)
	if _, err := client.do(r1); err != nil {
		t.Fatalf("first request failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	r2 := client.newRequest("POST", srv.URL, "/second", nil, nil, &res).SetContext(ctx)

	start := time.Now()
	_, err := client.do(r2)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("expected prompt cancellation, took %v", elapsed)
	}
}

func TestClientRateLimit451(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(451)
	}))
	defer srv.Close()

	client := NewClient(nil)
	client.SetBaseURL(srv.URL)
	client.SetRequestInterval(0, 0)

	_, err := client.NoteList(1)
	if !errors.Is(err, ErrGeekTimeRateLimit) {
		t.Fatalf("expected ErrGeekTimeRateLimit, got %v", err)
	}
}
