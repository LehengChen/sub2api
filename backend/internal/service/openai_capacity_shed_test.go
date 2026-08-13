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
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// --- mock: 只记录临时不可调度写入，其余方法不应被调用 ---

type capacityShedAccountRepoStub struct {
	AccountRepository // 嵌入接口，未实现的方法会 panic（不应被调用）

	tempUnschedCalls int
	setOverloadCalls int
	extendCalls      int
	overloadAccount  int64
	overloadUntil    time.Time
}

func (r *capacityShedAccountRepoStub) SetTempUnschedulable(_ context.Context, _ int64, _ time.Time, _ string) error {
	r.tempUnschedCalls++
	return nil
}

func (r *capacityShedAccountRepoStub) SetOverloaded(_ context.Context, accountID int64, until time.Time) error {
	r.setOverloadCalls++
	r.overloadAccount = accountID
	r.overloadUntil = until
	return nil
}

func (r *capacityShedAccountRepoStub) ExtendOverloaded(_ context.Context, accountID int64, until time.Time) error {
	r.extendCalls++
	r.overloadAccount = accountID
	r.overloadUntil = until
	return nil
}

// 通用 RequestScopedTransient 仍保持 HEAD 的“不走旧临时封禁”语义；OAuth
// capacity 的账号冷却由独立路径处理，不能扩大这个通用函数的行为。
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

func TestOpenAIOAuthCapacityShedEventRecognizesOnlyKnownSignals(t *testing.T) {
	oauth := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	grokOAuth := &Account{ID: 3, Platform: PlatformGrok, Type: AccountTypeOAuth}

	tests := map[string]struct {
		payload []byte
		message string
	}{
		"structured code": {
			payload: []byte(`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded"}}}`),
		},
		"issue capacity message": {
			payload: []byte(`{"type":"response.failed","error":{"type":"invalid_request_error"}}`),
			message: "Selected model is at capacity. Please try a different model.",
		},
		"observed overload message": {
			payload: []byte(`{"type":"response.failed","error":{"type":"invalid_request_error","message":"` + openAIUpstreamOverloadMessage + `"}}`),
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.True(t, isOpenAIOAuthCapacityShedEvent(oauth, tt.payload, tt.message))
			require.False(t, isOpenAIOAuthCapacityShedEvent(apiKey, tt.payload, tt.message))
			require.False(t, isOpenAIOAuthCapacityShedEvent(grokOAuth, tt.payload, tt.message))
		})
	}

	nonCapacity := []byte(`{"type":"response.failed","error":{"type":"invalid_request_error","message":"The service rejected this invalid request."}}`)
	require.False(t, isOpenAIOAuthCapacityShedEvent(oauth, nonCapacity, ""))
	require.False(t, isOpenAIOAuthCapacityShedEvent(oauth, nil, "Our servers are currently overloaded for maintenance. Please try again later."))
	require.False(t, isOpenAITransientProcessingError(http.StatusBadRequest, openAIUpstreamOverloadMessage, nil),
		"the production phrase must stay out of the cross-platform transient classifier")
}

