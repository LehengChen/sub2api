package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// issue #5281：stream=false 时上游仍可能回 SSE（其他 sub2api 实例、部分 OpenAI 兼容
// 上游），容量/限流错误经 HTTP 200 的终止事件回传。handleSSEToJSON 与
// handlePassthroughSSEToJSON 把所有终止事件塞进固定 502，而几百行外的流式读取器对
// 同一帧走 openAIStreamFailedEventShouldFailover / openAIStreamErrorEventShouldFailover
// 判定并换号——同一个上游、同一个事件，只因请求上的 stream 标志而结果相反。

func newNonStreamingFailoverContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

func newNonStreamingFailoverService() *OpenAIGatewayService {
	return &OpenAIGatewayService{cfg: &config.Config{}}
}

func newNonStreamingFailoverAccount() *Account {
	return &Account{
		ID:       1,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Name:     "oauth-account",
	}
}

func newNonStreamingSSEResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"rid-nonstreaming-failed"},
		},
	}
}

func sseTerminalBody(eventType, data string) []byte {
	return []byte(strings.Join([]string{
		"event: " + eventType,
		"data: " + data,
		"",
		"data: [DONE]",
	}, "\n"))
}

// 主复现：issue 报告的容量错误，与流式兄弟用例
// TestOpenAIStreamingResponseFailedBeforeOutputCapacityErrorReturnsFailover 逐项对齐。
func TestNonStreamingSSEToJSON_CapacityFailedEventFailsOver(t *testing.T) {
	c, rec := newNonStreamingFailoverContext(t)
	svc := newNonStreamingFailoverService()
	body := sseTerminalBody("response.failed",
		`{"type":"response.failed","error":{"message":"Selected model is at capacity. Please try a different model.","type":"invalid_request_error"}}`)

	result, err := svc.handleSSEToJSON(newNonStreamingSSEResponse(), c, newNonStreamingFailoverAccount(), body, "model", "model")

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.True(t, failoverErr.OpenAIOAuthCapacity)
	require.False(t, failoverErr.RetryableOnSameAccount)
	// 换号的前提：一个字节都没写出去。
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

// 未知 response.failed 不足以证明换号有意义，继续走协议错误路径。
func TestNonStreamingSSEToJSON_UnclassifiedFailedEventStaysProtocolError(t *testing.T) {
	c, rec := newNonStreamingFailoverContext(t)
	svc := newNonStreamingFailoverService()
	payload := []byte(`{"type":"response.failed","error":{"message":"upstream rejected request"}}`)
	body := sseTerminalBody("response.failed", string(payload))

	result, err := svc.handleSSEToJSON(newNonStreamingSSEResponse(), c, newNonStreamingFailoverAccount(), body, "model", "model")

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.Error(t, err)
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "upstream rejected request")
}

// 「response.failed 必须回写协议错误」这一契约没有丢：明确不可重试的错误仍写 502。
func TestNonStreamingSSEToJSON_NonRetryableFailedEventStillWritesProtocolError(t *testing.T) {
	cases := []struct {
		name    string
		data    string
		wantMsg string
	}{
		{
			name:    "invalid_request",
			data:    `{"type":"response.failed","error":{"type":"invalid_request_error","code":"invalid_request","message":"unknown parameter foo"}}`,
			wantMsg: "unknown parameter foo",
		},
		{
			name:    "context_window",
			data:    `{"type":"response.failed","response":{"id":"resp_failed","status":"failed","output":[],"error":{"code":"upstream_error","message":"input exceeds the context window"}}}`,
			wantMsg: "input exceeds the context window",
		},
		{
			name:    "content_policy",
			data:    `{"type":"response.failed","error":{"type":"content_policy_violation","message":"blocked by our content policy"}}`,
			wantMsg: "blocked by our content policy",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newNonStreamingFailoverContext(t)
			svc := newNonStreamingFailoverService()

			result, err := svc.handleSSEToJSON(newNonStreamingSSEResponse(), c,
				newNonStreamingFailoverAccount(), sseTerminalBody("response.failed", tc.data), "model", "model")

			require.Nil(t, result)
			require.Error(t, err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr), "不可重试的上游错误不得换号")
			require.Equal(t, http.StatusBadGateway, rec.Code)
			require.Contains(t, rec.Body.String(), tc.wantMsg)
			require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
		})
	}
}

