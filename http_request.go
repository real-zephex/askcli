package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	maxHTTPResponseLength = 8000
	maxHTTPRedirects      = 5
	defaultHTTPTimeout    = 10
)

type httpRequestRequest struct {
	URL            string
	Method         string
	Headers        map[string]string
	Body           string
	TimeoutSeconds int
	Reason         string
}

type httpRequestResult struct {
	Request        httpRequestRequest
	StatusCode     int
	StatusText     string
	Headers        map[string]string
	Body           string
	BodyTruncated  bool
	ExecutionErr   string
	UserDenied     bool
}

func parseHTTPRequestRequest(args map[string]any) (httpRequestRequest, error) {
	req := httpRequestRequest{
		Method:         "GET",
		TimeoutSeconds: defaultHTTPTimeout,
		Headers:        make(map[string]string),
	}

	// Required: url
	url, err := requiredStringArg(args, "url")
	if err != nil {
		return req, err
	}
	req.URL = strings.TrimSpace(url)
	if req.URL == "" {
		return req, fmt.Errorf("url cannot be empty")
	}

	// Validate URL has scheme
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		return req, fmt.Errorf("url must include scheme (http:// or https://)")
	}

	// Optional: method
	if v, ok := args["method"]; ok {
		if s, ok := v.(string); ok {
			method := strings.ToUpper(strings.TrimSpace(s))
			// Validate method is one of the allowed values
			switch method {
			case "GET", "POST", "PUT", "PATCH", "DELETE":
				req.Method = method
			default:
				return req, fmt.Errorf("invalid method '%s': must be one of GET, POST, PUT, PATCH, DELETE", s)
			}
		}
	}

	// Optional: headers
	if v, ok := args["headers"]; ok {
		if headersMap, ok := v.(map[string]any); ok {
			for key, val := range headersMap {
				if strVal, ok := val.(string); ok {
					req.Headers[key] = strVal
				}
			}
		}
	}

	// Optional: body (ignored for GET and DELETE)
	if v, ok := args["body"]; ok {
		if s, ok := v.(string); ok {
			req.Body = s
		}
	}

	// Validate body is not used with GET or DELETE
	if req.Body != "" && (req.Method == "GET" || req.Method == "DELETE") {
		return req, fmt.Errorf("body parameter is not allowed for %s requests", req.Method)
	}

	// Optional: timeout_seconds
	if v, ok := args["timeout_seconds"]; ok {
		timeout, err := parseInt(v)
		if err != nil {
			return req, fmt.Errorf("timeout_seconds must be an integer: %w", err)
		}
		if timeout < 1 || timeout > 60 {
			return req, fmt.Errorf("timeout_seconds must be between 1 and 60, got %d", timeout)
		}
		req.TimeoutSeconds = timeout
	}

	// Optional: reason
	if v, ok := args["reason"]; ok {
		if s, ok := v.(string); ok {
			req.Reason = strings.TrimSpace(s)
		}
	}

	return req, nil
}