func TestSanitizeOpenAIOAuthCapacityEventRemovesKnownSignalShapes(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	for name, tt := range map[string]struct {
		eventType string
		payload   string
	}{
		"nested response code": {
			eventType: "response.failed",
			payload:   `{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`,
		},
		"top-level error code": {
			eventType: "error",
			payload:   `{"type":"error","error":{"code":"slow_down","message":"Selected model is at capacity."}}`,
		},
		"message only": {
			eventType: "response.failed",
			payload:   `{"type":"response.failed","error":{"message":"Selected model is at capacity. Please try a different model."}}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			updated, ok := sanitizeOpenAIOAuthCapacityEventForClient(account, []byte(tt.payload), tt.eventType)

			require.True(t, ok)
			require.Equal(t, "response.failed", gjson.GetBytes(updated, "type").String())
			require.Equal(t, "failed", gjson.GetBytes(updated, "response.status").String())
			require.Equal(t, "server_error", gjson.GetBytes(updated, "response.error.code").String())
			require.Equal(t, "server_error", gjson.GetBytes(updated, "response.error.type").String())
			require.False(t, gjson.GetBytes(updated, "error").Exists())
			lower := strings.ToLower(string(updated))
			require.NotContains(t, lower, "at capacity")
			require.NotContains(t, lower, "overload")
			require.NotContains(t, lower, "slow_down")
		})
	}
}

func TestOpenAIOAuthOverloadResponseFailedBoundaries(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	preOutput := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		"",
		"event: response.failed",
		`data: {"type":"response.failed","error":{"type":"invalid_request_error","message":"` + openAIUpstreamOverloadMessage + `"}}`,
		"",
	}, "\n")
	postStructuralOutput := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		"",
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","item":{"id":"item_1","type":"message","content":[]}}`,
		"",
		"event: response.metadata",
		`data: {"type":"response.metadata","metadata":{"trace_id":"trace_1"}}`,
		"",
		"event: response.failed",
		`data: {"type":"response.failed","error":{"type":"invalid_request_error","message":"` + openAIUpstreamOverloadMessage + `"}}`,
		"",
	}, "\n")
	postKeepalive := strings.Join([]string{
		"event: keepalive",
		`data: {"type":"keepalive"}`,
		"",
		"event: response.failed",
		`data: {"type":"response.failed","response":{"error":{"code":"server_is_overloaded"}}}`,
		"",
	}, "\n")
	postSemanticOutput := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		"",
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","item":{"id":"item_1","type":"message","content":[]}}`,
		"",
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		"",
		"event: response.failed",
		`data: {"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"` + openAIUpstreamOverloadMessage + `"}}}`,
		"",
	}, "\n")

	runners := map[string]func(*Account, string) (*httptest.ResponseRecorder, error){
		"native timeout disabled": func(account *Account, body string) (*httptest.ResponseRecorder, error) {
			rec, _, err := runOpenAINativeCapacityStream(t, account, body, 0)
			return rec, err
		},
		"native timeout enabled": func(account *Account, body string) (*httptest.ResponseRecorder, error) {
			rec, _, err := runOpenAINativeCapacityStream(t, account, body, 30)
			return rec, err
		},
		"passthrough": func(account *Account, body string) (*httptest.ResponseRecorder, error) {
			return runOpenAIPassthroughCapacityStream(t, account, body)
		},
	}
	for name, run := range runners {
		t.Run(name+" OAuth pre-output", func(t *testing.T) {
			rec, err := run(&Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, preOutput)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.False(t, failoverErr.RetryableOnSameAccount)
			require.True(t, failoverErr.RequestScopedTransient)
			require.True(t, failoverErr.SafeToFailoverAfterWrite)
			require.True(t, failoverErr.OpenAIOAuthCapacity)
			require.Equal(t, "120", failoverErr.ResponseHeaders.Get("Retry-After"))
			require.Empty(t, rec.Body.String())
		})

		t.Run(name+" structural output still fails over", func(t *testing.T) {
			rec, err := run(&Account{ID: 13, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, postStructuralOutput)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.Empty(t, rec.Body.String())
			require.Empty(t, rec.Header().Get("X-Codex-Primary-Used-Percent"))
		})

		t.Run(name+" keepalive still fails over", func(t *testing.T) {
			rec, err := run(&Account{ID: 14, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, postKeepalive)
			var failoverErr *UpstreamFailoverError
			require.ErrorAs(t, err, &failoverErr)
			require.True(t, failoverErr.OpenAIOAuthCapacity)
			require.Empty(t, rec.Body.String())
		})

		t.Run(name+" semantic output rewrites retryable error", func(t *testing.T) {
			rec, err := run(&Account{ID: 15, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, postSemanticOutput)
			var failoverErr *UpstreamFailoverError
			require.Error(t, err)
			require.False(t, errors.As(err, &failoverErr))
			require.Contains(t, rec.Body.String(), `"delta":"hello"`)
			require.Contains(t, rec.Body.String(), `"code":"server_error"`)
			require.NotContains(t, strings.ToLower(rec.Body.String()), "server_is_overloaded")
			require.NotContains(t, strings.ToLower(rec.Body.String()), "overloaded")
		})
	}

	require.True(t, logSink.ContainsMessageAtLevel("openai.responses.oauth_capacity_prewrite_failover", "warn"))
	require.True(t, logSink.ContainsMessageAtLevel("openai.responses.oauth_capacity_postwrite_rewrite", "warn"))
	require.True(t, logSink.ContainsFieldValue("event_type", "response.failed"))
	require.True(t, logSink.ContainsFieldValue("capacity_match", "message"))
	require.True(t, logSink.ContainsFieldValue("decision", "failover"))
	require.True(t, logSink.ContainsFieldValue("decision", "rewrite_server_error"))
	require.True(t, logSink.ContainsFieldValue("protocol_output_started", "true"))
	require.True(t, logSink.ContainsFieldValue("semantic_output_started", "false"))
	require.True(t, logSink.ContainsFieldValue("semantic_output_started", "true"))
	require.True(t, logSink.ContainsFieldValue("output_start_event_type", "response.output_item.added"))
	require.True(t, logSink.ContainsFieldValue("semantic_output_start_event_type", "response.output_text.delta"))
}

func TestOpenAIStreamSemanticOutputClassificationKeepsUnknownEventsConservative(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		eventType string
		want      bool
	}{
		{name: "keepalive", payload: `{"type":"keepalive"}`, want: false},
		{name: "structural item", payload: `{"type":"response.output_item.added","item":{"content":[]}}`, want: false},
		{name: "empty delta", payload: `{"type":"response.output_text.delta","delta":""}`, want: false},
		{name: "text delta", payload: `{"type":"response.output_text.delta","delta":"hello"}`, want: true},
		{name: "unknown event", payload: `{"type":"response.future_output","value":"opaque"}`, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIStreamDataStartsSemanticOutput([]byte(tt.payload), tt.eventType))
		})
	}
}

