package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// LogEntry represents a single log line from an app.
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Source    string `json:"source,omitempty"`
}

// LogStreamer defines the interface for reading application logs.
type LogStreamer interface {
	// StreamLogs returns a channel of log entries for the given app.
	// If follow is true the channel stays open until the context is cancelled.
	// tail specifies the number of most recent log lines to return initially.
	StreamLogs(ctx context.Context, namespace, appName string, follow bool, tail int) (<-chan LogEntry, error)
}

// AppResolver maps an app ID to its Kubernetes namespace and app name.
type AppResolver interface {
	ResolveApp(ctx context.Context, appID string) (namespace, appName string, err error)
}

// LogHandler holds HTTP handlers for log streaming endpoints.
type LogHandler struct {
	streamer LogStreamer
	resolver AppResolver
}

// NewLogHandler creates a new LogHandler.
func NewLogHandler(streamer LogStreamer, resolver AppResolver) *LogHandler {
	return &LogHandler{
		streamer: streamer,
		resolver: resolver,
	}
}

// HandleGetLogs handles GET /api/v1/tenants/{tid}/apps/{appID}/logs.
// It supports Server-Sent Events (SSE) for real-time log streaming.
// Query parameters:
//
//	follow=true  - keep the connection open and stream new logs
//	tail=100     - number of historical log lines to return
func (h *LogHandler) HandleGetLogs(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "appID")
	if appID == "" {
		respondError(w, http.StatusBadRequest, "missing app ID")
		return
	}

	namespace, appName, err := h.resolver.ResolveApp(r.Context(), appID)
	if err != nil {
		respondError(w, http.StatusNotFound, "app not found")
		return
	}

	follow := r.URL.Query().Get("follow") == "true"
	tail := 100
	if t := r.URL.Query().Get("tail"); t != "" {
		if parsed, err := strconv.Atoi(t); err == nil && parsed > 0 {
			tail = parsed
		}
	}

	logCh, err := h.streamer.StreamLogs(r.Context(), namespace, appName, follow, tail)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to stream logs")
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		respondError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-logCh:
			if !ok {
				// Channel closed; send a final event and return.
				_, _ = fmt.Fprintf(w, "event: done\ndata: stream closed\n\n")
				flusher.Flush()
				return
			}
			_, _ = fmt.Fprintf(w, "data: [%s] %s\n\n", entry.Timestamp, entry.Message)
			flusher.Flush()
		}
	}
}