// 裸 error 帧走的是更保守的那个分类器：只有正向识别为瞬时才换号。
// 这条用例证明两种终止事件确实被分派到了各自的判定，而不是共用一个。
func TestNonStreamingSSEToJSON_BareErrorEventUsesConservativeClassifier(t *testing.T) {
	t.Run("non_transient_stays_protocol_error", func(t *testing.T) {
		c, rec := newNonStreamingFailoverContext(t)
		svc := newNonStreamingFailoverService()
		data := `{"type":"error","error":{"message":"upstream rejected request"}}`

		// 同一条文案：failed 判换号，error 判不换号——差异就在分派。
		require.True(t, openAIStreamFailedEventShouldFailover([]byte(data), "upstream rejected request"))
		require.False(t, openAIStreamErrorEventShouldFailover([]byte(data), "upstream rejected request"))

		result, err := svc.handleSSEToJSON(newNonStreamingSSEResponse(), c,
			newNonStreamingFailoverAccount(), sseTerminalBody("error", data), "model", "model")

		require.Nil(t, result)
		require.Error(t, err)
		var failoverErr *UpstreamFailoverError
		require.False(t, errors.As(err, &failoverErr))
		require.Equal(t, http.StatusBadGateway, rec.Code)
	})

	t.Run("transient_fails_over", func(t *testing.T) {
		c, _ := newNonStreamingFailoverContext(t)
		svc := newNonStreamingFailoverService()
		data := `{"type":"error","error":{"message":"Temporary upstream failure, please retry"}}`

		result, err := svc.handleSSEToJSON(newNonStreamingSSEResponse(), c,
			newNonStreamingFailoverAccount(), sseTerminalBody("error", data), "model", "model")

		require.Nil(t, result)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.False(t, c.Writer.Written())
	})
}

// 透传路径必须与合成路径同步修复，否则又造出一处新的不对称。
func TestNonStreamingPassthroughSSEToJSON_CapacityFailedEventFailsOver(t *testing.T) {
	c, rec := newNonStreamingFailoverContext(t)
	svc := newNonStreamingFailoverService()
	body := sseTerminalBody("response.failed",
		`{"type":"response.failed","error":{"message":"Selected model is at capacity. Please try a different model.","type":"invalid_request_error"}}`)

	result, err := svc.handlePassthroughSSEToJSON(newNonStreamingSSEResponse(), c, newNonStreamingFailoverAccount(), body, "model", "model")

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.True(t, failoverErr.OpenAIOAuthCapacity)
	require.False(t, failoverErr.RetryableOnSameAccount)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}

// 非流式扩展只接受 OAuth capacity 或明确的 429/瞬时分类，不能继承流式
// response.failed 对未知错误的宽松默认值。
func TestNonStreamingSSEToJSON_RequiresPositiveScopedClassifier(t *testing.T) {
	payloads := []string{
		`{"type":"response.failed","error":{"message":"Selected model is at capacity. Please try a different model.","type":"invalid_request_error"}}`,
		`{"type":"response.failed","error":{"message":"upstream rejected request"}}`,
		`{"type":"response.failed","error":{"type":"invalid_request_error","code":"invalid_request","message":"unknown parameter foo"}}`,
		`{"type":"response.failed","response":{"id":"r","status":"failed","output":[],"error":{"code":"upstream_error","message":"input exceeds the context window"}}}`,
		`{"type":"response.failed","error":{"type":"content_policy_violation","message":"blocked by our content policy"}}`,
	}

	for _, data := range payloads {
		t.Run(data[:min(len(data), 60)], func(t *testing.T) {
			payload := []byte(data)
			message := extractOpenAISSEErrorMessage(payload)
			account := newNonStreamingFailoverAccount()
			want := isOpenAIOAuthCapacityShedEvent(account, payload, message) ||
				openAIStreamFailureStatus(payload, message) == http.StatusTooManyRequests ||
				isOpenAITransientProcessingError(http.StatusBadRequest, message, payload)

			c, _ := newNonStreamingFailoverContext(t)
			svc := newNonStreamingFailoverService()
			_, err := svc.handleSSEToJSON(newNonStreamingSSEResponse(), c,
				account, sseTerminalBody("response.failed", data), "model", "model")

			var failoverErr *UpstreamFailoverError
			require.Equal(t, want, errors.As(err, &failoverErr),
				"非流式裁决必须要求明确的 OAuth/瞬时证据：%s", data)
		})
	}
}

// 服务侧已显式提交响应后不得再提议换号；能否真正换号仍由 handler 的
// openAIForwardMayFailover 用 keepalive 调整后的写出量仲裁（#3887），此处不重复实现。
func TestNonStreamingSSEToJSON_CommittedResponseKeepsProtocolError(t *testing.T) {
	c, rec := newNonStreamingFailoverContext(t)
	svc := newNonStreamingFailoverService()
	MarkResponseCommitted(c)
	body := sseTerminalBody("response.failed",
		`{"type":"response.failed","error":{"message":"Selected model is at capacity. Please try a different model.","type":"invalid_request_error"}}`)

	result, err := svc.handleSSEToJSON(newNonStreamingSSEResponse(), c, newNonStreamingFailoverAccount(), body, "model", "model")

	require.Nil(t, result)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

func TestNonStreamingTerminalFailureFailover_NilAccountProposesNothing(t *testing.T) {
	c, _ := newNonStreamingFailoverContext(t)
	svc := newNonStreamingFailoverService()
	payload := []byte(`{"type":"response.failed","error":{"message":"Selected model is at capacity. Please try a different model."}}`)

	require.Nil(t, svc.nonStreamingTerminalFailureFailover(
		c, newNonStreamingSSEResponse(), nil, false, "response.failed", payload,
		"Selected model is at capacity. Please try a different model."))
}