func TestOpenAIOAuthStructuralOutputKeepsOrdinaryFailedHEADBoundary(t *testing.T) {
	body := strings.Join([]string{
		"event: response.created",
		`data: {"type":"response.created","response":{"id":"resp_1"}}`,
		"",
		"event: response.output_item.added",
		`data: {"type":"response.output_item.added","item":{"id":"item_1","type":"message","content":[]}}`,
		"",
		"event: response.failed",
		`data: {"type":"response.failed","response":{"error":{"code":"server_error","message":"ordinary upstream failure"}}}`,
		"",
	}, "\n")

	for name, run := range map[string]func(*Account, string) (*httptest.ResponseRecorder, error){
		"native": func(account *Account, body string) (*httptest.ResponseRecorder, error) {
			rec, _, err := runOpenAINativeCapacityStream(t, account, body, 0)
			return rec, err
		},
		"passthrough": func(account *Account, body string) (*httptest.ResponseRecorder, error) {
			return runOpenAIPassthroughCapacityStream(t, account, body)
		},
	} {
		t.Run(name, func(t *testing.T) {
			rec, err := run(&Account{ID: 16, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body)
			var failoverErr *UpstreamFailoverError
			require.Error(t, err)
			require.False(t, errors.As(err, &failoverErr))
			require.Contains(t, rec.Body.String(), `"type":"response.output_item.added"`)
			require.Contains(t, rec.Body.String(), `"code":"server_error"`)
		})
	}
}

func TestOpenAIOAuthCapacityInstallsTwoMinuteAccountCooldown(t *testing.T) {
	body := `data: {"type":"response.output_text.delta","delta":"hello"}` + "\n\n" +
		`data: {"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"` + openAIUpstreamOverloadMessage + `"}}}` + "\n\n"
	runners := map[string]func(*capacityShedAccountRepoStub) (*httptest.ResponseRecorder, error){
		"native": func(repo *capacityShedAccountRepoStub) (*httptest.ResponseRecorder, error) {
			rec, _, err := runOpenAINativeCapacityStream(t, &Account{ID: 21, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body, 30, repo)
			return rec, err
		},
		"passthrough": func(repo *capacityShedAccountRepoStub) (*httptest.ResponseRecorder, error) {
			return runOpenAIPassthroughCapacityStream(t, &Account{ID: 21, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body, repo)
		},
	}
	for name, run := range runners {
		t.Run(name, func(t *testing.T) {
			repo := &capacityShedAccountRepoStub{}
			startedAt := time.Now()
			rec, err := run(repo)

			require.Error(t, err)
			require.Equal(t, 1, repo.extendCalls)
			require.Zero(t, repo.setOverloadCalls)
			require.Equal(t, int64(21), repo.overloadAccount)
			require.True(t, repo.overloadUntil.After(startedAt.Add(119*time.Second)))
			require.True(t, repo.overloadUntil.Before(time.Now().Add(121*time.Second)))
			require.Contains(t, rec.Body.String(), `"code":"server_error"`)
		})
	}

	t.Run("pre-semantic failover persists cooldown", func(t *testing.T) {
		repo := &capacityShedAccountRepoStub{}
		body := `data: {"type":"response.created","response":{"id":"resp_1"}}` + "\n\n" +
			`data: {"type":"response.failed","response":{"error":{"code":"server_is_overloaded"}}}` + "\n\n"
		rec, _, err := runOpenAINativeCapacityStream(t, &Account{ID: 23, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body, 30, repo)

		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.True(t, failoverErr.OpenAIOAuthCapacity)
		require.Equal(t, 1, repo.extendCalls)
		require.Zero(t, repo.setOverloadCalls)
		require.Equal(t, int64(23), repo.overloadAccount)
		require.Empty(t, rec.Body.String())
	})

	svc := &OpenAIGatewayService{}
	account := &Account{ID: 22, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc.markOpenAIOAuthCapacityOverloaded(context.Background(), account)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account), "capacity cooldown must block the local scheduler immediately")

	t.Run("does not shorten existing overload cooldown", func(t *testing.T) {
		repo := &capacityShedAccountRepoStub{}
		existingUntil := time.Now().Add(10 * time.Minute)
		account := &Account{ID: 24, Platform: PlatformOpenAI, Type: AccountTypeOAuth, OverloadUntil: &existingUntil}
		svc := &OpenAIGatewayService{accountRepo: repo}

		svc.markOpenAIOAuthCapacityOverloaded(context.Background(), account)

		require.Equal(t, 1, repo.extendCalls)
		require.Zero(t, repo.setOverloadCalls)
		require.Equal(t, existingUntil, repo.overloadUntil)
	})
}

func TestOpenAIOAuthOverloadResponseFailedPrecedesPassthroughRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	rule := newNonFailoverPassthroughRule(http.StatusBadRequest, "currently overloaded", http.StatusTeapot, "mapped error")
	rule.Platforms = []string{PlatformOpenAI}
	ruleSvc := &ErrorPassthroughService{}
	ruleSvc.setLocalCache([]*model.ErrorPassthroughRule{rule})
	BindErrorPassthroughService(c, ruleSvc)

	body := strings.Join([]string{
		"event: response.failed",
		`data: {"type":"response.failed","error":{"type":"invalid_request_error","message":"` + openAIUpstreamOverloadMessage + `"}}`,
		"",
	}, "\n")
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"rid-overload-rule"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	_, err := svc.handleStreamingResponse(c.Request.Context(), resp, c,
		&Account{ID: 14, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Now(), "gpt-5", "gpt-5")
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.False(t, IsResponseCommitted(c))
	require.Empty(t, rec.Body.String())
}

func TestOpenAINativeBareCapacityBeforeOutputFailsOver(t *testing.T) {
	logSink, restore := captureStructuredLog(t)
	defer restore()

	timeouts := map[string]int{
		"timeout disabled": 0,
		"timeout enabled":  30,
	}
	tests := map[string]string{
		"event header without data type": "event: error\n" +
			`data: {"error":{"code":"server_is_overloaded"}}` + "\n\n",
		"event header without space": "event:error\n" +
			`data: {"error":{"code":"slow_down"}}` + "\n\n",
		"data type error": `data: {"type":"error","error":{"code":"slow_down"}}` + "\n\n",
		"message-only overload": "event:error\n" +
			`data: {"error":{"type":"invalid_request_error","message":"` + openAIUpstreamOverloadMessage + `"}}` + "\n\n",
		"response.failed header without data type": "event:response.failed\n" +
			`data: {"error":{"code":"server_is_overloaded"}}` + "\n\n",
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
		"message-only overload": "event:error\n" +
			`data: {"error":{"type":"invalid_request_error","message":"` + openAIUpstreamOverloadMessage + `"}}` + "\n\n",
		"response.failed header without data type": "event:response.failed\n" +
			`data: {"error":{"code":"server_is_overloaded"}}` + "\n\n",
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
		account      *Account
		body         string
		expectedCode string
	}{
		"OpenAI API key": {account: &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, body: capacity, expectedCode: "server_is_overloaded"},
		"Grok OAuth":     {account: &Account{ID: 2, Platform: PlatformGrok, Type: AccountTypeOAuth}, body: capacity, expectedCode: "server_is_overloaded"},
		"non capacity":   {account: &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body: nonCapacity, expectedCode: "server_error"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			rec, _, err := runOpenAINativeCapacityStream(t, tt.account, tt.body, 30)

			require.Error(t, err)
			require.ErrorContains(t, err, "missing terminal event")
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr))
			require.Contains(t, rec.Body.String(), `"code":"`+tt.expectedCode+`"`)
		})
	}
}

