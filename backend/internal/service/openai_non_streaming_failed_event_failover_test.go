package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// stream=false 时上游仍可能返回 SSE，容量错误经 HTTP 200 的 response.failed
// 终止事件回传。此前该路径直接写 502，池内还有可调度账号也不会切号；
// 而完全相同的上游错误在流式路径上是会切号的
// （TestOpenAIStreamingResponseFailedBeforeOutputCapacityErrorReturnsFailover）。
const nonStreamingCapacityFailedSSE = `event: response.created
data: {"type":"response.created","response":{"id":"resp_1"}}

event: response.failed
data: {"type":"response.failed","error":{"message":"Selected model is at capacity. Please try a different model.","type":"invalid_request_error"}}

`

func newNonStreamingFailoverTestService() *OpenAIGatewayService {
	return &OpenAIGatewayService{cfg: &config.Config{
		Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize},
	}}
}

func newNonStreamingFailoverTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, rec
}

func newCapacityFailedSSEResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"rid-nonstream-capacity"},
		},
		Body: io.NopCloser(strings.NewReader(nonStreamingCapacityFailedSSE)),
	}
}

func TestNonStreamingSSEToJSONCapacityFailedReturnsFailover(t *testing.T) {
	svc := newNonStreamingFailoverTestService()
	c, rec := newNonStreamingFailoverTestContext()

	_, err := svc.handleNonStreamingResponse(
		context.Background(),
		newCapacityFailedSSEResponse(),
		c,
		&Account{ID: 7, Type: AccountTypeOAuth},
		"gpt-5.5",
		"gpt-5.5",
	)

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr),
		"容量错误必须触发切号，实际错误: %v", err)
	require.Empty(t, rec.Body.String(),
		"切号前不得向下游写入任何字节，否则无法安全重放")
	require.NotContains(t, rec.Body.String(), "at capacity",
		"上游原始容量文案不应暴露给最终用户")
}

func TestNonStreamingPassthroughSSEToJSONCapacityFailedReturnsFailover(t *testing.T) {
	svc := newNonStreamingFailoverTestService()
	c, rec := newNonStreamingFailoverTestContext()

	_, err := svc.handleNonStreamingResponsePassthrough(
		context.Background(),
		newCapacityFailedSSEResponse(),
		c,
		&Account{ID: 7, Type: AccountTypeOAuth},
		"gpt-5.5",
		"gpt-5.5",
	)

	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr),
		"透传路径的容量错误同样必须触发切号，实际错误: %v", err)
	require.Empty(t, rec.Body.String())
}

// 用户参数错误不是临时故障，必须原样回写而不是浪费一次切号。
func TestNonStreamingSSEToJSONInvalidRequestFailedStillWritesError(t *testing.T) {
	svc := newNonStreamingFailoverTestService()
	c, rec := newNonStreamingFailoverTestContext()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.failed",
			`data: {"type":"response.failed","error":{"message":"Invalid value for 'temperature'","type":"invalid_request_error"}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleNonStreamingResponse(
		context.Background(), resp, c,
		&Account{ID: 7, Type: AccountTypeOAuth}, "gpt-5.5", "gpt-5.5",
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "参数错误不应触发切号")
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "upstream_error")
}

// 上下文超限同样不可重放到别的账号——换号也一样超限。
func TestNonStreamingSSEToJSONContextWindowFailedStillWritesError(t *testing.T) {
	svc := newNonStreamingFailoverTestService()
	c, rec := newNonStreamingFailoverTestContext()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"event: response.failed",
			`data: {"type":"response.failed","error":{"message":"Your input exceeds the context window of this model.","type":"invalid_request_error"}}`,
			"",
		}, "\n"))),
	}

	_, err := svc.handleNonStreamingResponse(
		context.Background(), resp, c,
		&Account{ID: 7, Type: AccountTypeOAuth}, "gpt-5.5", "gpt-5.5",
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "上下文超限换号也无法解决，不应切号")
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

// 响应已提交（例如 body-signal compact 心跳已写出 200 响应头）时禁止切号：
// 下游已经收到字节，重放会产生重复输出。
func TestNonStreamingSSEToJSONSkipsFailoverAfterResponseCommitted(t *testing.T) {
	svc := newNonStreamingFailoverTestService()
	c, rec := newNonStreamingFailoverTestContext()
	MarkResponseCommitted(c)

	_, err := svc.handleNonStreamingResponse(
		context.Background(),
		newCapacityFailedSSEResponse(),
		c,
		&Account{ID: 7, Type: AccountTypeOAuth},
		"gpt-5.5",
		"gpt-5.5",
	)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr),
		"响应已提交后必须沿用原有错误回写路径，不得切号")
	require.Equal(t, http.StatusBadGateway, rec.Code)
}

// 判定函数本身的边界：account/resp 缺失或响应已写出时一律不切号。
func TestNonStreamingFailedEventFailoverGuards(t *testing.T) {
	svc := newNonStreamingFailoverTestService()
	payload := []byte(`{"type":"response.failed","error":{"message":"Selected model is at capacity. Please try a different model.","type":"invalid_request_error"}}`)
	msg := "Selected model is at capacity. Please try a different model."
	resp := newCapacityFailedSSEResponse()

	t.Run("nil_account", func(t *testing.T) {
		c, _ := newNonStreamingFailoverTestContext()
		require.Nil(t, svc.nonStreamingFailedEventFailover(c, nil, false, resp, payload, msg))
	})

	t.Run("nil_response", func(t *testing.T) {
		c, _ := newNonStreamingFailoverTestContext()
		require.Nil(t, svc.nonStreamingFailedEventFailover(c, &Account{ID: 1}, false, nil, payload, msg))
	})

	t.Run("nil_context", func(t *testing.T) {
		require.Nil(t, svc.nonStreamingFailedEventFailover(nil, &Account{ID: 1}, false, resp, payload, msg))
	})

	t.Run("response_committed", func(t *testing.T) {
		c, _ := newNonStreamingFailoverTestContext()
		MarkResponseCommitted(c)
		require.Nil(t, svc.nonStreamingFailedEventFailover(c, &Account{ID: 1}, false, resp, payload, msg))
	})

	t.Run("clean_context_failovers", func(t *testing.T) {
		c, _ := newNonStreamingFailoverTestContext()
		require.NotNil(t, svc.nonStreamingFailedEventFailover(c, &Account{ID: 1}, false, resp, payload, msg))
	})
}
