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
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/your-org/platform-service/internal/catalog"
	"github.com/your-org/platform-service/internal/health"
	"github.com/your-org/platform-service/internal/provisioner"
	"github.com/your-org/platform-service/internal/workflow"
)

const (
	maxBucketRequestBytes  = 1 << 20
	maxWebhookPayloadBytes = 2 << 20
	maxQueryValueLength    = 128
	recordWriteTimeout     = 3 * time.Second
)

var (
	catalogIDPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	githubEventPattern  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
	githubDeliveryLimit = 128
)

type BucketProvisioner interface {
	ProvisionBucket(context.Context, provisioner.BucketRequest) (provisioner.BucketResult, error)
}

type BucketProvisionRecorder interface {
	RecordBucketProvisioned(context.Context, provisioner.BucketRequest, provisioner.BucketResult, string) error
}

type Server struct {
	buckets             BucketProvisioner
	bucketRecords       BucketProvisionRecorder
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

func WithBucketProvisionRecorder(recorder BucketProvisionRecorder) ServerOption {
	return func(server *Server) {
		server.bucketRecords = recorder
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
	recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	defer func() {
		if recovered := recover(); recovered != nil {
			s.logger.Error("request panic recovered",
				"error", recovered,
				"method", r.Method,
				"path", r.URL.Path,
				"request_id", requestID,
			)
			if !recorder.wroteHeader {
				writeError(recorder, http.StatusInternalServerError, "internal_error", "request failed unexpectedly")
			}
		}

		s.logger.Info("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"bytes", recorder.bytes,
			"request_id", requestID,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	}()

	if allowedMethods, ok := allowedMethodsForPath(r.URL.Path); ok && !methodAllowed(r.Method, allowedMethods) {
		recorder.Header().Set("Allow", allowHeader(allowedMethods))
		writeError(recorder, http.StatusMethodNotAllowed, "method_not_allowed", "method is not allowed for this route")
		return
	}

	s.mux.ServeHTTP(recorder, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, requestID)))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /openapi.json", s.handleOpenAPI)
	s.mux.HandleFunc("GET /swagger", s.handleSwaggerUI)
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
	s.mux.HandleFunc("/", s.handleNotFound)
}

func (s *Server) handleNotFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "route not found")
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	if !validateAllowedQuery(w, r) {
		return
	}

	w.Header().Set("Content-Type", "application/vnd.oai.openapi+json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(openAPIJSON))
}

func (s *Server) handleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	if !validateAllowedQuery(w, r) {
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerUIHTML))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !validateAllowedQuery(w, r) {
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleHealthChecks(w http.ResponseWriter, r *http.Request) {
	if !validateAllowedQuery(w, r) {
		return
	}

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
	if !validateAllowedQuery(w, r) {
		return
	}

	writeJSON(w, http.StatusOK, s.catalogSnapshot(r.Context()))
}

func (s *Server) handleCatalogServices(w http.ResponseWriter, r *http.Request) {
	if !validateAllowedQuery(w, r, "owner", "environment") {
		return
	}

	query := r.URL.Query()
	services := filterServices(s.catalogSnapshot(r.Context()).Services, query.Get("owner"), query.Get("environment"))
	writeJSON(w, http.StatusOK, map[string][]catalog.Service{"services": services})
}

func (s *Server) handleCatalogService(w http.ResponseWriter, r *http.Request) {
	id, ok := catalogIDFromPath(w, r, "id")
	if !ok {
		return
	}

	for _, service := range s.catalogSnapshot(r.Context()).Services {
		if service.ID == id {
			writeJSON(w, http.StatusOK, service)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "catalog service not found")
}

func (s *Server) handleCatalogEnvironments(w http.ResponseWriter, r *http.Request) {
	if !validateAllowedQuery(w, r) {
		return
	}

	writeJSON(w, http.StatusOK, map[string][]catalog.Environment{"environments": s.catalogSnapshot(r.Context()).Environments})
}

func (s *Server) handleCatalogEnvironment(w http.ResponseWriter, r *http.Request) {
	id, ok := catalogIDFromPath(w, r, "id")
	if !ok {
		return
	}

	for _, environment := range s.catalogSnapshot(r.Context()).Environments {
		if environment.ID == id {
			writeJSON(w, http.StatusOK, environment)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "catalog environment not found")
}

func (s *Server) handleCatalogInfrastructure(w http.ResponseWriter, r *http.Request) {
	if !validateAllowedQuery(w, r, "environment", "type") {
		return
	}

	query := r.URL.Query()
	resources := filterInfrastructure(s.catalogSnapshot(r.Context()).Infrastructure, query.Get("environment"), query.Get("type"))
	writeJSON(w, http.StatusOK, map[string][]catalog.InfrastructureResource{"infrastructure": resources})
}

func (s *Server) handleCatalogInfrastructureResource(w http.ResponseWriter, r *http.Request) {
	id, ok := catalogIDFromPath(w, r, "id")
	if !ok {
		return
	}

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

	var request provisioner.BucketRequest
	if !decodeStrictJSONBody(w, r, &request, maxBucketRequestBytes) {
		return
	}

	if s.buckets == nil {
		writeError(w, http.StatusServiceUnavailable, "provisioner_unavailable", "bucket provisioner is not configured")
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

	if s.bucketRecords != nil {
		requestID, _ := r.Context().Value(requestIDKey{}).(string)
		recordCtx, cancel := context.WithTimeout(r.Context(), recordWriteTimeout)
		defer cancel()

		if err := s.bucketRecords.RecordBucketProvisioned(recordCtx, request, result, requestID); err != nil {
			s.logger.Error("bucket provision record write failed",
				"error", err,
				"bucket_name", result.BucketName,
				"request_id", requestID,
			)
		}
	}

	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleGitHubWebhook(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if err := requireJSONContentType(r); err != nil {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", err.Error())
		return
	}

	eventName := strings.TrimSpace(r.Header.Get("X-GitHub-Event"))
	if eventName == "" {
		writeError(w, http.StatusBadRequest, "missing_event", "X-GitHub-Event header is required")
		return
	}
	if !githubEventPattern.MatchString(eventName) {
		writeError(w, http.StatusBadRequest, "invalid_event", "X-GitHub-Event must be a valid GitHub event name")
		return
	}

	deliveryID := strings.TrimSpace(r.Header.Get("X-GitHub-Delivery"))
	if len(deliveryID) > githubDeliveryLimit {
		writeError(w, http.StatusBadRequest, "invalid_delivery", "X-GitHub-Delivery is too long")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookPayloadBytes))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "webhook payload must be 2 MiB or smaller")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_body", "webhook payload could not be read")
		return
	}

	if s.githubWebhookSecret != "" {
		signature := strings.TrimSpace(r.Header.Get("X-Hub-Signature-256"))
		if signature == "" {
			writeError(w, http.StatusUnauthorized, "missing_signature", "X-Hub-Signature-256 header is required")
			return
		}
		if !validGitHubSignature(signature, body, s.githubWebhookSecret) {
			writeError(w, http.StatusUnauthorized, "invalid_signature", "webhook signature verification failed")
			return
		}
	}

	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, "invalid_json", "webhook payload must be valid JSON")
		return
	}

	if s.githubWebhooks == nil {
		writeJSON(w, http.StatusAccepted, workflow.GitHubWebhookResult{
			Event:      eventName,
			DeliveryID: deliveryID,
			Actions:    []string{"webhook_processor_not_configured"},
		})
		return
	}

	result, err := s.githubWebhooks.ProcessGitHubWebhook(r.Context(), workflow.GitHubWebhookEvent{
		Event:      eventName,
		DeliveryID: deliveryID,
		Payload:    body,
	})
	if err != nil {
		s.logger.Error("github webhook processing failed",
			"error", err,
			"event", eventName,
			"delivery_id", deliveryID,
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

func decodeStrictJSONBody(w http.ResponseWriter, r *http.Request, target any, maxBytes int64) bool {
	if err := requireJSONContentType(r); err != nil {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", err.Error())
		return false
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body is too large")
			return false
		}
		if errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "empty_body", "request body must contain a JSON object")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be a valid JSON object")
		return false
	}

	var extra struct{}
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must contain only one JSON object")
		return false
	}

	return true
}

