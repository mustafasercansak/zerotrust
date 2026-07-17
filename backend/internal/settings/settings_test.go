package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zerotrust/backend/internal/testdb"
	"github.com/zerotrust/backend/pkg/database"
)

func TestSettingsIntegration(t *testing.T) {
	dbURL := testdb.URL(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("test db unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("test db unreachable: %v", err)
	}
	if err := database.RunMigrations(dbURL, "../../migrations"); err != nil {
		pool.Close()
		t.Fatalf("migrations failed: %v", err)
	}
	defer pool.Close()

	repo := NewRepository(pool)

	// Clean up settings table before and after tests
	pool.Exec(ctx, "DELETE FROM system_settings")
	defer pool.Exec(ctx, "DELETE FROM system_settings")

	err = repo.Set(ctx, "test_string", "hello")
	if err != nil {
		t.Fatalf("Failed to set string: %v", err)
	}

	val, err := repo.Get(ctx, "test_string")
	if err != nil || val != "hello" {
		t.Fatalf("Failed to get string, val: %s, err: %v", val, err)
	}

	err = repo.Set(ctx, "test_string", "world")
	if err != nil {
		t.Fatalf("Failed to set string: %v", err)
	}

	// Update existing test
	val, err = repo.Get(ctx, "test_string")
	if err != nil || val != "world" {
		t.Fatalf("Failed to get string, val: %s, err: %v", val, err)
	}

	// Not found
	_, err = repo.Get(ctx, "not_found")
	if err != ErrNotFound {
		t.Fatalf("Expected ErrNotFound, got %v", err)
	}

	// All
	repo.Set(ctx, "test_int", "42")
	repo.Set(ctx, "test_bool", "true")

	all, err := repo.All(ctx)
	if err != nil {
		t.Fatalf("Failed to get all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("Expected 3 items, got %v", len(all))
	}

	// Cache tests
	cache := NewCache(repo)

	// Test GetString
	if cache.GetString(ctx, "test_string", "default") != "world" {
		t.Fatal("Cache GetString failed")
	}
	if cache.GetString(ctx, "not_exist", "default") != "default" {
		t.Fatal("Cache GetString default failed")
	}

	// Test GetInt
	if cache.GetInt(ctx, "test_int", 0) != 42 {
		t.Fatal("Cache GetInt failed")
	}
	if cache.GetInt(ctx, "not_exist", 99) != 99 {
		t.Fatal("Cache GetInt default failed")
	}

	// Test GetBool
	if cache.GetBool(ctx, "test_bool", false) != true {
		t.Fatal("Cache GetBool failed")
	}
	if cache.GetBool(ctx, "not_exist", true) != true {
		t.Fatal("Cache GetBool default failed")
	}

	// Test cache TTL
	// Wait, we can't easily test TTL without sleeping, but we can verify it caches
	repo.Set(ctx, "test_int", "100") // Should still return 42 due to cache
	if cache.GetInt(ctx, "test_int", 0) != 42 {
		t.Fatal("Cache GetInt did not cache")
	}

	// Corrupted values
	repo.Set(ctx, "corrupted_int", "abc")
	if cache.GetInt(ctx, "corrupted_int", 77) != 77 {
		t.Fatal("Cache GetInt should return default on parsing error")
	}

	repo.Set(ctx, "corrupted_bool", "abc")
	if cache.GetBool(ctx, "corrupted_bool", false) != false {
		t.Fatal("Cache GetBool should return default on parsing error")
	}

	// Invalidate cache manually for coverage
	cache.mu.Lock()
	entry := cache.vals["test_int"]
	entry.expires = time.Now().Add(-time.Hour)
	cache.vals["test_int"] = entry

	entryBool := cache.vals["test_bool"]
	entryBool.expires = time.Now().Add(-time.Hour)
	cache.vals["test_bool"] = entryBool

	// Also put corrupted values in cache directly to test parsing error fallback
	cache.vals["cached_bad_int"] = cachedEntry{value: "bad", expires: time.Now().Add(time.Hour)}
	cache.vals["cached_bad_bool"] = cachedEntry{value: "bad", expires: time.Now().Add(time.Hour)}
	cache.mu.Unlock()

	if cache.GetInt(ctx, "test_int", 0) != 100 {
		t.Fatal("Cache GetInt should fetch fresh from DB after expiration")
	}

	// For bool, we didn't change DB value, but it should refresh
	if cache.GetBool(ctx, "test_bool", false) != true {
		t.Fatal("Cache GetBool should fetch fresh from DB after expiration")
	}

	// Test cached bad values
	if cache.GetInt(ctx, "cached_bad_int", 123) != 123 {
		t.Fatal("Cache GetInt should return default for cached invalid int")
	}
	if cache.GetBool(ctx, "cached_bad_bool", true) != true {
		t.Fatal("Cache GetBool should return default for cached invalid bool")
	}
}

func TestSettingsHandler(t *testing.T) {
	dbURL := testdb.URL(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("test db unavailable: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("test db unreachable: %v", err)
	}
	if err := database.RunMigrations(dbURL, "../../migrations"); err != nil {
		pool.Close()
		t.Fatalf("migrations failed: %v", err)
	}
	defer pool.Close()

	repo := NewRepository(pool)
	h := NewHandler(repo, nil)

	// Clean up settings table before and after tests
	pool.Exec(ctx, "DELETE FROM system_settings")
	defer pool.Exec(ctx, "DELETE FROM system_settings")

	// Pre-seed some data
	repo.Set(ctx, "max_sessions_per_user", "5")

	// Test List
	reqList, _ := http.NewRequest("GET", "/api/v1/admin/settings", nil)
	rr := httptest.NewRecorder()
	h.List(rr, reqList)

	if rr.Code != http.StatusOK {
		t.Fatalf("List expected 200, got %d", rr.Code)
	}

	var resp map[string]string
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["max_sessions_per_user"] != "5" {
		t.Fatalf("Expected max_sessions_per_user=5, got %v", resp["max_sessions_per_user"])
	}

	// Test Update Success
	bodySuccess := `{"password_complexity": "strong", "global_mfa_required": "true"}`
	reqUpdate, _ := http.NewRequest("PATCH", "/api/v1/admin/settings", bytes.NewBufferString(bodySuccess))
	rrUpdate := httptest.NewRecorder()
	h.Update(rrUpdate, reqUpdate)

	if rrUpdate.Code != http.StatusNoContent {
		t.Fatalf("Update expected 204, got %d", rrUpdate.Code)
	}

	val, _ := repo.Get(ctx, "password_complexity")
	if val != "strong" {
		t.Fatal("Expected password_complexity to be updated to strong")
	}

	// Test Update Invalid Key
	bodyInvalidKey := `{"unknown_key": "123"}`
	reqInvalidKey, _ := http.NewRequest("PATCH", "/api/v1/admin/settings", bytes.NewBufferString(bodyInvalidKey))
	rrInvalidKey := httptest.NewRecorder()
	h.Update(rrInvalidKey, reqInvalidKey)

	if rrInvalidKey.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for unknown key, got %d", rrInvalidKey.Code)
	}

	// Test Update Invalid Value
	bodyInvalidValue := `{"max_sessions_per_user": "999"}`
	reqInvalidValue, _ := http.NewRequest("PATCH", "/api/v1/admin/settings", bytes.NewBufferString(bodyInvalidValue))
	rrInvalidValue := httptest.NewRecorder()
	h.Update(rrInvalidValue, reqInvalidValue)

	if rrInvalidValue.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for invalid value, got %d", rrInvalidValue.Code)
	}

	// Test validators directly for coverage
	if !allowedKeys["max_sessions_per_user"]("5") || allowedKeys["max_sessions_per_user"]("0") || allowedKeys["max_sessions_per_user"]("abc") {
		t.Fatal("Validator max_sessions_per_user failed")
	}
	if !allowedKeys["session_idle_timeout_seconds"]("300") || allowedKeys["session_idle_timeout_seconds"]("10") || allowedKeys["session_idle_timeout_seconds"]("abc") {
		t.Fatal("Validator session_idle_timeout_seconds failed")
	}
	if !allowedKeys["session_idle_timeout_seconds_admin"]("300") || allowedKeys["session_idle_timeout_seconds_admin"]("10") || allowedKeys["session_idle_timeout_seconds_admin"]("abc") {
		t.Fatal("Validator session_idle_timeout_seconds_admin failed")
	}
	if !allowedKeys["session_absolute_timeout_seconds"]("28800") || allowedKeys["session_absolute_timeout_seconds"]("10") || allowedKeys["session_absolute_timeout_seconds"]("abc") {
		t.Fatal("Validator session_absolute_timeout_seconds failed")
	}
	if !allowedKeys["password_complexity"]("strong") || allowedKeys["password_complexity"]("invalid") {
		t.Fatal("Validator password_complexity failed")
	}
	if !allowedKeys["global_mfa_required"]("true") || allowedKeys["global_mfa_required"]("invalid") {
		t.Fatal("Validator global_mfa_required failed")
	}
	if !allowedKeys["max_login_attempts"]("5") || allowedKeys["max_login_attempts"]("99") || allowedKeys["max_login_attempts"]("abc") {
		t.Fatal("Validator max_login_attempts failed")
	}
	if !allowedKeys["risk_score_impossible_travel"]("80") || allowedKeys["risk_score_impossible_travel"]("-1") {
		t.Fatal("Validator risk_score_impossible_travel failed")
	}
	if !allowedKeys["risk_score_new_device"]("30") || allowedKeys["risk_score_new_device"]("101") {
		t.Fatal("Validator risk_score_new_device failed")
	}
	if !allowedKeys["risk_score_suspicious_hours"]("20") || allowedKeys["risk_score_suspicious_hours"]("-5") {
		t.Fatal("Validator risk_score_suspicious_hours failed")
	}
	if !allowedKeys["risk_score_failed_attempt"]("15") || allowedKeys["risk_score_failed_attempt"]("51") {
		t.Fatal("Validator risk_score_failed_attempt failed")
	}
	if !allowedKeys["risk_failed_attempt_max_score"]("45") || allowedKeys["risk_failed_attempt_max_score"]("101") {
		t.Fatal("Validator risk_failed_attempt_max_score failed")
	}
	if !allowedKeys["risk_suspicious_hour_start"]("23") || allowedKeys["risk_suspicious_hour_start"]("24") {
		t.Fatal("Validator risk_suspicious_hour_start failed")
	}
	if !allowedKeys["risk_suspicious_hour_end"]("5") || allowedKeys["risk_suspicious_hour_end"]("-1") {
		t.Fatal("Validator risk_suspicious_hour_end failed")
	}
	if !allowedKeys["risk_impossible_travel_velocity_kmh"]("800") || allowedKeys["risk_impossible_travel_velocity_kmh"]("50") {
		t.Fatal("Validator risk_impossible_travel_velocity_kmh failed")
	}
	if !allowedKeys["risk_impossible_travel_window_hours"]("24") || allowedKeys["risk_impossible_travel_window_hours"]("0") {
		t.Fatal("Validator risk_impossible_travel_window_hours failed")
	}
	if !allowedKeys["risk_impossible_travel_min_distance_km"]("10") || allowedKeys["risk_impossible_travel_min_distance_km"]("0") {
		t.Fatal("Validator risk_impossible_travel_min_distance_km failed")
	}

	// Test update bad json
	reqBadJson, _ := http.NewRequest("PATCH", "/api/v1/admin/settings", bytes.NewBufferString("{bad"))
	rrBadJson := httptest.NewRecorder()
	h.Update(rrBadJson, reqBadJson)
	if rrBadJson.Code != http.StatusBadRequest {
		t.Fatal("Expected 400 for bad JSON")
	}
}
