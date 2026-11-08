package shared

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// CallSibling is how this process's own --mcp adapter reaches its
// long-running HTTP-mode sibling — every MCP tool registration
// (fetch_page_html, login, extract_page_data, crawl_paginated,
// find_login_elements, has_login_credential) is a thin wrapper that
// builds a request body and calls this.
func CallSibling(route string, body []byte) ([]byte, error) {
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
		}
		if err := json.Unmarshal(respBody, &cfErr); err == nil && cfErr.Code != "" {
			return nil, fmt.Errorf("%s (%s: %s)", cfErr.Message, cfErr.Code, cfErr.Reason)
		}
		return nil, fmt.Errorf("browser session returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}
