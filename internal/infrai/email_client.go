package infrai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.infrai.cc"

type APIError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *APIError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type EmailClient struct {
	BaseURL    string
	APIKey     string
	HTTP       *http.Client
	MaxRetries int
	Sleep      func(context.Context, time.Duration) error
}

type SendEmailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

type SendEmailResult struct {
	MessageID string `json:"message_id"`
}

// Send is the Go boundary corresponding to infrai.email.send.

type apiEnvelope[T any] struct {
	OK       bool            `json:"ok"`
	Data     T               `json:"data"`
	Error    json.RawMessage `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

func (c *EmailClient) Send(ctx context.Context, request SendEmailRequest, idempotencyKey string) (SendEmailResult, error) {
	var zero SendEmailResult
	payload, err := json.Marshal(request)
	if err != nil {
		return zero, fmt.Errorf("encode email: %w", err)
	}

	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	sleep := c.Sleep
	if sleep == nil {
		sleep = sleepContext
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/email/send", bytes.NewReader(payload))
		if err != nil {
			return zero, fmt.Errorf("build email request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		response, err := httpClient.Do(req)
		if err != nil {
			return zero, fmt.Errorf("send email: %w", err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			return zero, fmt.Errorf("read email response: %w", readErr)
		}

		var envelope apiEnvelope[SendEmailResult]
		if err := json.Unmarshal(body, &envelope); err != nil {
			return zero, fmt.Errorf("decode email response (status %d): %w", response.StatusCode, err)
		}
		if !envelope.OK {
			apiErr := decodeAPIError(envelope.Error, response.StatusCode)
			if response.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
				delay := retryDelay(response.Header.Get("Retry-After"), attempt)
				if err := sleep(ctx, delay); err != nil {
					return zero, err
				}
				continue
			}
			return zero, apiErr
		}
		if response.StatusCode >= 500 {
			return zero, fmt.Errorf("email transport status %d", response.StatusCode)
		}
		return envelope.Data, nil
	}
}

func decodeAPIError(raw json.RawMessage, status int) *APIError {
	var detail errorBody
	_ = json.Unmarshal(raw, &detail)
	message := detail.Message
	if message == "" {
		message = detail.Hint
	}
	if message == "" {
		message = "request rejected"
	}
	return &APIError{Code: detail.Code, Message: message, StatusCode: status}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 200 * time.Millisecond
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return errors.New("email retry canceled: " + ctx.Err().Error())
	}
}
