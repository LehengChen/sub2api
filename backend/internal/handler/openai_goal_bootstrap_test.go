package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// Captured from Codex 0.169.0 against a localhost provider with a dummy key.
const goalBootstrapItem = `{"type":"function_call_output","namespace":"agent","name":"goal_objective","output":"Reply with OK."}`

func TestNormalizeCodexGoalBootstrap(t *testing.T) {
	for _, callID := range []string{"", `,"call_id":""`, `,"call_id":"  "`} {
		body := []byte(`{"model":"gpt-6.1-sol","metadata":{"integer":9007199254740993},"input":[{"type":"message","role":"user","content":"before"},{"type":"function_call_output","namespace":"agent","name":"goal_objective","output":"Reply with OK."` + callID + `},{"type":"message","role":"user","content":"<goal_context>Continue.</goal_context>"}]}`)
		got, changed := normalizeCodexGoalBootstrap(body)
		require.True(t, changed)
		require.Equal(t, "before", gjson.GetBytes(got, "input.0.content").String())
		require.Equal(t, "message", gjson.GetBytes(got, "input.1.type").String())
		require.Equal(t, "user", gjson.GetBytes(got, "input.1.role").String())
		require.Equal(t, "Reply with OK.", gjson.GetBytes(got, "input.1.content.0.text").String())
		require.Equal(t, "<goal_context>Continue.</goal_context>", gjson.GetBytes(got, "input.2.content").String())
		require.Equal(t, "9007199254740993", gjson.GetBytes(got, "metadata.integer").Raw)
		again, changedAgain := normalizeCodexGoalBootstrap(got)
		require.False(t, changedAgain)
		require.Equal(t, got, again)
	}
}

func TestNormalizeCodexGoalBootstrapWithHistoryAndDelegation(t *testing.T) {
	body := []byte(`{"model":"gpt-6.1-sol","previous_response_id":"resp-1","input":[{"type":"function_call","call_id":"call-1","name":"create_goal","arguments":"{}"},{"type":"function_call_output","call_id":"call-1","output":"done"},` + goalBootstrapItem + `,{"type":"function_call_output","namespace":"codex_tui","name":"send_message_to_thread","output":"` + delegationEnvelope + `"},{"type":"item_reference","id":"item-1"}]}`)
	got, changed := normalizeCodexGoalBootstrap(body)
	require.True(t, changed)
	require.Equal(t, "resp-1", gjson.GetBytes(got, "previous_response_id").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "input.0.call_id").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "input.1.call_id").String())
	require.Equal(t, "done", gjson.GetBytes(got, "input.1.output").String())
	require.Equal(t, "Reply with OK.", gjson.GetBytes(got, "input.2.content.0.text").String())
	require.Equal(t, delegationEnvelope, gjson.GetBytes(got, "input.3.content.0.text").String())
	require.Equal(t, "item-1", gjson.GetBytes(got, "input.4.id").String())
}

func TestNormalizeCodexGoalBootstrapRejectsOtherOrAmbiguousItems(t *testing.T) {
	cases := []string{
		`{"type":"function_call_output","namespace":"other","name":"goal_objective","output":"goal"}`,
		`{"type":"function_call_output","namespace":"agent","name":"other","output":"goal"}`,
		`{"type":"custom_tool_call_output","namespace":"agent","name":"goal_objective","output":"goal"}`,
		`{"type":"function_call_output","namespace":"agent","name":"goal_objective","output":{}}`,
		`{"type":"function_call_output","namespace":"agent","name":"goal_objective","call_id":"call-real","output":"goal"}`,
		`{"type":"function_call_output","namespace":"agent","name":"goal_objective","call_id":null,"output":"goal"}`,
		`{"type":"function_call_output","namespace":"agent","name":"goal_objective","call_id":42,"output":"goal"}`,
		`{"type":"function_call_output","namespace":"agent","name":"goal_objective","call_id":"call-real","call_id":"","output":"goal"}`,
		`{"type":"function_call_output","namespace":"agent","namespace":"other","name":"goal_objective","output":"goal"}`,
		goalBootstrapItem + `,{"type":"function_call_output","output":"unknown result"}`,
		goalBootstrapItem + `,{"type":"computer_call"}`,
		goalBootstrapItem + `,{"type":"item_reference","id":""}`,
	}
	for _, item := range cases {
		body := []byte(`{"model":"gpt-6.1-sol","input":[` + item + `]}`)
		got, changed := normalizeCodexGoalBootstrap(body)
		require.False(t, changed, item)
		require.Equal(t, body, got)
	}
}

func TestOpenAIResponsesGoalBootstrapPassesToolValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-6.1-sol","input":[` + goalBootstrapItem + `]}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h := &OpenAIGatewayHandler{}
	require.False(t, h.validateFunctionCallOutputRequest(c, body, zap.NewNop()))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "function_call_output requires call_id")

	normalized, changed := normalizeCodexGoalBootstrap(body)
	require.True(t, changed)
	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	require.True(t, h.validateFunctionCallOutputRequest(c, normalized, zap.NewNop()))
	require.False(t, w.Flushed)
}

