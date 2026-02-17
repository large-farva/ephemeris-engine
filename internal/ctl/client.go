package ctl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

// getJSON sends a GET request and decodes the JSON response into dst.
func getJSON(baseURL, path string, dst any) error {
	url := strings.TrimRight(baseURL, "/") + path
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(b))
		if msg != "" {
			return fmt.Errorf("HTTP %s: %s", resp.Status, msg)
		}
		return fmt.Errorf("HTTP %s from %s", resp.Status, path)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// getRaw sends a GET request and returns the raw response body.
func getRaw(baseURL, path string) (int, []byte, error) {
	url := strings.TrimRight(baseURL, "/") + path
	resp, err := httpClient.Get(url)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

// postJSON sends a POST request with a JSON body and decodes the response.
func postJSON(baseURL, path string, body, dst any) error {
	url := strings.TrimRight(baseURL, "/") + path
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(b)
	}
	resp, err := httpClient.Post(url, "application/json", reqBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Try to read an error message from the body.
		b, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(b))
		if msg != "" {
			return fmt.Errorf("HTTP %s: %s", resp.Status, msg)
		}
		return fmt.Errorf("HTTP %s from %s", resp.Status, path)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// decodeJSON decodes a JSON response body into dst. It checks the status code
// and returns an error with the body for non-2xx responses.
func decodeJSON(resp *http.Response, dst any) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(b))
		if msg != "" {
			return fmt.Errorf("HTTP %s: %s", resp.Status, msg)
		}
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

// printJSON prints v as indented JSON to stdout.
func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
