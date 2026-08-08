package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// --- mock: 只记录临时不可调度写入，其余方法不应被调用 ---

type capacityShedAccountRepoStub struct {
	AccountRepository // 嵌入接口，未实现的方法会 panic（不应被调用）

	tempUnschedCalls int
}

func (r *capacityShedAccountRepoStub) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.tempUnschedCalls++
	return nil
}

// 上游容量降载是请求级信号：故障因素（客户端身份、模型容量）与账号无关，
// 同账号重试用尽后不得把账号临时摘掉——否则一个被降载的请求会顺着 failover
// 把整池账号逐个封禁，而每个账号都会以同一个错误失败。
func TestTempUnscheduleRetryableErrorSkipsRequestScopedTransient(t *testing.T) {
	t.Run("请求级瞬时故障不写账号状态", func(t *testing.T) {
		repo := &capacityShedAccountRepoStub{}
		svc := &GatewayService{accountRepo: repo}

		svc.TempUnscheduleRetryableError(context.Background(), 1, &UpstreamFailoverError{
			StatusCode:             http.StatusBadGateway,
			RetryableOnSameAccount: true,
			RequestScopedTransient: true,
		})

		require.Zero(t, repo.tempUnschedCalls)
	})

	// 对照组：同样的 502 在未标记请求级瞬时故障时仍按原有语义临时摘号，
	// 确认上面的断言来自新增守卫而非其他前置条件。
	t.Run("未标记时保持原有临时摘号语义", func(t *testing.T) {
		repo := &capacityShedAccountRepoStub{}
		svc := &GatewayService{accountRepo: repo}

		svc.TempUnscheduleRetryableError(context.Background(), 1, &UpstreamFailoverError{
			StatusCode:             http.StatusBadGateway,
			RetryableOnSameAccount: true,
		})

		require.Equal(t, 1, repo.tempUnschedCalls)
	})
}

func TestStreamFailedEventCapacityShedSwitchesOpenAIOAuthAccount(t *testing.T) {
	oauth := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	grokOAuth := &Account{ID: 3, Platform: PlatformGrok, Type: AccountTypeOAuth}

	for _, code := range []string{"server_is_overloaded", "slow_down"} {
		payload := []byte(`{"type":"response.failed","response":{"error":{"code":"` + code + `"}}}`)
		require.True(t, isOpenAIUpstreamCapacityShedEvent(payload), code)
		require.False(t, openAIStreamFailedEventRetryableOnSameAccount(oauth, payload, "overloaded"), code)
		require.True(t, openAIStreamFailedEventRetryableOnSameAccount(apiKey, payload, "overloaded"), code)
		require.True(t, openAIStreamFailedEventRetryableOnSameAccount(grokOAuth, payload, "overloaded"), code)
	}

	// 非降载的 failed 事件在非池模式下仍不做同账号重试，避免放大改动面。
	other := []byte(`{"type":"response.failed","response":{"error":{"code":"server_error"}}}`)
	require.False(t, isOpenAIUpstreamCapacityShedEvent(other))
	require.False(t, openAIStreamFailedEventRetryableOnSameAccount(oauth, other, "boom"))
}

func TestOpenAINativeBareCapacityBeforeOutputFailsOver(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	timeouts := map[string]int{
		"default without staging": 0,
		"semantic staging":        30,
	}
	tests := map[string]string{
		"event header without data type": "event: error\n" +
			`data: {"error":{"code":"server_is_overloaded"}}` + "\n\n",
		"event header without space": "event:error\n" +
			`data: {"error":{"code":"slow_down"}}` + "\n\n",
		"data type error": `data: {"type":"error","error":{"code":"slow_down"}}` + "\n\n",
	}
	for timeoutName, timeoutSeconds := range timeouts {
		t.Run(timeoutName, func(t *testing.T) {
			for name, body := range tests {
				t.Run(name, func(t *testing.T) {
					rec, written, err := runOpenAINativeCapacityStream(t, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body, timeoutSeconds)

					var failoverErr *UpstreamFailoverError
					require.ErrorAs(t, err, &failoverErr)
					require.False(t, failoverErr.RetryableOnSameAccount)
					require.True(t, failoverErr.RequestScopedTransient)
					require.False(t, written)
					require.Empty(t, rec.Body.String())
				})
			}
		})
	}
	require.True(t, logSink.ContainsMessageAtLevel("openai.responses.oauth_capacity_prewrite_failover", "warn"))
	require.True(t, logSink.ContainsFieldValue("account_id", "1"))
	require.True(t, logSink.ContainsFieldValue("capacity_code", "server_is_overloaded"))
	require.True(t, logSink.ContainsFieldValue("capacity_code", "slow_down"))
	require.True(t, logSink.ContainsFieldValue("upstream_request_id", "upstream-request-id"))
}

func TestOpenAIPassthroughBareCapacityBeforeOutputFailsOver(t *testing.T) {
	for name, body := range map[string]string{
		"event header without data type": "event:error\n" +
			`data: {"error":{"code":"server_is_overloaded"}}` + "\n\n",
		"data type error": `data: {"type":"error","error":{"code":"slow_down"}}` + "\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			rec, err := runOpenAIPassthroughCapacityStream(t, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body)

			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.False(t, failoverErr.RetryableOnSameAccount)
			require.True(t, failoverErr.RequestScopedTransient)
			require.Empty(t, rec.Body.String())
		})
	}
}

