package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type customNodeRecordingStore struct {
	*fakeStore
	inserted int
	updated  []int64
	deleted  []int64
}

func (s *customNodeRecordingStore) InsertMap(_ context.Context, table string, _ map[string]any) (int64, error) {
	if table == "custom_node" {
		s.inserted++
	}
	return int64(100 + s.inserted), nil
}

func (s *customNodeRecordingStore) UpdateMap(_ context.Context, table string, id int64, _ map[string]any) error {
	if table == "custom_node" {
		s.updated = append(s.updated, id)
	}
	return nil
}

func (s *customNodeRecordingStore) DeleteByID(_ context.Context, table string, id int64) error {
	if table == "custom_node" {
		s.deleted = append(s.deleted, id)
	}
	return nil
}

func adminRequest(t *testing.T, handler http.Handler, tokens tokenGenerator, method, path, body string) Response {
	t.Helper()
	token, err := tokens.Generate(1, "root", 0)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Authorization", token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var envelope Response
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response for %s: %v; body=%s", path, err, response.Body.String())
	}
	return envelope
}

type tokenGenerator interface {
	Generate(int64, string, int) (string, error)
}

func TestCustomNodeIDsPreserveInt64AndRemoveDuplicates(t *testing.T) {
	const snowflake = "9223372036854775000"
	ids, err := customNodeIDs(map[string]any{"ids": []any{snowflake, snowflake, "42"}})
	if err != nil {
		t.Fatalf("parse custom node IDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != 9223372036854775000 || ids[1] != 42 {
		t.Fatalf("unexpected IDs: %#v", ids)
	}
}

func TestCustomNodeIDsRejectInvalidInput(t *testing.T) {
	for _, body := range []map[string]any{
		{},
		{"ids": []any{}},
		{"ids": []any{"not-an-id"}},
		{"ids": []any{"0"}},
	} {
		if _, err := customNodeIDs(body); err == nil {
			t.Fatalf("expected invalid input to fail: %#v", body)
		}
	}
}

func TestCustomNodeBatchRoutes(t *testing.T) {
	dataStore := &customNodeRecordingStore{fakeStore: &fakeStore{configs: map[string]string{}}}
	handler, tokens := testRouter(dataStore)

	imported := adminRequest(t, handler, tokens, http.MethodPost, "/api/v1/custom-nodes", `{"links":["vless://uuid@one.example:443?security=tls#one","trojan://secret@two.example:443?sni=two.example#two"],"visibility":"global"}`)
	if imported.Code != 0 || dataStore.inserted != 2 {
		t.Fatalf("batch import failed: response=%+v inserted=%d", imported, dataStore.inserted)
	}

	updated := adminRequest(t, handler, tokens, http.MethodPost, "/api/v1/custom-nodes/batch/status", `{"ids":["9223372036854775000","42"],"status":0}`)
	if updated.Code != 0 || len(dataStore.updated) != 2 || dataStore.updated[0] != 9223372036854775000 {
		t.Fatalf("batch status failed: response=%+v updated=%#v", updated, dataStore.updated)
	}

	deleted := adminRequest(t, handler, tokens, http.MethodPost, "/api/v1/custom-nodes/batch/delete", `{"ids":["9223372036854775000","42"]}`)
	if deleted.Code != 0 || len(dataStore.deleted) != 2 || dataStore.deleted[0] != 9223372036854775000 {
		t.Fatalf("batch delete failed: response=%+v deleted=%#v", deleted, dataStore.deleted)
	}
}
