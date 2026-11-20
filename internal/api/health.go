package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

// Field is one extra scalar the server reported about itself.
type Field struct {
	Name  string
	Value string
}

// Health is what the server says about itself.
type Health struct {
	Status string
	// Version is empty when the server sent none.
	Version string
	// Extra holds every other top-level string, number or boolean, in the order the server sent them.
	Extra []Field
}

// Health asks the server for its status. The call needs no token.
func (c *Client) Health(ctx context.Context) (Health, error) {
	status, body, err := c.call(ctx, http.MethodGet, "/health", "", nil)
	if err != nil {
		var apiErr *Error
		if errors.As(err, &apiErr) && apiErr.Kind >= KindInvalidConnectKey && apiErr.Kind <= KindDeviceRejected {
			apiErr.Kind = KindFailure
		}
		return Health{}, err
	}
	health, ok := parseHealth(body)
	if !ok {
		return Health{}, unexpected(status)
	}
	return health, nil
}

// parseHealth reads a JSON object with a string status; version is optional, other scalars are kept
// in order and nested values are dropped.
func parseHealth(body []byte) (Health, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return Health{}, false
	}
	var health Health
	hasStatus := false
	for decoder.More() {
		keyToken, err := decoder.Token()
		key, isString := keyToken.(string)
		if err != nil || !isString {
			return Health{}, false
		}
		var value any
		if err := decoder.Decode(&value); err != nil {
			return Health{}, false
		}
		text, scalar := scalarText(value)
		switch {
		case key == "status":
			if _, isText := value.(string); !isText {
				return Health{}, false
			}
			health.Status, hasStatus = text, true
		case key == "version" && scalar:
			health.Version = text
		case scalar:
			health.Extra = append(health.Extra, Field{Name: key, Value: text})
		}
	}
	if _, err := decoder.Token(); err != nil || !hasStatus || health.Status == "" {
		return Health{}, false
	}
	return health, true
}

func scalarText(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case json.Number:
		return v.String(), true
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	}
	return "", false
}
