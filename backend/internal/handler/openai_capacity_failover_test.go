//go:build unit

package handler

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIOAuthCapacitySwitchesToDifferentAccount(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"429", 429, `{"detail":"Rate limit exceeded"}`},
		{"503", 503, `{"error":{"message":"temporarily unavailable"}}`},
		{"HTTP overload", 503, `{"error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`},
		{"HTTP concurrency without code", 400, `{"error":{"message":"Concurrency limit exceeded for account, please retry later"}}`},
		{"SSE concurrency without code", 200, "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"Concurrency limit exceeded for account, please retry later\"}}}\n\n"},
		{"SSE overload", 200, "data: {\"type\":\"error\",\"error\":{\"code\":\"server_is_overloaded\",\"message\":\"overloaded\"}}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			contentType := "application/json"
			if tc.status == 200 {
				contentType = "text/event-stream"
			}
			upstream := newAstraProCapturedUpstream(&http.Response{
				StatusCode: tc.status,
				Header:     http.Header{"Content-Type": []string{contentType}, "Retry-After": []string{"90"}},
				Body:       io.NopCloser(strings.NewReader(tc.body)),
			}, astra200())
			h := newOpenAIResponsesFailoverTestHandler(t, upstream)
			c, rec := newAstraProFailoverContext(t, astraProRequestBody)
			h.Responses(c)
			urls, ids, bodies := upstream.snapshot()
			assertAstraProAccountSwitch(t, ids)
			assertAstraProWire(t, urls, bodies)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "completed", gjson.GetBytes(rec.Body.Bytes(), "status").String())
		})
	}
}

func TestOpenAIOAuthCapacityExhaustsWithoutReturningToFirstAccount(t *testing.T) {
	for _, status := range []int{429, 503} {
		response := func() *http.Response {
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"temporarily unavailable"}}`))}
		}
		upstream := newAstraProCapturedUpstream(response(), response())
		h := newOpenAIResponsesFailoverTestHandler(t, upstream)
		c, rec := newAstraProFailoverContext(t, astraProRequestBody)
		h.Responses(c)
		_, ids, _ := upstream.snapshot()
		assertAstraProAccountSwitch(t, ids)
		require.GreaterOrEqual(t, rec.Code, 400)
	}
}
