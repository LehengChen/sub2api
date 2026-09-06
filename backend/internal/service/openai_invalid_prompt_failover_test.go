package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const invalidPromptPolicyMessage = "Invalid prompt: your prompt was flagged as potentially violating our usage policy. Please try again with a different prompt: https://platform.openai.com/docs/guides/reasoning#advice-on-prompting"

func invalidPromptSSE(eventType, payload string) string {
	return "event: " + eventType + "\n" + "data: " + payload + "\n\n"
}

func TestOpenAIInvalidPromptDetectionIsNarrowAndOAuthScoped(t *testing.T) {
	oauth := &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{ID: 42, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	grok := &Account{ID: 43, Platform: PlatformGrok, Type: AccountTypeOAuth}

	structured := []byte(`{"type":"response.failed","response":{"error":{"type":"invalid_prompt","message":"blocked"}}}`)
	messageOnly := []byte(`{"type":"error","error":{"message":"` + invalidPromptPolicyMessage + `"}}`)
	relatedButDifferent := []byte(`{"type":"error","error":{"message":"invalid prompt cache key; please try again"}}`)

	require.Equal(t, "structured", openAIInvalidPromptMatch(structured, ""))
	require.Equal(t, "message", openAIInvalidPromptMatch(messageOnly, ""))
	require.Empty(t, openAIInvalidPromptMatch(relatedButDifferent, ""))
	require.True(t, isOpenAIOAuthInvalidPromptEvent(oauth, structured, ""))
	require.True(t, isOpenAIOAuthInvalidPromptEvent(oauth, messageOnly, ""))
	require.False(t, isOpenAIOAuthInvalidPromptEvent(apiKey, messageOnly, ""))
	require.False(t, isOpenAIOAuthInvalidPromptEvent(grok, messageOnly, ""))
}

func TestOpenAIOAuthInvalidPromptStreamingReturnsErrorWithoutFailover(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	account := &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	responses := map[string]string{
		"response_failed": invalidPromptSSE("response.failed",
			`{"type":"response.failed","response":{"status":"failed","error":{"type":"invalid_prompt","message":"`+invalidPromptPolicyMessage+`"}}}`),
		"bare_error": invalidPromptSSE("error",
			`{"type":"error","error":{"message":"`+invalidPromptPolicyMessage+`"}}`),
	}
	for name, body := range responses {
		t.Run("native_"+name, func(t *testing.T) {
			rec, _, err := runOpenAINativeCapacityStream(t, account, body, 30)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "invalid_prompt must not select another account")
			require.Contains(t, rec.Body.String(), "Invalid prompt")
		})
		t.Run("passthrough_"+name, func(t *testing.T) {
			rec, err := runOpenAIPassthroughCapacityStream(t, account, body)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "invalid_prompt must not select another account")
			require.Contains(t, rec.Body.String(), "Invalid prompt")
		})
	}

	require.True(t, logSink.ContainsMessageAtLevel("openai.responses.oauth_invalid_prompt_no_failover", "info"))
	require.True(t, logSink.ContainsFieldValue("account_id", "41"))
	require.True(t, logSink.ContainsFieldValue("invalid_prompt_match", "message"))
	require.True(t, logSink.ContainsFieldValue("decision", "return_client_error"))
	require.True(t, logSink.ContainsFieldValue("upstream_request_id", "upstream-request-id"))
	require.True(t, logSink.ContainsFieldValue("path", "native_sse"))
	require.True(t, logSink.ContainsFieldValue("path", "passthrough_sse"))
}