func requireJSONContentType(r *http.Request) error {
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if contentType == "" {
		return errors.New("Content-Type must be application/json")
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}

	return nil
}

func validateAllowedQuery(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	allowedSet := map[string]struct{}{}
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}

	fields := map[string]string{}
	for name, values := range r.URL.Query() {
		if _, ok := allowedSet[name]; !ok {
			fields[name] = "is not supported"
			continue
		}
		if len(values) > 1 {
			fields[name] = "must be provided at most once"
			continue
		}
		if len(strings.TrimSpace(values[0])) > maxQueryValueLength {
			fields[name] = "must be 128 characters or fewer"
			continue
		}
		if hasControlCharacter(values[0]) {
			fields[name] = "must not contain control characters"
		}
	}

	if len(fields) > 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error:   "validation_failed",
			Message: "request query validation failed",
			Fields:  fields,
		})
		return false
	}

	return true
}

func catalogIDFromPath(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	if !validateAllowedQuery(w, r) {
		return "", false
	}

	id := strings.TrimSpace(r.PathValue(name))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error:   "validation_failed",
			Message: "request path validation failed",
			Fields:  map[string]string{name: "is required"},
		})
		return "", false
	}
	if !catalogIDPattern.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error:   "validation_failed",
			Message: "request path validation failed",
			Fields:  map[string]string{name: "must be 1-128 characters using letters, numbers, dots, underscores, colons, or hyphens"},
		})
		return "", false
	}

	return id, true
}

func hasControlCharacter(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func allowedMethodsForPath(path string) ([]string, bool) {
	switch {
	case path == "/openapi.json",
		path == "/swagger",
		path == "/healthz",
		path == "/v1/health-checks",
		path == "/v1/catalog",
		path == "/v1/catalog/services",
		path == "/v1/catalog/environments",
		path == "/v1/catalog/infrastructure",
		isSingleSegmentChild(path, "/v1/catalog/services/"),
		isSingleSegmentChild(path, "/v1/catalog/environments/"),
		isSingleSegmentChild(path, "/v1/catalog/infrastructure/"):
		return []string{http.MethodGet}, true
	case path == "/v1/s3-buckets",
		path == "/v1/github/webhook":
		return []string{http.MethodPost}, true
	default:
		return nil, false
	}
}

func methodAllowed(method string, allowed []string) bool {
	for _, allowedMethod := range allowed {
		if method == allowedMethod || (method == http.MethodHead && allowedMethod == http.MethodGet) {
			return true
		}
	}
	return false
}

func allowHeader(allowed []string) string {
	methods := make([]string, 0, len(allowed)+1)
	for _, method := range allowed {
		methods = append(methods, method)
		if method == http.MethodGet {
			methods = append(methods, http.MethodHead)
		}
	}
	return strings.Join(methods, ", ")
}

func isSingleSegmentChild(path, prefix string) bool {
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	value := strings.TrimPrefix(path, prefix)
	return value != "" && !strings.Contains(value, "/")
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(body []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	written, err := r.ResponseWriter.Write(body)
	r.bytes += written
	return written, err
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
