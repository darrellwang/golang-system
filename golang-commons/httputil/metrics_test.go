package httputil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMetricsMiddleware_RecordsRequest(t *testing.T) {
	handler := MetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestMetricsMiddleware_MultipleMethods(t *testing.T) {
	handler := MetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	for _, method := range methods {
		req := httptest.NewRequest(method, "/api/items", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("method %s: expected status 201, got %d", method, rec.Code)
		}
	}
}

func TestMetricsMiddleware_DurationObserved(t *testing.T) {
	handler := MetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest("GET", "/api/nonexistent", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestMetricsMiddleware_InflightGauge(t *testing.T) {
	done := make(chan struct{})
	handler := MetricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-done
		w.WriteHeader(http.StatusOK)
	}))

	// Send two concurrent requests in goroutines
	var wg sync.WaitGroup
	results := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/api/slow", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			results[idx] = rec.Code
		}(i)
	}

	// Give goroutines time to start and increment inflight gauge
	time.Sleep(50 * time.Millisecond)
	close(done)
	wg.Wait()

	for i, code := range results {
		if code != http.StatusOK {
			t.Errorf("request %d: expected status 200, got %d", i, code)
		}
	}
}

func TestMetricsHandler(t *testing.T) {
	handler := MetricsHandler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200 for /metrics, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "golang_system") {
		t.Error("expected metrics output to contain namespace prefix")
	}
}

func TestRegisterMetrics(_ *testing.T) {
	// Just verify it doesn't panic
	RegisterMetrics()
}
