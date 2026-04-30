package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/your-org/platform-service/internal/health"
	"github.com/your-org/platform-service/internal/provisioner"
	"github.com/your-org/platform-service/internal/workflow"
)

type BucketProvisioner interface {
	ProvisionBucket(context.Context, provisioner.BucketRequest) (provisioner.BucketResult, error)
}

type Server struct {
	buckets             BucketProvisioner
	health              HealthChecker
	githubWebhooks      GitHubWebhookProcessor
	githubWebhookSecret string
	logger              *slog.Logger
	mux                 *http.ServeMux
}

type GitHubWebhookProcessor interface {
	ProcessGitHubWebhook(context.Context, workflow.GitHubWebhookEvent) (workflow.GitHubWebhookResult, error)
}

type HealthChecker interface {
	CheckHealth(context.Context) health.AggregateResult
}

type ServerOption func(*Server)

func WithHealthChecker(checker HealthChecker) ServerOption {
	return func(server *Server) {
		server.health = checker
	}
}

func WithGitHubWebhooks(processor GitHubWebhookProcessor) ServerOption {
	return func(server *Server) {
		server.githubWebhooks = processor
	}
}

func WithGitHubWebhookSecret(secret string) ServerOption {
	return func(server *Server) {
		server.githubWebhookSecret = strings.TrimSpace(secret)
	}
}

func NewServer(buckets BucketProvisioner, logger *slog.Logger, options ...ServerOption) http.Handler {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}

	server := &Server{
		buckets: buckets,
		logger:  logger,
		mux:     http.NewServeMux(),
	}
	for _, option := range options {
		option(server)
	}

	server.routes()
	return server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := requestID(r)
	w.Header().Set("X-Request-ID", requestID)

	started := time.Now()
	s.mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID)))
	s.logger.Info("request completed",
		"method", r.Method,
		"path", r.URL.Path,
		"request_id", requestID,
		"duration_ms", time.Since(started).Milliseconds(),
	)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /v1/health-checks", s.handleHealthChecks)
	s.mux.HandleFunc("POST /v1/s3-buckets", s.handleCreateBucket)
	s.mux.HandleFunc("POST /v1/github/webhook", s.handleGitHubWebhook)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleHealthChecks(w http.ResponseWriter, r *http.Request) {
	if s.health == nil {
		writeJSON(w, http.StatusOK, health.AggregateResult{
			Status:    health.StatusHealthy,
			CheckedAt: time.Now().UTC(),
			Services:  []health.ServiceResult{},
		})
		return
	}

	result := s.health.CheckHealth(r.Context())
	status := http.StatusOK
	if result.Status != health.StatusHealthy {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, result)
}

func (s *Server) handleCreateBucket(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var request provisioner.BucketRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
		return
	}

	result, err := s.buckets.ProvisionBucket(r.Context(), request)
	if err != nil {
		var validationErr *provisioner.ValidationError
		if errors.As(err, &validationErr) {
			writeJSON(w, http.StatusBadRequest, errorResponse{
				Error:   "validation_failed",
				Message: validationErr.Error(),
				Fields:  validationErr.Fields,
			})
			return
		}

		s.logger.Error("bucket provisioning failed",
			"error", err,
			"request_id", r.Context().Value(requestIDKey{}),
		)
		writeError(w, http.StatusInternalServerError, "provisioning_failed", "failed to provision requested bucket")
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	eventName := strings.TrimSpace(r.Header.Get("X-GitHub-Event"))
	if eventName == "" {
		writeError(w, http.StatusBadRequest, "missing_event", "X-GitHub-Event header is required")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "payload_too_large", "webhook payload must be 2 MiB or smaller")
		return
	}

	if s.githubWebhookSecret != "" && !validGitHubSignature(r.Header.Get("X-Hub-Signature-256"), body, s.githubWebhookSecret) {
		writeError(w, http.StatusUnauthorized, "invalid_signature", "webhook signature verification failed")
		return
	}

	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, "invalid_json", "webhook payload must be valid JSON")
		return
	}

	if s.githubWebhooks == nil {
		writeJSON(w, http.StatusAccepted, workflow.GitHubWebhookResult{
			Event:      eventName,
			DeliveryID: strings.TrimSpace(r.Header.Get("X-GitHub-Delivery")),
			Actions:    []string{"webhook_processor_not_configured"},
		})
		return
	}

	result, err := s.githubWebhooks.ProcessGitHubWebhook(r.Context(), workflow.GitHubWebhookEvent{
		Event:      eventName,
		DeliveryID: strings.TrimSpace(r.Header.Get("X-GitHub-Delivery")),
		Payload:    body,
	})
	if err != nil {
		s.logger.Error("github webhook processing failed",
			"error", err,
			"event", eventName,
			"delivery_id", strings.TrimSpace(r.Header.Get("X-GitHub-Delivery")),
			"request_id", r.Context().Value(requestIDKey{}),
		)
		writeError(w, http.StatusBadRequest, "webhook_processing_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, result)
}

type requestIDKey struct{}

type errorResponse struct {
	Error   string            `json:"error"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Error: code, Message: message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func requestID(r *http.Request) string {
	if header := strings.TrimSpace(r.Header.Get("X-Request-ID")); header != "" {
		return header
	}

	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(data[:])
}

func validGitHubSignature(signatureHeader string, body []byte, secret string) bool {
	signatureHeader = strings.TrimSpace(signatureHeader)
	if !strings.HasPrefix(signatureHeader, "sha256=") {
		return false
	}

	received, err := hex.DecodeString(strings.TrimPrefix(signatureHeader, "sha256="))
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(received, expected)
}
