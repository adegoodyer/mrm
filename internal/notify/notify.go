// Package notify sends the summary of a headless sync to Gotify and ntfy.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/adegoodyer/mrm/internal/config"
	"github.com/adegoodyer/mrm/internal/secret"
)

// Message is one notification.
type Message struct {
	Title    string
	Body     string
	Priority int // 0-10, Gotify scale
}

// Token resolves an endpoint's secret from token_env or token_cmd.
func Token(ctx context.Context, ep *config.Endpoint) (string, error) {
	return secret.Resolve(ctx, ep.TokenCmd, ep.TokenEnv)
}

// Send delivers msg to every configured endpoint and joins the errors.
func Send(ctx context.Context, n config.Notify, msg Message) error {
	var errs []error
	if n.Gotify != nil {
		if err := gotify(ctx, n.Gotify, msg); err != nil {
			errs = append(errs, fmt.Errorf("gotify: %w", err))
		}
	}
	if n.Ntfy != nil {
		if err := ntfy(ctx, n.Ntfy, msg); err != nil {
			errs = append(errs, fmt.Errorf("ntfy: %w", err))
		}
	}
	return errors.Join(errs...)
}

var client = &http.Client{Timeout: 15 * time.Second}

func gotify(ctx context.Context, ep *config.Endpoint, msg Message) error {
	token, err := Token(ctx, ep)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"title": msg.Title, "message": msg.Body, "priority": msg.Priority})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(ep.URL, "/")+"/message", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Gotify-Key", token) // header, not the query string, so it stays out of logs
	}
	return do(req)
}

func ntfy(ctx context.Context, ep *config.Endpoint, msg Message) error {
	token, err := Token(ctx, ep)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, strings.NewReader(msg.Body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", msg.Title)
	if msg.Priority >= 7 {
		req.Header.Set("Priority", "high")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return do(req)
}

func do(req *http.Request) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
