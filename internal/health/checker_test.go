package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestParseTargetsAppliesDefaults(t *testing.T) {
	targets, err := ParseTargets(`[{"name":"github","url":"https://api.github.com/meta"}]`)
	if err != nil {
		t.Fatalf("parse targets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}

	target := targets[0]
	if target.Name != "github" {
		t.Fatalf("unexpected target name: %q", target.Name)
	}
	if target.Method != http.MethodGet {
		t.Fatalf("expected default method %s, got %s", http.MethodGet, target.Method)
	}
	if target.ExpectedStatus != http.StatusOK {
		t.Fatalf("expected default status %d, got %d", http.StatusOK, target.ExpectedStatus)
	}
	if target.Timeout != 2*time.Second {
		t.Fatalf("expected default timeout 2s, got %s", target.Timeout)
	}
}

func TestParseTargetsSupportsSnakeCaseExpectedStatus(t *testing.T) {
	targets, err := ParseTargets(`[{"name":"status","url":"https://example.com/health","method":"HEAD","expected_status":204,"timeout":"750ms"}]`)
	if err != nil {
		t.Fatalf("parse targets: %v", err)
	}

	target := targets[0]
	if target.Method != http.MethodHead {
		t.Fatalf("expected HEAD method, got %s", target.Method)
	}
	if target.ExpectedStatus != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, target.ExpectedStatus)
	}
	if target.Timeout != 750*time.Millisecond {
		t.Fatalf("expected timeout 750ms, got %s", target.Timeout)
	}
}

func TestParseTargetsRejectsInvalidTarget(t *testing.T) {
	_, err := ParseTargets(`[{"name":"bad","url":"ftp://example.com/health"}]`)
	if err == nil {
		t.Fatal("expected invalid target error")
	}
}

func TestCheckerAggregatesHealthyTargets(t *testing.T) {
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(service.Close)

	checker := NewChecker([]Target{
		{Name: "status-api", URL: service.URL, ExpectedStatus: http.StatusNoContent},
	}, service.Client())

	result := checker.CheckHealth(context.Background())

	if result.Status != StatusHealthy {
		t.Fatalf("expected aggregate healthy, got %q", result.Status)
	}
	if len(result.Services) != 1 {
		t.Fatalf("expected 1 service result, got %d", len(result.Services))
	}
	if result.Services[0].Status != StatusHealthy {
		t.Fatalf("expected service healthy, got %q", result.Services[0].Status)
	}
	if result.Services[0].HTTPStatus != http.StatusNoContent {
		t.Fatalf("expected HTTP status %d, got %d", http.StatusNoContent, result.Services[0].HTTPStatus)
	}
}

func TestCheckerAggregatesUnhealthyTargets(t *testing.T) {
	healthyService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthyService.Close)

	unhealthyService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(unhealthyService.Close)

	checker := NewChecker([]Target{
		{Name: "identity", URL: healthyService.URL},
		{Name: "payments", URL: unhealthyService.URL},
	}, healthyService.Client())

	result := checker.CheckHealth(context.Background())

	if result.Status != StatusUnhealthy {
		t.Fatalf("expected aggregate unhealthy, got %q", result.Status)
	}
	if len(result.Services) != 2 {
		t.Fatalf("expected 2 service results, got %d", len(result.Services))
	}
	if result.Services[0].Status != StatusHealthy {
		t.Fatalf("expected first service healthy, got %q", result.Services[0].Status)
	}
	if result.Services[1].Status != StatusUnhealthy {
		t.Fatalf("expected second service unhealthy, got %q", result.Services[1].Status)
	}
	if result.Services[1].HTTPStatus != http.StatusInternalServerError {
		t.Fatalf("expected HTTP status %d, got %d", http.StatusInternalServerError, result.Services[1].HTTPStatus)
	}
}
