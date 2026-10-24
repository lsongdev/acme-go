package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// Handler returns the JSON API. Routes are relative to where the handler is
// mounted; strip the prefix with http.StripPrefix.
//
// Successful responses are {"data": ...}; failures are
// {"error": {"code": ..., "message": ...}} with a matching status code.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /providers", s.handleListProviders)
	mux.HandleFunc("POST /providers", s.handleCreateProvider)
	mux.HandleFunc("GET /providers/{id}/directory", s.handleProviderDirectory)
	mux.HandleFunc("DELETE /providers/{id}", s.handleDeleteProvider)

	mux.HandleFunc("GET /accounts", s.handleListAccounts)
	mux.HandleFunc("POST /accounts", s.handleRegisterAccount)
	mux.HandleFunc("GET /accounts/{id}", s.handleGetAccount)
	mux.HandleFunc("POST /accounts/{id}/deactivate", s.handleDeactivateAccount)
	mux.HandleFunc("DELETE /accounts/{id}", s.handleDeleteAccount)

	mux.HandleFunc("GET /certificates", s.handleListCertificates)
	mux.HandleFunc("POST /certificates", s.handleCreateCertificate)
	mux.HandleFunc("GET /certificates/{id}", s.handleGetCertificate)
	mux.HandleFunc("PATCH /certificates/{id}", s.handleUpdateCertificate)
	mux.HandleFunc("DELETE /certificates/{id}", s.handleDeleteCertificate)
	mux.HandleFunc("POST /certificates/{id}/renew", s.handleRenewCertificate)
	mux.HandleFunc("POST /certificates/{id}/revoke", s.handleRevokeCertificate)
	mux.HandleFunc("GET /certificates/{id}/download", s.handleDownloadCertificate)

	mux.HandleFunc("GET /issuances", s.handleListIssuances)
	mux.HandleFunc("GET /issuances/{id}", s.handleGetIssuance)
	mux.HandleFunc("POST /issuances/{id}/continue", s.handleContinueIssuance)
	mux.HandleFunc("POST /issuances/{id}/cancel", s.handleCancelIssuance)

	mux.HandleFunc("GET /orders", s.handleListOrders)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such API endpoint")
	})
	return mux
}

func (s *Service) handleListProviders(w http.ResponseWriter, r *http.Request) {
	items, err := s.ListProviders(r.Context())
	s.respond(w, http.StatusOK, items, err)
}

func (s *Service) handleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var request CreateProviderRequest
	if !decode(w, r, &request) {
		return
	}
	item, err := s.CreateProvider(r.Context(), request)
	s.respond(w, http.StatusCreated, item, err)
}

func (s *Service) handleProviderDirectory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := s.ProviderDirectory(r.Context(), id)
	s.respond(w, http.StatusOK, item, err)
}

func (s *Service) handleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.respond(w, http.StatusNoContent, nil, s.DeleteProvider(r.Context(), id))
}

func (s *Service) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	items, err := s.ListAccounts(r.Context())
	s.respond(w, http.StatusOK, items, err)
}

func (s *Service) handleRegisterAccount(w http.ResponseWriter, r *http.Request) {
	var request RegisterAccountRequest
	if !decode(w, r, &request) {
		return
	}
	item, err := s.RegisterAccount(r.Context(), request)
	s.respond(w, http.StatusCreated, item, err)
}

func (s *Service) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := s.GetAccount(r.Context(), id)
	s.respond(w, http.StatusOK, item, err)
}

func (s *Service) handleDeactivateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := s.DeactivateAccount(r.Context(), id)
	s.respond(w, http.StatusOK, item, err)
}

func (s *Service) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.respond(w, http.StatusNoContent, nil, s.DeleteAccount(r.Context(), id))
}

func (s *Service) handleListCertificates(w http.ResponseWriter, r *http.Request) {
	items, err := s.ListCertificates(r.Context())
	for i := range items {
		items[i].CertPEM = "" // keep the list small; fetch one for its PEM
	}
	s.respond(w, http.StatusOK, items, err)
}

func (s *Service) handleCreateCertificate(w http.ResponseWriter, r *http.Request) {
	var request CreateCertificateRequest
	if !decode(w, r, &request) {
		return
	}
	certificate, issuance, err := s.CreateCertificate(r.Context(), request)
	s.respond(w, http.StatusAccepted, map[string]any{"certificate": certificate, "issuance": issuance}, err)
}

