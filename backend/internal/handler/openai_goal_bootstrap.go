package handler

import "github.com/tidwall/gjson"

func normalizeCodexGoalBootstrap(body []byte) ([]byte, bool) {
	if !gjson.GetBytes(body, `input.#(name=="goal_objective")`).Exists() {
		return body, false
	}
	// Codex 0.169 sends the objective as agent context without a tool call ID.
	// Delegation context can accompany it; adapt both with the existing guards.
	return normalizeCodexCallOutputBootstrap(body, func(item map[string]any) bool {
		_, stringOutput := item["output"].(string)
		return (stringField(item, "type") == "function_call_output" &&
			stringField(item, "namespace") == "agent" &&
			stringField(item, "name") == "goal_objective" && stringOutput) ||
			isCodexDelegationCandidate(item)
	}, true)
}
