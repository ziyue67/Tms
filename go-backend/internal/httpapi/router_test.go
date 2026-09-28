package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ziyue67/tms/go-backend/internal/auth"
	"github.com/ziyue67/tms/go-backend/internal/config"
	"github.com/ziyue67/tms/go-backend/internal/store"
)

type fakeStore struct {
	user             store.User
	configs          map[string]string
	updatedPassword  string
	updatedConfigMap map[string]string
}

func (s *fakeStore) Ping(context.Context) error { return nil }

func (s *fakeStore) UserByLogin(_ context.Context, login string) (store.User, error) {
	if login == s.user.Username || (s.user.Email.Valid && login == s.user.Email.String) {
		return s.user, nil
	}
	return store.User{}, sql.ErrNoRows
}

func (s *fakeStore) UpdatePassword(_ context.Context, _ int64, encoded string) error {
	s.updatedPassword = encoded
	return nil
}

func (s *fakeStore) ConfigValue(_ context.Context, name string) (store.SiteConfig, error) {
	value, exists := s.configs[name]
	if !exists {
		return store.SiteConfig{}, sql.ErrNoRows
	}
	return store.SiteConfig{ID: 1, Name: name, Value: value, Time: 1}, nil
}

func (s *fakeStore) Configs(context.Context) ([]store.SiteConfig, error) {
	items := make([]store.SiteConfig, 0, len(s.configs))
	for name, value := range s.configs {
		items = append(items, store.SiteConfig{Name: name, Value: value})
	}
	return items, nil
}

func (s *fakeStore) UpsertConfigs(_ context.Context, values map[string]string) error {
	if values == nil {
		return errors.New("missing values")
	}
	s.updatedConfigMap = values
	return nil
}

func testRouter(dataStore DataStore) (http.Handler, *auth.TokenService) {
	tokens := auth.NewTokenService("test-secret", time.Hour)
	return New(Dependencies{
		Config: config.Config{JWTSecret: "test-secret"},
		Store:  dataStore,
		Tokens: tokens,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}), tokens
}

func TestLegacyLivenessContract(t *testing.T) {
	handler, _ := testRouter(&fakeStore{configs: map[string]string{}})
	request := httptest.NewRequest(http.MethodGet, "/flow/test", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != "test" {
		t.Fatalf("unexpected liveness response: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestLoginPreservesEnvelopeAndUpgradesLegacyPassword(t *testing.T) {
	dataStore := &fakeStore{
		user:    store.User{ID: 1, Username: "admin_user", Password: "3c85cdebade1c51cf64ca9f3c09d182d", RoleID: 0, Status: 1},
		configs: map[string]string{},
	}
	handler, _ := testRouter(dataStore)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/user/login", bytes.NewBufferString(`{"username":"admin_user","password":"admin_user"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	var body struct {
		Code int `json:"code"`
		Data struct {
			Token                 string `json:"token"`
			Name                  string `json:"name"`
			RoleID                int    `json:"role_id"`
			RequirePasswordChange bool   `json:"requirePasswordChange"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 0 || body.Data.Token == "" || body.Data.Name != "admin_user" || body.Data.RoleID != 0 || !body.Data.RequirePasswordChange {
		t.Fatalf("unexpected login response: %+v", body)
	}
	if dataStore.updatedPassword == "" || dataStore.updatedPassword == dataStore.user.Password {
		t.Fatal("legacy password was not upgraded to bcrypt")
	}
}

func TestAdminConfigRequiresAdminRole(t *testing.T) {
	dataStore := &fakeStore{configs: map[string]string{"smtp_password": "secret"}}
	handler, tokens := testRouter(dataStore)
	userToken, err := tokens.Generate(2, "user", 1)
	if err != nil {
		t.Fatalf("generate user token: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/config/private-list", nil)
	request.Header.Set("Authorization", userToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	var body Response
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != 403 {
		t.Fatalf("expected role error, got %+v", body)
	}
}

func TestAdminCanUpdateSingleConfig(t *testing.T) {
	dataStore := &fakeStore{configs: map[string]string{}}
	handler, tokens := testRouter(dataStore)
	adminToken, err := tokens.Generate(1, "admin", 0)
	if err != nil {
		t.Fatalf("generate admin token: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/config/update-single", bytes.NewBufferString(`{"name":"app_name","value":"TMS Go"}`))
	request.Header.Set("Authorization", adminToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if dataStore.updatedConfigMap["app_name"] != "TMS Go" {
		t.Fatalf("configuration was not updated: %#v", dataStore.updatedConfigMap)
	}
}