func (s *Service) handleGetCertificate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := s.GetCertificate(r.Context(), id)
	s.respond(w, http.StatusOK, item, err)
}

func (s *Service) handleUpdateCertificate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var request UpdateCertificateRequest
	if !decode(w, r, &request) {
		return
	}
	item, err := s.UpdateCertificate(r.Context(), id, request)
	s.respond(w, http.StatusOK, item, err)
}

func (s *Service) handleDeleteCertificate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.respond(w, http.StatusNoContent, nil, s.DeleteCertificate(r.Context(), id))
}

func (s *Service) handleRenewCertificate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := s.RenewCertificate(r.Context(), id)
	s.respond(w, http.StatusAccepted, item, err)
}

func (s *Service) handleRevokeCertificate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var request struct {
		Reason int `json:"reason"`
	}
	if !decode(w, r, &request) {
		return
	}
	item, err := s.RevokeCertificate(r.Context(), id, request.Reason)
	s.respond(w, http.StatusOK, item, err)
}

// handleDownloadCertificate serves the certificate as PEM files. The format
// query parameter selects fullchain.pem, privkey.pem or (by default) a zip
// archive with both.
func (s *Service) handleDownloadCertificate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	certificate, err := s.GetCertificate(r.Context(), id)
	if err != nil {
		s.respond(w, 0, nil, err)
		return
	}
	if certificate.CertPEM == "" {
		writeError(w, http.StatusConflict, "conflict", "the certificate has not been issued yet")
		return
	}
	name := strings.ReplaceAll(certificate.Domains[0], "*", "_wildcard")
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Query().Get("format") {
	case "fullchain":
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.fullchain.pem"`, name))
		_, _ = io.WriteString(w, certificate.CertPEM)
	case "key":
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.privkey.pem"`, name))
		_, _ = io.WriteString(w, certificate.KeyPEM)
	case "", "zip":
		var buffer bytes.Buffer
		archive := zip.NewWriter(&buffer)
		for file, content := range map[string]string{"fullchain.pem": certificate.CertPEM, "privkey.pem": certificate.KeyPEM} {
			f, err := archive.Create(name + "/" + file)
			if err == nil {
				_, err = io.WriteString(f, content)
			}
			if err != nil {
				s.respond(w, 0, nil, err)
				return
			}
		}
		if err := archive.Close(); err != nil {
			s.respond(w, 0, nil, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, name))
		_, _ = w.Write(buffer.Bytes())
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "format must be zip, fullchain or key")
	}
}

func (s *Service) handleListIssuances(w http.ResponseWriter, r *http.Request) {
	var certificateID int64
	if value := r.URL.Query().Get("certificate_id"); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id < 1 {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid certificate_id")
			return
		}
		certificateID = id
	}
	items, err := s.ListIssuances(r.Context(), certificateID)
	s.respond(w, http.StatusOK, items, err)
}

func (s *Service) handleGetIssuance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := s.GetIssuance(r.Context(), id)
	s.respond(w, http.StatusOK, item, err)
}

func (s *Service) handleContinueIssuance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var request struct {
		Force bool `json:"force"`
	}
	if r.ContentLength != 0 && !decode(w, r, &request) {
		return
	}
	s.respond(w, http.StatusAccepted, nil, s.ContinueIssuance(r.Context(), id, request.Force))
}

func (s *Service) handleCancelIssuance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	s.respond(w, http.StatusAccepted, nil, s.CancelIssuance(r.Context(), id))
}

func (s *Service) handleListOrders(w http.ResponseWriter, r *http.Request) {
	items, err := s.ListOrders(r.Context())
	s.respond(w, http.StatusOK, items, err)
}

// respond writes data with status, or maps err to an error response.
func (s *Service) respond(w http.ResponseWriter, status int, data any, err error) {
	switch {
	case err == nil:
	case errors.Is(err, ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, "invalid_request", err.Error())
		return
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	default:
		// Most remaining errors come from the CA and are worth showing.
		s.log.Error("API request failed", slog.Any("error", err))
		writeError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if status == http.StatusNoContent || data == nil {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, map[string]any{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid id")
		return 0, false
	}
	return id, true
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}
