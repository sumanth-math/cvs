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

	"github.com/your-org/platform-service/internal/catalog"
	"github.com/your-org/platform-service/internal/health"
	"github.com/your-org/platform-service/internal/provisioner"
	"github.com/your-org/platform-service/internal/workflow"
)

type BucketProvisioner interface {
	ProvisionBucket(context.Context, provisioner.BucketRequest) (provisioner.BucketResult, error)
}

type Server struct {
	buckets             BucketProvisioner
	catalog             CatalogReader
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

type CatalogReader interface {
	Snapshot(context.Context) catalog.Catalog
}

type ServerOption func(*Server)

func WithHealthChecker(checker HealthChecker) ServerOption {
	return func(server *Server) {
		server.health = checker
	}
}

func WithCatalog(reader CatalogReader) ServerOption {
	return func(server *Server) {
		server.catalog = reader
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
	s.mux.HandleFunc("GET /v1/catalog", s.handleCatalog)
	s.mux.HandleFunc("GET /v1/catalog/services", s.handleCatalogServices)
	s.mux.HandleFunc("GET /v1/catalog/services/{id}", s.handleCatalogService)
	s.mux.HandleFunc("GET /v1/catalog/environments", s.handleCatalogEnvironments)
	s.mux.HandleFunc("GET /v1/catalog/environments/{id}", s.handleCatalogEnvironment)
	s.mux.HandleFunc("GET /v1/catalog/infrastructure", s.handleCatalogInfrastructure)
	s.mux.HandleFunc("GET /v1/catalog/infrastructure/{id}", s.handleCatalogInfrastructureResource)
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

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.catalogSnapshot(r.Context()))
}

func (s *Server) handleCatalogServices(w http.ResponseWriter, r *http.Request) {
	services := filterServices(s.catalogSnapshot(r.Context()).Services, r.URL.Query().Get("owner"), r.URL.Query().Get("environment"))
	writeJSON(w, http.StatusOK, map[string][]catalog.Service{"services": services})
}

func (s *Server) handleCatalogService(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	for _, service := range s.catalogSnapshot(r.Context()).Services {
		if service.ID == id {
			writeJSON(w, http.StatusOK, service)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "catalog service not found")
}

func (s *Server) handleCatalogEnvironments(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string][]catalog.Environment{"environments": s.catalogSnapshot(r.Context()).Environments})
}

func (s *Server) handleCatalogEnvironment(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	for _, environment := range s.catalogSnapshot(r.Context()).Environments {
		if environment.ID == id {
			writeJSON(w, http.StatusOK, environment)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "catalog environment not found")
}

func (s *Server) handleCatalogInfrastructure(w http.ResponseWriter, r *http.Request) {
	resources := filterInfrastructure(s.catalogSnapshot(r.Context()).Infrastructure, r.URL.Query().Get("environment"), r.URL.Query().Get("type"))
	writeJSON(w, http.StatusOK, map[string][]catalog.InfrastructureResource{"infrastructure": resources})
}

func (s *Server) handleCatalogInfrastructureResource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	for _, resource := range s.catalogSnapshot(r.Context()).Infrastructure {
		if resource.ID == id {
			writeJSON(w, http.StatusOK, resource)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "catalog infrastructure resource not found")
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

func (s *Server) catalogSnapshot(ctx context.Context) catalog.Catalog {
	if s.catalog == nil {
		return catalog.Catalog{
			Services:       []catalog.Service{},
			Environments:   []catalog.Environment{},
			Infrastructure: []catalog.InfrastructureResource{},
		}
	}
	return s.catalog.Snapshot(ctx)
}

func filterServices(services []catalog.Service, owner, environment string) []catalog.Service {
	owner = strings.TrimSpace(owner)
	environment = strings.TrimSpace(environment)
	if owner == "" && environment == "" {
		return services
	}

	filtered := make([]catalog.Service, 0, len(services))
	for _, service := range services {
		if owner != "" && service.Owner != owner {
			continue
		}
		if environment != "" && !containsString(service.Environments, environment) {
			continue
		}
		filtered = append(filtered, service)
	}
	return filtered
}

func filterInfrastructure(resources []catalog.InfrastructureResource, environment, resourceType string) []catalog.InfrastructureResource {
	environment = strings.TrimSpace(environment)
	resourceType = strings.TrimSpace(resourceType)
	if environment == "" && resourceType == "" {
		return resources
	}

	filtered := make([]catalog.InfrastructureResource, 0, len(resources))
	for _, resource := range resources {
		if environment != "" && resource.Environment != environment {
			continue
		}
		if resourceType != "" && resource.Type != resourceType {
			continue
		}
		filtered = append(filtered, resource)
	}
	return filtered
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
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
