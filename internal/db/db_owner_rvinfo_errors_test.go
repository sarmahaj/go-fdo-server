package db

import (
	"errors"
	"testing"
)

func setupTestDBForOwnerRv(t *testing.T) {
	t.Helper()
	_, err := InitDb("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
}

func TestInsertRvInfo_Invalid_ReturnsErrInvalidRvInfo(t *testing.T) {
	setupTestDBForOwnerRv(t)
	invalid := []byte(`[{bad}]`)
	err := InsertRvInfo(invalid)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidRvInfo) {
		t.Fatalf("expected ErrInvalidRvInfo, got %v", err)
	}
}

func TestUpdateRvInfo_Invalid_ReturnsErrInvalidRvInfo(t *testing.T) {
	setupTestDBForOwnerRv(t)
	invalid := []byte(`[{bad}]`)
	err := UpdateRvInfo(invalid)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidRvInfo) {
		t.Fatalf("expected ErrInvalidRvInfo, got %v", err)
	}
}
