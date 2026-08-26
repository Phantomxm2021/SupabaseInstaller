package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"supabase-manager/apps/manager/internal/operation"
	"supabase-manager/internal/contracts"
)

func RegisterOperationRoutes(mux *http.ServeMux, operations *operation.Service) {
	mux.HandleFunc("GET /api/operations/{id}", func(response http.ResponseWriter, request *http.Request) {
		found, err := operations.Get(request.Context(), request.PathValue("id"))
		if err != nil {
			writeError(response, http.StatusNotFound, "OPERATION_NOT_FOUND", "Operation was not found")
			return
		}
		writeJSON(response, http.StatusOK, found)
	})
	mux.HandleFunc("GET /api/operations/{id}/events", operationEvents(operations))
}

func operationEvents(operations *operation.Service) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		flusher, ok := response.(http.Flusher)
		if !ok {
			writeError(response, http.StatusInternalServerError, "STREAM_UNSUPPORTED", "Event streaming is unavailable")
			return
		}
		lastSequence, _ := strconv.ParseInt(request.Header.Get("Last-Event-ID"), 10, 64)
		response.Header().Set("Content-Type", "text/event-stream")
		response.Header().Set("Cache-Control", "no-cache")
		response.Header().Set("X-Accel-Buffering", "no")
		response.WriteHeader(http.StatusOK)
		poll := time.NewTicker(500 * time.Millisecond)
		defer poll.Stop()
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		for {
			events, err := operations.EventsAfter(request.Context(), request.PathValue("id"), lastSequence)
			if err != nil {
				_, _ = fmt.Fprintf(response, "event: error\ndata: {\"code\":\"EVENT_READ_FAILED\"}\n\n")
				flusher.Flush()
				return
			}
			for _, event := range events {
				_, _ = fmt.Fprintf(response, "id: %d\nevent: %s\ndata: %s\n\n", event.Sequence, event.Type, event.Payload)
				lastSequence = event.Sequence
			}
			if len(events) > 0 {
				flusher.Flush()
			}
			current, err := operations.Get(request.Context(), request.PathValue("id"))
			if err != nil || terminalOperation(current.Status) {
				return
			}
			select {
			case <-request.Context().Done():
				return
			case <-poll.C:
			case <-heartbeat.C:
				_, _ = fmt.Fprint(response, ": heartbeat\n\n")
				flusher.Flush()
			}
		}
	}
}

func terminalOperation(status contracts.OperationStatus) bool {
	switch status {
	case contracts.OperationSucceeded, contracts.OperationFailed, contracts.OperationRolledBack, contracts.OperationCancelled:
		return true
	default:
		return false
	}
}
