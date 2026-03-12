// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package handlers

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/fido-device-onboard/go-fdo-server/internal/state"
	"gorm.io/gorm"
)

func RvInfoHandler(rvInfoState *state.RvInfoState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("Received RV request", "method", r.Method, "path", r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			getRvInfo(w, r, rvInfoState)
		case http.MethodPost:
			createRvInfo(w, r, rvInfoState)
		case http.MethodPut:
			updateRvInfo(w, r, rvInfoState)
		default:
			slog.Error("Method not allowed", "method", r.Method, "path", r.URL.Path)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func getRvInfo(w http.ResponseWriter, r *http.Request, rvInfoState *state.RvInfoState) {
	slog.Warn("V1 API /api/v1/rvinfo is deprecated and will be removed in a future release. Please migrate to /api/v2/rvinfo")
	slog.Debug("Fetching rvInfo")
	rvInfoJSON, err := rvInfoState.FetchRvInfoJSON(r.Context())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Return empty array for backward compatibility
			slog.Debug("No rvInfo found, returning empty array")
			w.Header().Set("Content-Type", "application/json")
			writeResponse(w, []byte("[]"))
			return
		} else {
			slog.Error("Error fetching rvInfo", "error", err)
			http.Error(w, "Error fetching rvInfo", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	writeResponse(w, rvInfoJSON)
}

func createRvInfo(w http.ResponseWriter, r *http.Request, rvInfoState *state.RvInfoState) {
	slog.Warn("V1 API /api/v1/rvinfo is deprecated and will be removed in a future release. Please migrate to /api/v2/rvinfo")
	rvInfo, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if err := rvInfoState.InsertRvInfoV1(r.Context(), rvInfo); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			slog.Error("rvInfo already exists (constraint)", "error", err)
			http.Error(w, "rvInfo already exists", http.StatusConflict)
			return
		}
		if errors.Is(err, state.ErrInvalidRvInfo) {
			slog.Error("Invalid rvInfo payload", "error", err)
			http.Error(w, "Invalid rvInfo", http.StatusBadRequest)
			return
		}
		slog.Error("Error inserting rvInfo", "error", err)
		http.Error(w, "Error inserting rvInfo", http.StatusInternalServerError)
		return
	}

	slog.Debug("rvInfo created")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	writeResponse(w, rvInfo)
}

func updateRvInfo(w http.ResponseWriter, r *http.Request, rvInfoState *state.RvInfoState) {
	slog.Warn("V1 API /api/v1/rvinfo is deprecated and will be removed in a future release. Please migrate to /api/v2/rvinfo")
	rvInfo, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Error reading body", "error", err)
		http.Error(w, "Error reading body", http.StatusInternalServerError)
		return
	}

	if err := rvInfoState.UpdateRvInfoV1(r.Context(), rvInfo); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("rvInfo does not exist, cannot update")
			http.Error(w, "rvInfo does not exist", http.StatusNotFound)
			return
		}
		if errors.Is(err, state.ErrInvalidRvInfo) {
			slog.Error("Invalid rvInfo payload", "error", err)
			http.Error(w, "Invalid rvInfo", http.StatusBadRequest)
			return
		}
		slog.Error("Error updating rvInfo", "error", err)
		http.Error(w, "Error updating rvInfo", http.StatusInternalServerError)
		return
	}

	slog.Debug("rvInfo updated")

	w.Header().Set("Content-Type", "application/json")
	writeResponse(w, rvInfo)
}
