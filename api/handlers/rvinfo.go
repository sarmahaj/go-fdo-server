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
		case http.MethodDelete:
			deleteRvInfo(w, r, rvInfoState)
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

	// V1 API: PUT acts as upsert (create OR update)
	err = rvInfoState.UpdateRvInfoV1(r.Context(), rvInfo)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		// Record doesn't exist, try to insert instead
		slog.Debug("rvInfo does not exist, creating new")
		err = rvInfoState.InsertRvInfoV1(r.Context(), rvInfo)
	}

	if err != nil {
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
		slog.Error("Error upserting rvInfo", "error", err)
		http.Error(w, "Error upserting rvInfo", http.StatusInternalServerError)
		return
	}

	slog.Debug("rvInfo upserted")

	w.Header().Set("Content-Type", "application/json")
	writeResponse(w, rvInfo)
}

func deleteRvInfo(w http.ResponseWriter, r *http.Request, rvInfoState *state.RvInfoState) {
	slog.Warn("V1 API /api/v1/rvinfo is deprecated and will be removed in a future release. Please migrate to /api/v2/rvinfo")

	// Get current config before deleting (to return it)
	rvInfoJSON, err := rvInfoState.FetchRvInfoJSON(r.Context())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("No rvInfo found to delete")
			http.Error(w, "No rvInfo found", http.StatusNotFound)
			return
		}
		slog.Error("Error fetching rvInfo", "error", err)
		http.Error(w, "Error fetching rvInfo", http.StatusInternalServerError)
		return
	}

	// Delete the config
	if err := rvInfoState.DeleteRvInfo(r.Context()); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("No rvInfo found to delete")
			http.Error(w, "No rvInfo found", http.StatusNotFound)
			return
		}
		slog.Error("Error deleting rvInfo", "error", err)
		http.Error(w, "Error deleting rvInfo", http.StatusInternalServerError)
		return
	}

	slog.Debug("rvInfo deleted")

	// Return the deleted config
	w.Header().Set("Content-Type", "application/json")
	writeResponse(w, rvInfoJSON)
}
