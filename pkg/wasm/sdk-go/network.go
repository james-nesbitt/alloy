//go:build tinygo || wasip1 || wasm
package wasm

import (
	"encoding/json"
	"fmt"
)

// ContentTypeJSON is the standard JSON content type.
const ContentTypeJSON = "application/json"

// PostJSON is a convenience wrapper for performing a JSON POST request.
func PostJSON[T any, R any](url string, headers map[string]string, body T) (*R, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("fetch: marshal request: %w", err)
	}

	if headers == nil {
		headers = make(map[string]string)
	}
	headers["Content-Type"] = ContentTypeJSON

	resp, err := Fetch(FetchRequest{
		Method:  "POST",
		URL:     url,
		Headers: headers,
		Body:    data,
	})
	if err != nil {
		return nil, err
	}

	if resp.Status < 200 || resp.Status >= 300 {
		return nil, fmt.Errorf("fetch: status %d: %s", resp.Status, string(resp.Body))
	}

	var result R
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("fetch: unmarshal response: %w", err)
	}

	return &result, nil
}

// GetJSON is a convenience wrapper for performing a JSON GET request.
func GetJSON[R any](url string, headers map[string]string) (*R, error) {
	resp, err := Fetch(FetchRequest{
		Method:  "GET",
		URL:     url,
		Headers: headers,
	})
	if err != nil {
		return nil, err
	}

	if resp.Status < 200 || resp.Status >= 300 {
		return nil, fmt.Errorf("fetch: status %d: %s", resp.Status, string(resp.Body))
	}

	var result R
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("fetch: unmarshal response: %w", err)
	}

	return &result, nil
}
