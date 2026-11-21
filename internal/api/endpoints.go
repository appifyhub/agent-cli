package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ConnectRequest is what the connect call sends. TTL is passed through untouched.
type ConnectRequest struct {
	ConnectKey string
	DeviceID   string
	Hostname   string
	OS         string
	TTL        string
	TTLSet     bool
}

// Connection is the answer to a successful connect.
type Connection struct {
	Token           string
	ExpiresAt       time.Time
	UserID          string
	UserDisplayName string
	// CreditBalance is nil when the backend sent none or something that is not a number.
	CreditBalance *float64
}

// Chat is a chat the user can send notifications to. Title is empty when the chat has none.
type Chat struct {
	ID       string
	Platform string
	Title    string
	Private  bool
}

// Delivery is the answer to a notification. A notification may already be sent and billed when the
// answer arrives, so any 2xx is a success: Chat is nil and CreditBalance is nil when the body is unusable.
type Delivery struct {
	Chat          *Chat
	CreditBalance *float64
}

// Status is the public backend and CLI version report. CLIVersion is empty when not advertised.
type Status struct {
	Status     string
	Version    string
	CLIVersion string
}

// Status reads the backend report without authenticating.
func (c *Client) Status(ctx context.Context) (Status, error) {
	status, body, err := c.call(ctx, http.MethodGet, "/cli/status", "", nil)
	if err != nil {
		var apiErr *Error
		if errors.As(err, &apiErr) && apiErr.Kind >= KindInvalidConnectKey && apiErr.Kind <= KindDeviceRejected {
			apiErr.Kind = KindFailure
		}
		return Status{}, err
	}
	if status != http.StatusOK {
		return Status{}, unexpected(status)
	}
	var fields map[string]json.RawMessage
	var result Status
	var version *string
	if json.Unmarshal(body, &fields) != nil ||
		json.Unmarshal(fields["status"], &result.Status) != nil || result.Status == "" ||
		json.Unmarshal(fields["service_version"], &version) != nil || version == nil {
		return Status{}, unexpected(status)
	}
	if raw, ok := fields["cli_version"]; ok && json.Unmarshal(raw, &result.CLIVersion) != nil {
		return Status{}, unexpected(status)
	}
	result.Version = *version
	return result, nil
}

type connectBody struct {
	ConnectKey string  `json:"connect_key"`
	DeviceID   string  `json:"device_id"`
	Hostname   string  `json:"hostname"`
	OS         string  `json:"os"`
	TTL        *string `json:"ttl_as_text,omitempty"`
}

// Connect redeems a connect key. The call is not authenticated and is never retried: the key is single-use.
func (c *Client) Connect(ctx context.Context, r ConnectRequest) (Connection, error) {
	var ttl *string
	if r.TTLSet || r.TTL != "" {
		ttl = &r.TTL
	}
	status, body, err := c.call(ctx, http.MethodPost, "/cli/connect", "", connectBody{
		ConnectKey: r.ConnectKey, DeviceID: r.DeviceID, Hostname: r.Hostname, OS: r.OS, TTL: ttl,
	})
	if err != nil {
		return Connection{}, err
	}
	var wire struct {
		Token           *string         `json:"token"`
		ExpiresAt       *string         `json:"expires_at"`
		UserID          *string         `json:"user_id"`
		UserDisplayName *string         `json:"user_display_name"`
		CreditBalance   json.RawMessage `json:"credit_balance"`
	}
	if json.Unmarshal(body, &wire) != nil || blank(wire.Token) || blank(wire.ExpiresAt) || blank(wire.UserID) || blank(wire.UserDisplayName) {
		return Connection{}, unexpected(status)
	}
	expires, err := time.Parse(time.RFC3339, *wire.ExpiresAt)
	if err != nil {
		return Connection{}, unexpected(status)
	}
	return Connection{
		Token:           *wire.Token,
		ExpiresAt:       expires.UTC(),
		UserID:          *wire.UserID,
		UserDisplayName: *wire.UserDisplayName,
		CreditBalance:   balance(wire.CreditBalance),
	}, nil
}

// RevokeAll removes every CLI token of the user. Any 2xx answer is success: the revocation has
// happened whatever the body says.
func (c *Client) RevokeAll(ctx context.Context, token, userID string) error {
	status, _, err := c.call(ctx, http.MethodDelete, "/cli/user/"+url.PathEscape(userID)+"/devices", token, nil)
	if status >= 200 && status < 300 {
		return nil
	}
	return err
}

// Chats lists the chats the user can notify.
func (c *Client) Chats(ctx context.Context, token string) ([]Chat, error) {
	status, body, err := c.call(ctx, http.MethodGet, "/cli/chats", token, nil)
	if err != nil {
		return nil, err
	}
	var wire []wireChat
	if json.Unmarshal(body, &wire) != nil || wire == nil {
		return nil, unexpected(status)
	}
	chats := make([]Chat, 0, len(wire))
	for _, w := range wire {
		chat, ok := w.chat()
		if !ok {
			return nil, unexpected(status)
		}
		chats = append(chats, chat)
	}
	return chats, nil
}

// NotifyUser sends a message to the user's private chat; an empty platform lets the backend choose.
func (c *Client) NotifyUser(ctx context.Context, token, userID, message, platform string) (Delivery, error) {
	var platformValue *string
	if platform != "" {
		platformValue = &platform
	}
	return c.notify(ctx, "/cli/notify/user/"+url.PathEscape(userID), token, struct {
		Message  string  `json:"message"`
		Platform *string `json:"platform"`
	}{message, platformValue})
}

// NotifyChat sends a message to a chat.
func (c *Client) NotifyChat(ctx context.Context, token, chatID, message string) (Delivery, error) {
	return c.notify(ctx, "/cli/notify/chat/"+url.PathEscape(chatID), token, struct {
		Message string `json:"message"`
	}{message})
}

func (c *Client) notify(ctx context.Context, path, token string, payload any) (Delivery, error) {
	status, body, err := c.call(ctx, http.MethodPost, path, token, payload)
	if err != nil && (status < 200 || status >= 300) {
		return Delivery{}, err
	}
	var delivery Delivery
	var wire struct {
		wireChat
		CreditBalance json.RawMessage `json:"credit_balance"`
	}
	if json.Unmarshal(bytes.TrimSpace(body), &wire) == nil {
		if chat, ok := wire.chat(); ok {
			delivery.Chat = &chat
		}
		delivery.CreditBalance = balance(wire.CreditBalance)
	}
	return delivery, nil
}

type wireChat struct {
	ChatID   *string `json:"chat_id"`
	Platform *string `json:"platform"`
	Title    *string `json:"title"`
	Private  *bool   `json:"is_private"`
}

func (w wireChat) chat() (Chat, bool) {
	if blank(w.ChatID) || blank(w.Platform) || w.Private == nil {
		return Chat{}, false
	}
	chat := Chat{ID: *w.ChatID, Platform: *w.Platform, Private: *w.Private}
	if w.Title != nil {
		chat.Title = *w.Title
	}
	return chat, true
}

func blank(s *string) bool { return s == nil || strings.TrimSpace(*s) == "" }

// balance reads credit_balance only when it is a JSON number; anything else counts as absent.
func balance(raw json.RawMessage) *float64 {
	if len(raw) == 0 || (raw[0] != '-' && (raw[0] < '0' || raw[0] > '9')) {
		return nil
	}
	var n float64
	if json.Unmarshal(raw, &n) != nil {
		return nil
	}
	return &n
}