func TestOpenAINativeBareCapacityKeepsLegacyBoundaries(t *testing.T) {
	capacity := "event: error\n" + `data: {"error":{"code":"server_is_overloaded"}}` + "\n\n"
	nonCapacity := "event: error\n" + `data: {"error":{"code":"server_error"}}` + "\n\n"
	tests := map[string]struct {
		account *Account
		body    string
	}{
		"OpenAI API key": {account: &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, body: capacity},
		"Grok OAuth":     {account: &Account{ID: 2, Platform: PlatformGrok, Type: AccountTypeOAuth}, body: capacity},
		"non capacity":   {account: &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body: nonCapacity},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rec, _, err := runOpenAINativeCapacityStream(t, tt.account, tt.body, 30)

			require.Error(t, err)
			require.ErrorContains(t, err, "missing terminal event")
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr))
			require.Contains(t, rec.Body.String(), `"code"`)
		})
	}
}

func TestOpenAIPassthroughBareCapacityKeepsLegacyBoundaries(t *testing.T) {
	capacity := "event: error\n" + `data: {"error":{"code":"server_is_overloaded"}}` + "\n\n"
	nonCapacity := "event: error\n" + `data: {"error":{"code":"server_error"}}` + "\n\n"
	for name, tt := range map[string]struct {
		account *Account
		body    string
	}{
		"OpenAI API key": {account: &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, body: capacity},
		"Grok OAuth":     {account: &Account{ID: 2, Platform: PlatformGrok, Type: AccountTypeOAuth}, body: capacity},
		"non capacity":   {account: &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body: nonCapacity},
	} {
		t.Run(name, func(t *testing.T) {
			rec, err := runOpenAIPassthroughCapacityStream(t, tt.account, tt.body)

			require.ErrorContains(t, err, "missing terminal event")
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr))
			require.Contains(t, rec.Body.String(), `"code"`)
		})
	}
}

func TestOpenAINativeBareCapacityAfterOutputDoesNotReplay(t *testing.T) {
	body := `data: {"type":"response.output_text.delta","delta":"hello"}` + "\n\n" +
		"event: error\n" + `data: {"error":{"code":"server_is_overloaded"}}` + "\n\n"
	rec, _, err := runOpenAINativeCapacityStream(t, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body, 30)

	require.Error(t, err)
	require.ErrorContains(t, err, "missing terminal event")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Contains(t, rec.Body.String(), `"delta":"hello"`)
	require.Contains(t, rec.Body.String(), `"code":"server_is_overloaded"`)
}

func TestOpenAINativeStagedStructuralEventsEOFKeepHEADMissingTerminal(t *testing.T) {
	body := `data: {"type":"response.created","response":{"id":"resp_1"}}` + "\n\n" +
		`data: {"type":"response.output_item.added","item":{"id":"item_1","type":"message","content":[]}}` + "\n\n"
	rec, _, err := runOpenAINativeCapacityStream(t, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body, 30)

	require.Error(t, err)
	require.ErrorContains(t, err, "missing terminal event")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Contains(t, rec.Body.String(), `"type":"response.created"`)
	require.Contains(t, rec.Body.String(), `"type":"response.output_item.added"`)
}

func runOpenAINativeCapacityStream(t *testing.T, account *Account, body string, firstOutputTimeoutSeconds int) (*httptest.ResponseRecorder, bool, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{
		cfg: &config.Config{Gateway: config.GatewayConfig{
			OpenAIFirstOutputTimeoutSeconds: firstOutputTimeoutSeconds,
		}},
		toolCorrector: NewCodexToolCorrector(),
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"upstream-request-id"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c, account, time.Now(), "gpt-5", "gpt-5")
	return rec, c.Writer.Written(), err
}

func runOpenAIPassthroughCapacityStream(t *testing.T, account *Account, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"upstream-request-id"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
	_, err := svc.handleStreamingResponsePassthrough(c.Request.Context(), resp, c, account, time.Now(), "gpt-5", "gpt-5")
	return rec, err
}

// 出站身份的版本声明只能有一个来源：UA 的版本段、version 头、探针版本三处必须同源，
// 各自硬编码会漂移成互相矛盾的身份，而自相矛盾或陈旧的身份会被上游优先降载。
func TestCodexOutboundVersionHasSingleSource(t *testing.T) {
	require.True(t,
		strings.HasPrefix(codexCLIUserAgent, openai.CodexDefaultOriginator+"/"+codexCLIVersion+" "),
		"codexCLIUserAgent=%q 必须以 codexCLIVersion=%q 作为版本段", codexCLIUserAgent, codexCLIVersion,
	)
	require.Equal(t, codexCLIVersion, openAICodexProbeVersion)
	require.GreaterOrEqual(t, CompareVersions(codexCLIVersion, codexUpstreamMinVersion), 0,
		"codexCLIVersion=%q 不得低于上游最低门槛 %q", codexCLIVersion, codexUpstreamMinVersion,
	)
}
