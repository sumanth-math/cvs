package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/your-org/platform-service/internal/provisioner"
)

type BucketProvisioner interface {
	ProvisionBucket(context.Context, provisioner.BucketRequest) (provisioner.BucketResult, error)
}

type Server struct {
	buckets BucketProvisioner
	logger  *slog.Logger
	mux     *http.ServeMux
}

func NewServer(buckets BucketProvisioner, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}

	server := &Server{
		buckets: buckets,
		logger:  logger,
		mux:     http.NewServeMux(),
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
	s.mux.HandleFunc("POST /v1/s3-buckets", s.handleCreateBucket)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
