// Package server exposes the deliberately small authenticated API of the
// native Nginx proxy agent. It never accepts raw Nginx configuration.
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"supabase-manager/apps/nginxproxy/internal/site"
)

const maxRequestBytes = 64 << 10

type siteStore interface {
	Apply(context.Context, site.RenderedSite) error
	Remove(context.Context, string) error
}

type Handler struct {
	token    string
	renderer site.Renderer
	store    siteStore
}

func New(token string, renderer site.Renderer, store siteStore) *Handler {
	return &Handler{token: token, renderer: renderer, store: store}
}

func (h *Handler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodGet && request.URL.Path == "/healthz" {
		response.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodPost {
		writeError(response, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !h.authorized(request) {
		writeError(response, http.StatusUnauthorized, "unauthorized")
		return
	}

	switch request.URL.Path {
	case "/v1/sites/apply":
		h.apply(response, request)
	case "/v1/sites/remove":
		h.remove(response, request)
	default:
		writeError(response, http.StatusNotFound, "not found")
	}
}

func (h *Handler) authorized(request *http.Request) bool {
	provided := request.Header.Get("Authorization")
	expected := "Bearer " + h.token
	if h.token == "" || len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (h *Handler) apply(response http.ResponseWriter, request *http.Request) {
	var input site.ApplyRequest
	if err := decodeJSON(response, request, &input); err != nil {
		return
	}
	rendered, err := h.renderer.RenderApply(input)
	if err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.Apply(request.Context(), rendered); err != nil {
		writeError(response, http.StatusInternalServerError, fmt.Sprintf("apply site: %v", err))
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func (h *Handler) remove(response http.ResponseWriter, request *http.Request) {
	var input struct {
		Slug string `json:"slug"`
	}
	if err := decodeJSON(response, request, &input); err != nil {
		return
	}
	availableName, err := site.ManagedSiteName(input.Slug)
	if err != nil {
		writeError(response, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.Remove(request.Context(), availableName); err != nil {
		writeError(response, http.StatusInternalServerError, fmt.Sprintf("remove site: %v", err))
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

func decodeJSON(response http.ResponseWriter, request *http.Request, destination any) error {
	request.Body = http.MaxBytesReader(response, request.Body, maxRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(response, http.StatusBadRequest, "invalid JSON request")
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(response, http.StatusBadRequest, "request must contain one JSON object")
		return err
	}
	return nil
}

func writeError(response http.ResponseWriter, status int, message string) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(map[string]string{"error": message})
}
