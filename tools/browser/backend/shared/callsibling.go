package shared

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
)

// CallSibling is how this process's own --mcp adapter reaches its
// long-running HTTP-mode sibling — every MCP tool registration
// (fetch_page_html, extract_page_data, crawl_paginated,
// find_login_elements) is a thin wrapper that
// builds a request body and calls this.
func CallSibling(route string, body []byte) ([]byte, error) {
	body = withAIContext(body, os.Getenv("TOOL_CONVERSATION_ID"))
	portStr := os.Getenv("TOOL_OWN_PORT")
	if portStr == "" {
		return nil, fmt.Errorf("browser session is not currently running — enable the tool first")
	}

	url := fmt.Sprintf("http://127.0.0.1:%s/%s", portStr, route)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("browser session is not reachable: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read browser session response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var cfErr struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Reason  string `json:"reason"`
			// Advice (step 96) says what the caller should do about it.
			Advice string `json:"advice"`
		}
		if err := json.Unmarshal(respBody, &cfErr); err == nil && cfErr.Code != "" {
			msg := fmt.Sprintf("%s (%s: %s)", cfErr.Message, cfErr.Code, cfErr.Reason)
			if cfErr.Advice != "" {
				msg += ". " + cfErr.Advice
			}
			return nil, errors.New(msg)
		}
		return nil, fmt.Errorf("browser session returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

// withAIContext sets the body's "aiContextId" to the conversation id core put in this process's environment (TOOL_CONVERSATION_ID, set by core
// for every AI tool call, plan 183). It is ALWAYS overwritten, or removed when there is none, so a value the model put in its arguments can never
// reach the running tool: the id comes from core only. A body that is not a JSON object is returned unchanged.
func withAIContext(body []byte, conversationID string) []byte {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil || obj == nil {
		return body
	}
	if conversationID == "" {
		if _, has := obj["aiContextId"]; !has {
			return body
		}
		delete(obj, "aiContextId")
	} else {
		quoted, err := json.Marshal(conversationID)
		if err != nil {
			return body
		}
		obj["aiContextId"] = quoted
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}