type goalHTTPUpstream struct {
	service.HTTPUpstream
	target *url.URL
}

func (u *goalHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	forward := req.Clone(req.Context())
	forward.URL.Scheme, forward.URL.Host = u.target.Scheme, u.target.Host
	return http.DefaultClient.Do(forward)
}

func newGoalResponsesTestServer(t *testing.T, reply func([]byte) string) (*httptest.Server, <-chan []byte) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	payloads := make(chan []byte, 16)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		select {
		case payloads <- body:
		default:
			http.Error(w, "test request limit reached", http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, reply(body))
	}))
	t.Cleanup(upstream.Close)
	target, err := url.Parse(upstream.URL)
	require.NoError(t, err)
	accountRepo := &openAIWSUsageHandlerAccountRepoStub{account: service.Account{
		ID: 9901, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Status: service.StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "local-dummy-token", "chatgpt_account_id": "local-dummy-account"},
	}}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Default.RateMultiplier = 1
	billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billing.Stop)
	usageRepo := &openAIWSUsageHandlerUsageLogRepoStub{created: make(chan *service.UsageLog, 16)}
	gateway := service.NewOpenAIGatewayService(accountRepo, usageRepo, nil, nil, nil, nil, nil, cfg,
		nil, nil, service.NewBillingService(cfg, nil), nil, billing,
		&goalHTTPUpstream{target: target}, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
	cache := &concurrencyCacheMock{
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	h := &OpenAIGatewayHandler{
		cfg: cfg, gatewayService: gateway, billingCacheService: billing, apiKeyService: &service.APIKeyService{},
		concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second),
	}
	groupID := int64(16)
	key := &service.APIKey{ID: 1801, GroupID: &groupID, User: &service.User{ID: 1701, Status: service.StatusActive}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(middleware.ContextKeyAPIKey), key)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: key.User.ID, Concurrency: 1})
		c.Next()
	})
	router.POST("/v1/responses", h.Responses)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server, payloads
}

const goalCompletedSSE = `event: response.output_item.done
data: {"type":"response.output_item.done","output_index":0,"item":{"id":"msg_goal_test","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"OK","annotations":[]}]}}

event: response.completed
data: {"type":"response.completed","response":{"id":"resp_goal_test","model":"gpt-6.1-sol","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":10,"output_tokens":1,"total_tokens":11}}}

`

func TestOpenAIResponsesGoalBootstrapOAuthHTTP(t *testing.T) {
	server, payloads := newGoalResponsesTestServer(t, func([]byte) string { return goalCompletedSSE })
	cases := []struct {
		name  string
		input string
		code  int
	}{
		{"ordinary", `{"type":"message","role":"user","content":"Reply with OK."}`, http.StatusOK},
		{"goal", goalBootstrapItem, http.StatusOK},
		{"goal and tool history", `{"type":"function_call","name":"create_goal","call_id":"call-real","arguments":"{}"},{"type":"function_call_output","call_id":"call-real","output":"done"},` + goalBootstrapItem, http.StatusOK},
		{"goal and delegation", goalBootstrapItem + `,{"type":"function_call_output","namespace":"codex_tui","name":"create_thread","output":"` + delegationEnvelope + `"}`, http.StatusOK},
		{"unknown tool output", `{"type":"function_call_output","namespace":"agent","name":"unknown","output":"other"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"model":"gpt-6.1-sol","stream":true,"input":[` + tc.input + `]}`
			req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/responses", strings.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "codex_cli_rs/0.169.0")
			response, err := server.Client().Do(req)
			require.NoError(t, err)
			defer response.Body.Close()
			result, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, tc.code, response.StatusCode, string(result))
			if tc.code != http.StatusOK {
				require.Empty(t, payloads, "invalid tool output must not reach an account")
				return
			}
			require.Contains(t, string(result), "response.completed")
			select {
			case forwarded := <-payloads:
				require.Equal(t, "gpt-6.1-sol", gjson.GetBytes(forwarded, "model").String())
				require.False(t, gjson.GetBytes(forwarded, `input.#(name=="goal_objective")`).Exists())
				if tc.name == "goal and tool history" {
					callID := gjson.GetBytes(forwarded, `input.#(type=="function_call").call_id`).String()
					require.NotEmpty(t, callID)
					require.Equal(t, callID, gjson.GetBytes(forwarded, `input.#(type=="function_call_output").call_id`).String())
					require.Equal(t, "done", gjson.GetBytes(forwarded, `input.#(type=="function_call_output").output`).String())
				}
			case <-time.After(3 * time.Second):
				t.Fatal("request did not reach the OAuth upstream")
			}
		})
	}
}
