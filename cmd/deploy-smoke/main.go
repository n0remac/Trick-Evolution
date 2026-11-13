// deploy-smoke checks the deployed HTTP and two-seat WebSocket path.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type session struct {
	Room, Seat, Token string
}

type message struct {
	Type     string `json:"type"`
	Revision uint64 `json:"revision"`
}

func main() {
	base := flag.String("url", "http://127.0.0.1:18081", "HTTP(S) base URL")
	commit := flag.String("commit", "", "expected health commit")
	connectIP := flag.String("connect-ip", "", "connect to this IP while verifying the URL hostname and TLS certificate")
	flag.Parse()
	if err := check(*base, *commit, *connectIP); err != nil {
		fmt.Fprintln(os.Stderr, "deploy smoke:", err)
		os.Exit(1)
	}
	fmt.Println("HTTP health and two-client WebSocket start/reconnect passed")
}

func check(base, commit, connectIP string) error {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	wsDialer := &websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	if connectIP != "" {
		if net.ParseIP(connectIP) == nil {
			return fmt.Errorf("connect-ip must be an IP address")
		}
		dial := func(ctx context.Context, network, address string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(connectIP, port))
		}
		transport.Proxy = nil
		transport.DialContext = dial
		wsDialer.NetDialContext = dial
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	response, err := client.Get(strings.TrimRight(base, "/") + "/healthz")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var health struct{ Status, Commit string }
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&health) != nil || health.Status != "ok" || (commit != "" && health.Commit != commit) {
		return fmt.Errorf("health status %d, commit %q (expected %q)", response.StatusCode, health.Commit, commit)
	}
	room := fmt.Sprintf("deploy%d", time.Now().UnixNano())
	join := func(name string) (session, error) {
		body, _ := json.Marshal(map[string]string{"room": room, "name": name})
		resp, err := client.Post(strings.TrimRight(base, "/")+"/game/join", "application/json", bytes.NewReader(body))
		if err != nil {
			return session{}, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return session{}, fmt.Errorf("join %s: HTTP %d", name, resp.StatusCode)
		}
		var s session
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			return s, err
		}
		if s.Token == "" || s.Seat == "" {
			return s, fmt.Errorf("join %s omitted authority", name)
		}
		return s, nil
	}
	host, err := join("Deployment host")
	if err != nil {
		return err
	}
	guest, err := join("Deployment guest")
	if err != nil {
		return err
	}
	u, err := url.Parse(base)
	if err != nil {
		return err
	}
	scheme := "ws"
	if u.Scheme == "https" {
		scheme = "wss"
	}
	dial := func(s session) (*websocket.Conn, error) {
		address := scheme + "://" + u.Host + "/ws/game?room=" + url.QueryEscape(s.Room) + "&token=" + url.QueryEscape(s.Token)
		conn, response, err := wsDialer.Dial(address, http.Header{"Origin": []string{strings.TrimRight(base, "/")}})
		if err != nil {
			if response != nil {
				return nil, fmt.Errorf("websocket HTTP %d: %w", response.StatusCode, err)
			}
			return nil, err
		}
		return conn, nil
	}
	a, err := dial(host)
	if err != nil {
		return err
	}
	defer a.Close()
	b, err := dial(guest)
	if err != nil {
		return err
	}
	defer b.Close()
	read := func(c *websocket.Conn, wanted string, revision uint64) error {
		for i := 0; i < 12; i++ {
			_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
			var m message
			if err := c.ReadJSON(&m); err != nil {
				return err
			}
			if m.Type == wanted && m.Revision >= revision {
				return nil
			}
		}
		return fmt.Errorf("missing %s revision %d", wanted, revision)
	}
	if err := read(a, "lobby", 0); err != nil {
		return err
	}
	if err := read(b, "lobby", 0); err != nil {
		return err
	}
	if err := a.WriteJSON(map[string]string{"type": "start", "id": "deploy-start"}); err != nil {
		return err
	}
	if err := read(a, "state", 1); err != nil {
		return err
	}
	if err := read(b, "state", 1); err != nil {
		return err
	}
	_ = b.Close()
	b, err = dial(guest)
	if err != nil {
		return err
	}
	defer b.Close()
	return read(b, "state", 1)
}
