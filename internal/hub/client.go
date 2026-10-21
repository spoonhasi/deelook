// Package hub is a read-only client for the deemesh-hub HTTP API.
//
// deelook never writes to a machine, so this package deliberately has no
// way to send POST /machine/... requests.
package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ErrNoHub is every call's error while no hub address is set: deelook runs
// without deemesh (the HMI screens need it, sampling does not).
var ErrNoHub = errors.New("deemesh-hub 주소가 없습니다 (deemesh 안 씀: 화면 오른쪽 위에서 넣을 수 있음)")

// Envelope is one deemesh response: status 0 with a value, or a negative
// status with an error text. Desc and Unit are passed through as the hub
// sends them (desc is a string for one value, an array for a range read).
type Envelope struct {
	Status int             `json:"status"`
	Value  json.RawMessage `json:"value,omitempty"`
	Error  string          `json:"error,omitempty"`
	Desc   json.RawMessage `json:"desc,omitempty"`
	Unit   json.RawMessage `json:"unit,omitempty"`
}

func (e Envelope) OK() bool { return e.Status == 0 }

// Machine is the part of a /machineList entry deelook needs. Host and port
// are for deelook's own FOCAS2 sampling connection; they are not sent to
// browsers.
type Machine struct {
	ID       int    `json:"machine_id"`
	Name     string `json:"machine_name"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
}

type Client struct {
	mu   sync.Mutex
	base string // "" while deelook runs without a hub
	http *http.Client
}

// New returns a client for the hub at base (e.g. "http://127.0.0.1:8080";
// "" for none). requestTimeout bounds one HTTP round trip to the hub.
func New(base string, requestTimeout time.Duration) *Client {
	return &Client{
		base: strings.TrimRight(base, "/"),
		http: &http.Client{Timeout: requestTimeout},
	}
}

// Base is the hub's address, "" for none.
func (c *Client) Base() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.base
}

// SetBase points the client at another hub ("" for none); calls already
// under way finish with the old one.
func (c *Client) SetBase(base string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.base = strings.TrimRight(base, "/")
}

// url is the hub's address plus path, or ErrNoHub.
func (c *Client) url(path string) (string, error) {
	base := c.Base()
	if base == "" {
		return "", ErrNoHub
	}
	return base + path, nil
}

// ReadBatch reads many addresses in one round trip (POST /read/batch).
// The result holds one envelope per command, in order. A failed command is
// a negative envelope, not an error; err is set only when the batch itself
// did not go through. timeout is the hub-side limit for the whole batch.
func (c *Client) ReadBatch(ctx context.Context, commands []string, timeout time.Duration) ([]Envelope, error) {
	body, err := json.Marshal(map[string]any{
		"commands": commands,
		"timeout":  timeout.Milliseconds(),
	})
	if err != nil {
		return nil, err
	}
	u, err := c.url("/read/batch")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	var out struct {
		Status int        `json:"status"`
		Error  string     `json:"error"`
		Data   []Envelope `json:"data"`
	}
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(commands) {
		return nil, fmt.Errorf("hub batch refused (status %d): %s", out.Status, out.Error)
	}
	return out.Data, nil
}

// Read reads one address, e.g. "/machine/cncModel?machine=1".
func (c *Client) Read(ctx context.Context, command string) (Envelope, error) {
	u, err := c.url(command)
	if err != nil {
		return Envelope{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Envelope{}, err
	}
	var env Envelope
	err = c.do(req, &env)
	return env, err
}

// Machines returns the hub's machine list.
func (c *Client) Machines(ctx context.Context) ([]Machine, error) {
	env, err := c.Read(ctx, "/machineList")
	if err != nil {
		return nil, err
	}
	if !env.OK() {
		return nil, fmt.Errorf("hub /machineList: status %d: %s", env.Status, env.Error)
	}
	var ms []Machine
	if err := json.Unmarshal(env.Value, &ms); err != nil {
		return nil, fmt.Errorf("hub /machineList: %w", err)
	}
	return ms, nil
}

// do sends req and decodes the JSON answer into out. The hub answers its
// envelope even with HTTP 4xx/5xx (403 whitelist, 409 busy...), so the body
// is decoded whatever the HTTP status; only an undecodable body is an error.
func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("hub unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("hub read: %w", err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("hub HTTP %d: unexpected answer: %.200s", resp.StatusCode, raw)
	}
	return nil
}