func TestOpenAIOAuthInvalidPromptNonStreamingReturnsTypedClientError(t *testing.T) {
	payload := `{"type":"response.failed","error":{"type":"invalid_prompt","message":"` + invalidPromptPolicyMessage + `"}}`
	body := []byte(invalidPromptSSE("response.failed", payload))
	account := &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	tests := map[string]func(*OpenAIGatewayService, *http.Response, []byte) (string, error){
		"native": func(svc *OpenAIGatewayService, resp *http.Response, body []byte) (string, error) {
			c, rec := newNonStreamingFailoverContext(t)
			_, err := svc.handleSSEToJSON(resp, c, account, body, "gpt-6-astra", "gpt-6-astra")
			return rec.Body.String(), err
		},
		"passthrough": func(svc *OpenAIGatewayService, resp *http.Response, body []byte) (string, error) {
			c, rec := newNonStreamingFailoverContext(t)
			_, err := svc.handlePassthroughSSEToJSON(resp, c, account, body, "gpt-6-astra", "gpt-6-astra")
			return rec.Body.String(), err
		},
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			responseBody, err := run(newNonStreamingFailoverService(), newNonStreamingSSEResponse(), body)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr))
			require.Equal(t, "invalid_prompt", gjson.Get(responseBody, "error.type").String())
			require.Equal(t, invalidPromptPolicyMessage, gjson.Get(responseBody, "error.message").String())
		})
	}
}

func TestOpenAIOAuthInvalidPromptHTTP400RemainsDirect(t *testing.T) {
	c, rec := newOpenAIUpstreamErrorTestContext(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	body := `{"error":{"type":"invalid_prompt","message":"` + invalidPromptPolicyMessage + `"}}`

	_, err := svc.handleErrorResponse(context.Background(),
		newOpenAIUpstreamErrorResponse(http.StatusBadRequest, body), c,
		&Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, nil)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_prompt", gjson.Get(rec.Body.String(), "error.type").String())
	require.Equal(t, invalidPromptPolicyMessage, gjson.Get(rec.Body.String(), "error.message").String())
}

func TestOpenAIOAuthInvalidPromptWSHTTPBridgeDoesNotFailOver(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_invalid_prompt"}}`,
		"",
		`data: {"type":"response.failed","response":{"id":"resp_invalid_prompt","status":"failed","error":{"type":"invalid_prompt","message":"` + invalidPromptPolicyMessage + `"}}}`,
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"rid-invalid-prompt-ws-bridge"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 44, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1}
	c, _ := newOpenAIUpstreamErrorTestContext(t)
	payload := []byte(`{"type":"response.create","model":"gpt-6-astra","input":"hi"}`)
	var writes [][]byte

	result, err := svc.proxyOpenAIWSHTTPBridgeTurn(
		context.Background(), c, account, "oauth-token", payload, len(payload),
		"gpt-6-astra", "", "", "", "", 1,
		func(message []byte) error {
			writes = append(writes, append([]byte(nil), message...))
			return nil
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "response.failed", result.UpstreamTerminalEvent)
	require.Len(t, writes, 2)
	require.Equal(t, "invalid_prompt", gjson.GetBytes(writes[1], "response.error.type").String())
	require.Contains(t, gjson.GetBytes(writes[1], "response.error.message").String(), "Invalid prompt")
	require.True(t, logSink.ContainsFieldValue("path", "ws_http_bridge"))
}

func TestInvalidPromptPatchKeepsAPIKeyAndGrokRetryBoundary(t *testing.T) {
	body := invalidPromptSSE("error", `{"type":"error","error":{"message":"`+invalidPromptPolicyMessage+`"}}`)
	for name, account := range map[string]*Account{
		"openai_api_key": {ID: 42, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		"grok_oauth":     {ID: 43, Platform: PlatformGrok, Type: AccountTypeOAuth},
	} {
		t.Run(name, func(t *testing.T) {
			rec, _, err := runOpenAINativeCapacityStream(t, account, body, 30)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Empty(t, rec.Body.String())
		})
	}
}

func TestOpenAIOAuthInvalidPromptDoesNotMaskCapacity(t *testing.T) {
	account := &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	body := invalidPromptSSE("response.failed", `{"type":"response.failed","response":{"error":{"code":"server_is_overloaded"}}}`)
	rec, _, err := runOpenAINativeCapacityStream(t, account, body, 30)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.OpenAIOAuthCapacity)
	require.Empty(t, rec.Body.String())
}
