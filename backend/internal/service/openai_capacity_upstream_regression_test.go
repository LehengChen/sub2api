package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIHTTPCapacityShedIsRequestScopedForOAuthAccounts(t *testing.T) {
	payload := []byte(`{"error":{"type":"server_error","message":"Our servers are currently overloaded. Please try again later."}}`)
	failoverErr := newOpenAIUpstreamFailoverError(
		http.StatusBadRequest,
		http.Header{"X-Request-Id": []string{"rid-http-capacity"}},
		payload,
		"Our servers are currently overloaded. Please try again later.",
		false,
	)

	require.True(t, failoverErr.RetryableOnSameAccount)
	require.True(t, failoverErr.RequestScopedTransient)

	repo := &capacityShedAccountRepoStub{}
	(&GatewayService{accountRepo: repo}).TempUnscheduleRetryableError(context.Background(), 1, failoverErr)
	require.Zero(t, repo.tempUnschedCalls)

	rateLimitService := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	gateway := &OpenAIGatewayService{rateLimitService: rateLimitService}
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.False(t, gateway.handleOpenAIAccountUpstreamError(
		context.Background(), account, http.StatusBadRequest, nil, payload, "gpt-5",
	))
	require.Zero(t, repo.tempUnschedCalls)
}

func TestOpenAIStreamDataStartsClientOutputUsesPayloadAwareAddedEvents(t *testing.T) {
	tests := []struct {
		data      string
		eventType string
		want      bool
	}{
		{`{"type":"error","error":{"code":"server_is_overloaded","message":"overloaded"}}`, "error", false},
		{`{"type":"error","error":{"type":"invalid_request_error","code":"content_policy_violation","message":"blocked"}}`, "error", true},
		{`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded"}}}`, "response.failed", false},
		{`{"type":"response.created","response":{"id":"resp_1"}}`, "response.created", false},
		{`{"type":"response.output_item.added","item":{"type":"reasoning","summary":[]}}`, "response.output_item.added", false},
		{`{"type":"response.output_item.added","item":{"type":"reasoning","encrypted_content":"ciphertext"}}`, "response.output_item.added", true},
		{`{"type":"response.reasoning_summary_part.added","part":{"type":"summary_text","text":""}}`, "response.reasoning_summary_part.added", false},
		{`{"type":"response.reasoning_summary_part.added","part":{"type":"summary_text","text":"thinking"}}`, "response.reasoning_summary_part.added", true},
		{`{"type":"response.output_text.delta","delta":"hi"}`, "response.output_text.delta", true},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, openAIStreamDataStartsClientOutput(tt.data, tt.eventType), "data=%s type=%s", tt.data, tt.eventType)
	}
}

func TestSanitizeOpenAICapacityShedErrorCodeForClient(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantChanged bool
		wantCode    string
	}{
		{"nested code", `{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"overloaded"}}}`, true, `"code":"server_error"`},
		{"top-level code", `{"type":"error","error":{"code":"slow_down","message":"slow down"}}`, true, `"code":"server_error"`},
		{"message only", `{"type":"response.failed","response":{"error":{"message":"Our servers are currently overloaded. Please try again later."}}}`, true, `"code":"server_error"`},
		{"rate limit", `{"type":"response.failed","response":{"error":{"code":"rate_limit_exceeded","message":"try again"}}}`, false, `"code":"rate_limit_exceeded"`},
		{"invalid json", `not-json`, false, `not-json`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := sanitizeOpenAICapacityShedErrorCodeForClient([]byte(tt.payload))
			require.Equal(t, tt.wantChanged, changed)
			require.Contains(t, string(got), tt.wantCode)
		})
	}
}
