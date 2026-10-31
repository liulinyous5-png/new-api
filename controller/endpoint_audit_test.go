/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/endpointaudit"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEndpointAuditRejectsUnsafeURLs(t *testing.T) {
	ctx := t.Context()
	for _, raw := range []string{
		"ftp://1.1.1.1/v1/messages",
		"https://user:pass@1.1.1.1/v1/messages",
	} {
		_, err := endpointaudit.NormalizeMessagesURL(ctx, raw)
		assert.Error(t, err, raw)
	}
	for _, raw := range []string{
		"https://127.0.0.1/v1/messages",
		"http://10.1.1.1/v1/messages",
		"http://localhost/v1/messages",
		"https://192.168.1.1",
		"https://198.18.4.174",
	} {
		parsed, err := endpointaudit.NormalizeMessagesURL(ctx, raw)
		require.NoError(t, err, raw)
		assert.Equal(t, "/v1/messages", parsed.Path, raw)
	}
	parsed, err := endpointaudit.NormalizeMessagesURL(ctx, "https://169.254.169.254/latest")
	require.NoError(t, err)
	assert.Equal(t, "/latest/v1/messages", parsed.Path)
	parsed, err = endpointaudit.NormalizeMessagesURL(ctx, "http://1.1.1.1")
	require.NoError(t, err)
	assert.Equal(t, "http", parsed.Scheme)
	assert.Equal(t, "/v1/messages", parsed.Path)
	parsed, err = endpointaudit.NormalizeMessagesURL(ctx, "https://1.1.1.1")
	require.NoError(t, err)
	assert.Equal(t, "/v1/messages", parsed.Path)
	parsed, err = endpointaudit.NormalizeMessagesURL(ctx, "https://1.1.1.1/v1")
	require.NoError(t, err)
	assert.Equal(t, "/v1/messages", parsed.Path)
	parsed, err = endpointaudit.NormalizeMessagesURL(ctx, "https://1.1.1.1/v1/messages")
	require.NoError(t, err)
	assert.Equal(t, "/v1/messages", parsed.Path)
}

func TestEndpointAuditAssertionsAndStoredKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/messages", r.URL.Path)
		require.Equal(t, "sk-test-secret-value", r.Header.Get("x-api-key"))
		require.Equal(t, "context-management-2025-06-27", r.Header.Get("anthropic-beta"))
		require.NotContains(t, r.URL.RawQuery, "sk-test-secret-value")
		payload, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.NotContains(t, string(payload), "stream")
		assert.Contains(t, string(payload), "claude-opus-4-6")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"context_management":{"applied_edits":[{"type":"clear_tool_uses_20250919","cleared_tool_uses":3}]},"usage":{"input_tokens":8,"note":"sk-test-secret-value"}}`))
	}))
	t.Cleanup(server.Close)

	client := endpointaudit.NewSafeClient()
	transport := client.Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	client.Transport = transport
	restoreClient := endpointaudit.SetClientForTest(client)
	t.Cleanup(restoreClient)

	suiteID := openEndpointAuditDB(t)
	engine := gin.New()
	engine.GET("/tests", ListEndpointAuditRuns)
	engine.POST("/tests", StartEndpointAudit)
	engine.GET("/tests/:id", GetEndpointAuditRun)

	body := []byte(`{"base_url":"` + server.URL + `","api_key":"sk-test-secret-value","suite_id":` + strconv.Itoa(suiteID) + `}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/tests", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "sk-test-secret-value")
	var started struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &started))
	require.NotZero(t, started.Data.ID)

	var result string
	require.Eventually(t, func() bool {
		got := httptest.NewRecorder()
		engine.ServeHTTP(got, httptest.NewRequest(http.MethodGet, "/tests/"+strconv.Itoa(started.Data.ID), nil))
		if got.Code != http.StatusOK {
			return false
		}
		result = got.Body.String()
		return strings.Contains(result, `"verdict":"passed"`)
	}, 5*time.Second, 20*time.Millisecond)
	assert.NotContains(t, result, "sk-test-secret-value")
	assert.Contains(t, result, "[redacted]")
	listed := httptest.NewRecorder()
	engine.ServeHTTP(listed, httptest.NewRequest(http.MethodGet, "/tests", nil))
	require.Equal(t, http.StatusOK, listed.Code)
	assert.JSONEq(t, `{"success":true,"message":"","data":[]}`, listed.Body.String())
	assert.Contains(t, result, "clear tool uses applied")
	assert.Contains(t, result, "cleared_tool_uses")
	assert.NotContains(t, result, "sk-test")

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/tests", bytes.NewReader([]byte(`{"base_url":"ftp://1.1.1.1","api_key":"sk-test-secret-value","suite_id":`+strconv.Itoa(suiteID)+`}`)))
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	local := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/tests", bytes.NewReader([]byte(`{"base_url":"https://127.0.0.1","api_key":"sk-test-secret-value","suite_id":`+strconv.Itoa(suiteID)+`}`)))
	engine.ServeHTTP(local, request)
	assert.Equal(t, http.StatusOK, local.Code)
	assert.NotContains(t, local.Body.String(), "sk-test-secret-value")
}

func openEndpointAuditDB(t *testing.T) int {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	require.NoError(t, db.AutoMigrate(
		&model.ChannelQualityProbe{},
		&model.ChannelQualitySuite{},
		&model.ChannelQualitySuiteItem{},
	))
	probe := &model.ChannelQualityProbe{
		Name:       "clear_tool_uses",
		Headers:    `{"anthropic-beta":"context-management-2025-06-27"}`,
		Body:       `{"model":"{{model}}","stream":true,"messages":[{"role":"user","content":"Reply with ok"}]}`,
		Statuses:   `[200]`,
		Assertions: `[{"name":"clear tool uses applied","expr":"int(usage.input_tokens) == 8"}]`,
	}
	require.NoError(t, db.Create(probe).Error)
	suite := &model.ChannelQualitySuite{Name: "check", CreatedAt: 1, UpdatedAt: 1}
	require.NoError(t, db.Create(suite).Error)
	require.NoError(t, db.Create(&model.ChannelQualitySuiteItem{
		SuiteId: suite.Id, ProbeId: probe.Id, Model: "claude-opus-4-6", Sort: 0,
	}).Error)
	return suite.Id
}