func TestOpenAIPassthroughBareCapacityKeepsLegacyBoundaries(t *testing.T) {
	capacity := "event: error\n" + `data: {"error":{"code":"server_is_overloaded"}}` + "\n\n"
	nonCapacity := "event: error\n" + `data: {"error":{"code":"server_error"}}` + "\n\n"
	for name, tt := range map[string]struct {
		account      *Account
		body         string
		expectedCode string
	}{
		"OpenAI API key": {account: &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, body: capacity, expectedCode: "server_is_overloaded"},
		"Grok OAuth":     {account: &Account{ID: 2, Platform: PlatformGrok, Type: AccountTypeOAuth}, body: capacity, expectedCode: "server_is_overloaded"},
		"non capacity":   {account: &Account{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body: nonCapacity, expectedCode: "server_error"},
	} {
		t.Run(name, func(t *testing.T) {
			rec, err := runOpenAIPassthroughCapacityStream(t, tt.account, tt.body)

			require.ErrorContains(t, err, "missing terminal event")
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(err, &failoverErr))
			require.Contains(t, rec.Body.String(), `"code":"`+tt.expectedCode+`"`)
		})
	}
}

func TestOpenAINativeBareCapacityAfterOutputRewritesRetryableError(t *testing.T) {
	body := `data: {"type":"response.output_text.delta","delta":"hello"}` + "\n\n" +
		"event: error\n" + `data: {"error":{"code":"server_is_overloaded"}}` + "\n\n"
	rec, _, err := runOpenAINativeCapacityStream(t, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body, 30)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Contains(t, rec.Body.String(), `"delta":"hello"`)
	require.Contains(t, rec.Body.String(), `"type":"response.failed"`)
	require.Contains(t, rec.Body.String(), `"code":"server_error"`)
	require.NotContains(t, strings.ToLower(rec.Body.String()), "server_is_overloaded")
	require.NotContains(t, strings.ToLower(rec.Body.String()), "overloaded")
}

