package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type cdpClient struct {
	conn    *websocket.Conn
	nextID  int
	errors  []string
	console []string
}

type cdpEnvelope struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func dialCDP(address string) (*cdpClient, error) {
	conn, _, err := (&websocket.Dialer{HandshakeTimeout: 5 * time.Second}).Dial(address, http.Header{"Origin": []string{"http://127.0.0.1"}})
	if err != nil {
		return nil, fmt.Errorf("connect to Chromium DevTools: %w", err)
	}
	return &cdpClient{conn: conn}, nil
}

func (c *cdpClient) close() {
	if c != nil {
		_ = c.conn.Close()
	}
}

func (c *cdpClient) call(ctx context.Context, method string, params any, result any) error {
	c.nextID++
	id := c.nextID
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := c.conn.WriteJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return fmt.Errorf("%s send: %w", method, err)
	}
	for {
		deadline := time.Now().Add(8 * time.Second)
		if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
			deadline = limit
		}
		_ = c.conn.SetReadDeadline(deadline)
		var message cdpEnvelope
		if err := c.conn.ReadJSON(&message); err != nil {
			return fmt.Errorf("%s response: %w", method, err)
		}
		if message.ID != id {
			if message.Method != "" {
				c.recordEvent(message)
			}
			continue
		}
		if message.Error != nil {
			return fmt.Errorf("%s: CDP %d: %s", method, message.Error.Code, message.Error.Message)
		}
		if result != nil && len(message.Result) != 0 {
			if err := json.Unmarshal(message.Result, result); err != nil {
				return fmt.Errorf("%s decode: %w", method, err)
			}
		}
		return nil
	}
}

func (c *cdpClient) recordEvent(message cdpEnvelope) {
	switch message.Method {
	case "Runtime.exceptionThrown":
		var details struct {
			ExceptionDetails struct {
				Text       string `json:"text"`
				URL        string `json:"url"`
				LineNumber int    `json:"lineNumber"`
				Exception  struct {
					Description string `json:"description"`
				} `json:"exception"`
			} `json:"exceptionDetails"`
		}
		if json.Unmarshal(message.Params, &details) == nil {
			description := details.ExceptionDetails.Exception.Description
			if description == "" {
				description = details.ExceptionDetails.Text
			}
			c.errors = append(c.errors, fmt.Sprintf("%s:%d %s", details.ExceptionDetails.URL, details.ExceptionDetails.LineNumber+1, description))
		}
	case "Runtime.consoleAPICalled":
		var call struct {
			Type string `json:"type"`
			Args []struct {
				Value       any    `json:"value"`
				Description string `json:"description"`
			} `json:"args"`
		}
		if json.Unmarshal(message.Params, &call) == nil {
			var parts []string
			for _, arg := range call.Args {
				if arg.Value != nil {
					parts = append(parts, fmt.Sprint(arg.Value))
				} else {
					parts = append(parts, arg.Description)
				}
			}
			c.console = append(c.console, call.Type+": "+strings.Join(parts, " "))
		}
	}
}

type page struct {
	name     string
	cdp      *cdpClient
	debugURL string
}

func (p *page) eval(ctx context.Context, expression string, output any) error {
	var reply struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := p.cdp.call(ctx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true, "awaitPromise": true}, &reply); err != nil {
		return err
	}
	if reply.ExceptionDetails != nil {
		description := reply.ExceptionDetails.Exception.Description
		if description == "" {
			description = reply.ExceptionDetails.Text
		}
		return fmt.Errorf("%s JavaScript: %s", p.name, description)
	}
	if output != nil {
		if len(reply.Result.Value) == 0 {
			return fmt.Errorf("%s JavaScript returned no value for %q", p.name, expression)
		}
		if err := json.Unmarshal(reply.Result.Value, output); err != nil {
			return fmt.Errorf("%s JavaScript result: %w", p.name, err)
		}
	}
	return nil
}

func (p *page) click(ctx context.Context, label string) error {
	quoted, _ := json.Marshal(label)
	expression := `(() => { const button = [...document.querySelectorAll('button')].find(x => x.textContent.trim() === ` + string(quoted) + `); if (!button || button.disabled) return false; button.click(); return true; })()`
	var clicked bool
	if err := p.eval(ctx, expression, &clicked); err != nil {
		return err
	}
	if !clicked {
		return fmt.Errorf("%s: button %q is absent or disabled", p.name, label)
	}
	return nil
}

func (p *page) startAutoplay(ctx context.Context, maximumRevision int) error {
	expression := fmt.Sprintf(`(() => {
		clearInterval(window.__trickE2EAutoplay);
		let last = -1;
		window.__trickE2EAutoplay = setInterval(() => {
			const raw = document.querySelector('.panel .row .tag')?.textContent;
			const revision = raw ? Number(raw.replace('Revision ', '')) : NaN;
			if (!Number.isFinite(revision) || revision === last || (%d > 0 && revision >= %d)) return;
			last = revision;
			const card = document.querySelector('.hand .card:not(:disabled)');
			if (card) card.click();
		}, 40);
		return true;
	})()`, maximumRevision, maximumRevision)
	var started bool
	if err := p.eval(ctx, expression, &started); err != nil {
		return err
	}
	if !started {
		return fmt.Errorf("%s autoplay did not start", p.name)
	}
	return nil
}

func (p *page) stopAutoplay(ctx context.Context) error {
	return p.eval(ctx, `clearInterval(window.__trickE2EAutoplay); window.__trickE2EAutoplay = null; true`, nil)
}