func executeHTTPRequest(req httpRequestRequest) httpRequestResult {
	res := httpRequestResult{Request: req}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(req.TimeoutSeconds)*time.Second)
	defer cancel()

	// Create HTTP request
	var bodyReader io.Reader
	if req.Body != "" {
		bodyReader = strings.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bodyReader)
	if err != nil {
		res.ExecutionErr = fmt.Sprintf("failed to create request: %v", err)
		return res
	}

	// Set headers
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	// Create client with redirect limit
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxHTTPRedirects {
				return fmt.Errorf("stopped after %d redirects", maxHTTPRedirects)
			}
			return nil
		},
	}

	// Execute request
	httpResp, err := client.Do(httpReq)
	if err != nil {
		// Check for specific error types
		if ctx.Err() == context.DeadlineExceeded {
			res.ExecutionErr = fmt.Sprintf("request timed out after %d seconds", req.TimeoutSeconds)
		} else if strings.Contains(err.Error(), "no such host") {
			res.ExecutionErr = fmt.Sprintf("DNS resolution failed: %v", err)
		} else if strings.Contains(err.Error(), "connection refused") {
			res.ExecutionErr = fmt.Sprintf("connection refused: %v", err)
		} else {
			res.ExecutionErr = fmt.Sprintf("request failed: %v", err)
		}
		return res
	}
	defer httpResp.Body.Close()

	// Capture status
	res.StatusCode = httpResp.StatusCode
	res.StatusText = httpResp.Status

	// Capture filtered headers
	res.Headers = make(map[string]string)
	allowedHeaders := []string{"Content-Type", "X-Request-Id", "Location", "WWW-Authenticate"}
	for _, header := range allowedHeaders {
		if value := httpResp.Header.Get(header); value != "" {
			res.Headers[header] = value
		}
	}

	// Read response body
	bodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		res.ExecutionErr = fmt.Sprintf("failed to read response body: %v", err)
		return res
	}

	bodyStr := string(bodyBytes)

	// Truncate if necessary
	if len(bodyStr) > maxHTTPResponseLength {
		bodyStr = bodyStr[:maxHTTPResponseLength]
		res.BodyTruncated = true
	}

	// Pretty-print JSON if Content-Type indicates JSON
	contentType := httpResp.Header.Get("Content-Type")
	if strings.Contains(contentType, "application/json") || strings.Contains(contentType, "text/json") {
		var jsonData any
		if err := json.Unmarshal([]byte(bodyStr), &jsonData); err == nil {
			prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
			if err == nil {
				bodyStr = string(prettyJSON)
				// Re-check length after pretty-printing
				if len(bodyStr) > maxHTTPResponseLength {
					bodyStr = bodyStr[:maxHTTPResponseLength]
					res.BodyTruncated = true
				}
			}
		}
	}

	res.Body = bodyStr

	return res
}

func (res httpRequestResult) toToolResponse() map[string]any {
	if res.ExecutionErr != "" {
		return map[string]any{
			"error": map[string]any{
				"message": res.ExecutionErr,
			},
		}
	}

	if res.UserDenied {
		return map[string]any{
			"error": map[string]any{
				"message": "request denied by user",
			},
		}
	}

	response := map[string]any{
		"status_code": res.StatusCode,
		"status_text": res.StatusText,
		"headers":     res.Headers,
		"body":        res.Body,
	}

	if res.BodyTruncated {
		response["body_truncated"] = true
		response["truncation_note"] = fmt.Sprintf("Response body was truncated to %d characters", maxHTTPResponseLength)
	}

	return response
}

func printHTTPRequestCall(req httpRequestRequest) {
	fmt.Printf("\n🌐 HTTP Request\n")
	fmt.Printf("   Method: %s\n", req.Method)
	fmt.Printf("   URL: %s\n", req.URL)
	if len(req.Headers) > 0 {
		fmt.Printf("   Headers:\n")
		for key, value := range req.Headers {
			fmt.Printf("     %s: %s\n", key, value)
		}
	}
	if req.Body != "" {
		fmt.Printf("   Body: %s\n", truncateForDisplay(req.Body, 200))
	}
	if req.Reason != "" {
		fmt.Printf("   Reason: %s\n", req.Reason)
	}
}

func askForHTTPRequestApproval() bool {
	fmt.Print("   Send request? [y/N]: ")
	return askYesNo()
}

func printHTTPRequestDenied() {
	fmt.Println("   ❌ Request denied by user")
}

func printHTTPRequestResult(res httpRequestResult) {
	if res.ExecutionErr != "" {
		fmt.Printf("   ❌ Error: %s\n", res.ExecutionErr)
		return
	}

	fmt.Printf("   ✓ Status: %s\n", res.StatusText)
	if len(res.Headers) > 0 {
		fmt.Printf("   Response Headers:\n")
		for key, value := range res.Headers {
			fmt.Printf("     %s: %s\n", key, value)
		}
	}
	if res.Body != "" {
		fmt.Printf("   Body: %s\n", truncateForDisplay(res.Body, 500))
	}
	if res.BodyTruncated {
		fmt.Printf("   ⚠️  Response truncated to %d characters\n", maxHTTPResponseLength)
	}
}

func truncateForDisplay(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// Made with Bob