func TestOpenAIPassthroughBareCapacityAfterOutputRewritesRetryableError(t *testing.T) {
	body := `data: {"type":"response.function_call_arguments.delta","delta":"{\"path\":\"a\"}"}` + "\n\n" +
		"event:response.failed\n" + `data: {"error":{"code":"slow_down"}}` + "\n\n"
	rec, err := runOpenAIPassthroughCapacityStream(t, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body)

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Contains(t, rec.Body.String(), `"type":"response.function_call_arguments.delta"`)
	require.Contains(t, rec.Body.String(), `"type":"response.failed"`)
	require.Contains(t, rec.Body.String(), `"code":"server_error"`)
	require.NotContains(t, strings.ToLower(rec.Body.String()), "slow_down")
}

func TestOpenAINativeStagedStructuralEventsEOFKeepHEADMissingTerminal(t *testing.T) {
	body := `data: {"type":"response.created","response":{"id":"resp_1"}}` + "\n\n" +
		`data: {"type":"response.output_item.added","item":{"id":"item_1","type":"message","content":[]}}` + "\n\n"
	for _, timeoutSeconds := range []int{0, 30} {
		rec, _, err := runOpenAINativeCapacityStream(t, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body, timeoutSeconds)

		require.Error(t, err)
		require.ErrorContains(t, err, "missing terminal event")
		var failoverErr *UpstreamFailoverError
		require.False(t, errors.As(err, &failoverErr))
		require.Contains(t, rec.Body.String(), `"type":"response.created"`)
		require.Contains(t, rec.Body.String(), `"type":"response.output_item.added"`)
		require.Equal(t, "upstream-request-id", rec.Header().Get("X-Request-Id"))
	}
}

func TestOpenAIPassthroughStagedStructuralEventsEOFKeepHEADMissingTerminal(t *testing.T) {
	body := `data: {"type":"response.created","response":{"id":"resp_1"}}` + "\n\n" +
		`data: {"type":"response.output_item.added","item":{"id":"item_1","type":"message","content":[]}}` + "\n\n"
	rec, err := runOpenAIPassthroughCapacityStream(t, &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, body)

	require.Error(t, err)
	require.ErrorContains(t, err, "missing terminal event")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.Contains(t, rec.Body.String(), `"type":"response.created"`)
	require.Contains(t, rec.Body.String(), `"type":"response.output_item.added"`)
	require.Equal(t, "upstream-request-id", rec.Header().Get("X-Request-Id"))
	require.Equal(t, "91", rec.Header().Get("X-Codex-Primary-Used-Percent"))
}

func runOpenAINativeCapacityStream(t *testing.T, account *Account, body string, firstOutputTimeoutSeconds int, repos ...AccountRepository) (*httptest.ResponseRecorder, bool, error) {
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
	if len(repos) > 0 {
		svc.accountRepo = repos[0]
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

func runOpenAIPassthroughCapacityStream(t *testing.T, account *Account, body string, repos ...AccountRepository) (*httptest.ResponseRecorder, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{MaxLineSize: defaultMaxLineSize}}}
	if len(repos) > 0 {
		svc.accountRepo = repos[0]
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":                 []string{"text/event-stream"},
			"X-Request-Id":                 []string{"upstream-request-id"},
			"X-Codex-Primary-Used-Percent": []string{"91"},
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
