package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"raglex/internal/model"
)

// Mailer sends transactional emails for Raglex Cloud accounts.
type Mailer interface {
	Enabled() bool
	Send(ctx context.Context, to, subject, text, htmlBody string) error
}

// OpenMailClient sends transactional email through OpenMail (https://api.openmail.sh).
type OpenMailClient struct {
	BaseURL     string
	APIKey      string
	FromAddress string

	mu      sync.Mutex
	inboxID string
	hc      *http.Client
}

// NewOpenMailClient creates a new OpenMail API sender. If inboxID is empty, it
// will automatically resolve the inbox matching fromAddress via GET /v1/inboxes.
func NewOpenMailClient(apiKey, inboxID, fromAddress string) *OpenMailClient {
	return &OpenMailClient{
		BaseURL:     "https://api.openmail.sh",
		APIKey:      strings.TrimSpace(apiKey),
		FromAddress: strings.TrimSpace(fromAddress),
		inboxID:     strings.TrimSpace(inboxID),
		hc:          &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *OpenMailClient) Enabled() bool {
	return c != nil && c.APIKey != ""
}

func (c *OpenMailClient) resolveInboxID(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.inboxID != "" {
		id := c.inboxID
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+"/v1/inboxes", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("openmail list inboxes HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		Data []struct {
			ID      string `json:"id"`
			Address string `json:"address"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	var chosen string
	for _, inb := range out.Data {
		if c.FromAddress == "" || strings.EqualFold(inb.Address, c.FromAddress) {
			chosen = inb.ID
			break
		}
	}
	if chosen == "" && len(out.Data) > 0 {
		chosen = out.Data[0].ID
	}
	if chosen == "" {
		return "", errors.New("openmail: no inbox found for API key")
	}
	c.mu.Lock()
	c.inboxID = chosen
	c.mu.Unlock()
	return chosen, nil
}

func (c *OpenMailClient) Send(ctx context.Context, to, subject, text, htmlBody string) error {
	if !c.Enabled() {
		return errors.New("openmail: mailer not configured")
	}
	inboxID, err := c.resolveInboxID(ctx)
	if err != nil {
		return err
	}
	payload := map[string]string{
		"to":      strings.TrimSpace(to),
		"subject": subject,
		"body":    text,
	}
	if strings.TrimSpace(htmlBody) != "" {
		payload["html"] = htmlBody
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/v1/inboxes/%s/send", strings.TrimRight(c.BaseURL, "/"), url.PathEscape(inboxID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "rglx-"+model.NewID())

	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusBadRequest && payload["html"] != "" {
		// Retry without optional html field in case API rejects extra keys.
		_, _ = io.Copy(io.Discard, resp.Body)
		delete(payload, "html")
		raw2, _ := json.Marshal(payload)
		req2, err2 := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw2))
		if err2 != nil {
			return err2
		}
		req2.Header.Set("Authorization", "Bearer "+c.APIKey)
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Idempotency-Key", "rglx-"+model.NewID())
		resp2, err2 := c.hc.Do(req2)
		if err2 != nil {
			return err2
		}
		defer resp2.Body.Close()
		if resp2.StatusCode < 200 || resp2.StatusCode >= 300 {
			body2, _ := io.ReadAll(io.LimitReader(resp2.Body, 4096))
			return fmt.Errorf("openmail send HTTP %d: %s", resp2.StatusCode, strings.TrimSpace(string(body2)))
		}
		_, _ = io.Copy(io.Discard, resp2.Body)
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("openmail send HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func renderEmailShell(title, subtitle, code, actionLabel, actionURL, footerNote string) string {
	btnHTML := ""
	if actionLabel != "" && actionURL != "" {
		btnHTML = fmt.Sprintf(`<div style="margin:22px 0 8px"><a href="%s" style="display:inline-block;background:#f4f4f5;color:#111114;text-decoration:none;font-weight:600;font-size:14px;padding:11px 22px;border-radius:10px">%s</a></div>`,
			html.EscapeString(actionURL), html.EscapeString(actionLabel))
	}
	codeHTML := ""
	if code != "" {
		codeHTML = fmt.Sprintf(`<div style="margin:18px 0;padding:16px 18px;border-radius:12px;background:#0c0c0f;border:1px solid rgba(255,255,255,0.1);font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:26px;font-weight:700;letter-spacing:6px;color:#ffffff;text-align:center">%s</div>`,
			html.EscapeString(code))
	}
	return fmt.Sprintf(`<!doctype html><html><body style="margin:0;padding:28px 16px;background:#0b0b0e;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#f4f4f5">
  <div style="max-width:460px;margin:0 auto;background:#141419;border:1px solid rgba(255,255,255,0.09);border-radius:16px;padding:28px 28px 24px">
    <div style="font-size:12px;font-weight:600;letter-spacing:.04em;color:#ff5c33;margin-bottom:10px">RAGLEX CLOUD</div>
    <h1 style="margin:0 0 8px;font-size:20px;line-height:1.35;color:#ffffff">%s</h1>
    <p style="margin:0 0 6px;font-size:14px;line-height:1.6;color:#a1a1aa">%s</p>
    %s
    %s
    <p style="margin:20px 0 0;padding-top:16px;border-top:1px solid rgba(255,255,255,0.07);font-size:12px;line-height:1.5;color:#71717a">%s</p>
  </div>
</body></html>`,
		html.EscapeString(title),
		html.EscapeString(subtitle),
		codeHTML,
		btnHTML,
		html.EscapeString(footerNote),
	)
}
