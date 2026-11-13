package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type browser struct {
	command *exec.Cmd
	profile string
	logFile *os.File
	port    int
	control *cdpClient
	pages   []*page
}

func browserBinary(explicit string) (string, error) {
	if explicit != "" {
		return exec.LookPath(explicit)
	}
	for _, candidate := range []string{"chromium", "chromium-browser", "google-chrome", "chrome"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("Chromium was not found; pass -browser /path/to/chrome")
}

func launchBrowser(ctx context.Context, executable, artifacts string) (*browser, error) {
	profile, err := os.MkdirTemp("", "trick-evolution-d1-browser-")
	if err != nil {
		return nil, err
	}
	logFile, err := os.Create(filepath.Join(artifacts, "chromium.log"))
	if err != nil {
		_ = os.RemoveAll(profile)
		return nil, err
	}
	args := []string{
		"--headless=new", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
		"--disable-background-networking", "--disable-component-update", "--disable-default-apps", "--disable-sync",
		"--disable-background-timer-throttling", "--disable-backgrounding-occluded-windows",
		"--disable-renderer-backgrounding", "--disable-features=IntensiveWakeUpThrottling",
		"--remote-debugging-port=0", "--remote-allow-origins=*", "--no-first-run", "--no-default-browser-check",
		"--user-data-dir=" + profile, "about:blank",
	}
	command := exec.Command(executable, args...)
	command.Stdout, command.Stderr = logFile, logFile
	if err := command.Start(); err != nil {
		_ = logFile.Close()
		_ = os.RemoveAll(profile)
		return nil, fmt.Errorf("start Chromium: %w", err)
	}
	b := &browser{command: command, profile: profile, logFile: logFile}
	deadline := time.Now().Add(30 * time.Second)
	activePort := filepath.Join(profile, "DevToolsActivePort")
	var wsPath string
	for time.Now().Before(deadline) && ctx.Err() == nil {
		data, err := os.ReadFile(activePort)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) >= 2 {
				b.port, err = strconv.Atoi(lines[0])
				if err == nil {
					wsPath = lines[1]
					break
				}
			}
		}
		time.Sleep(75 * time.Millisecond)
	}
	if wsPath == "" {
		b.close()
		return nil, fmt.Errorf("Chromium did not publish DevToolsActivePort within 30 seconds; see chromium.log")
	}
	b.control, err = dialCDP(fmt.Sprintf("ws://127.0.0.1:%d%s", b.port, wsPath))
	if err != nil {
		b.close()
		return nil, err
	}
	return b, nil
}

func (b *browser) close() {
	for _, p := range b.pages {
		p.cdp.close()
	}
	if b.control != nil {
		b.control.close()
	}
	if b.command != nil && b.command.Process != nil {
		_ = b.command.Process.Kill()
		_ = b.command.Wait()
	}
	if b.logFile != nil {
		_ = b.logFile.Close()
	}
	if b.profile != "" {
		_ = os.RemoveAll(b.profile)
	}
}

func (b *browser) newPage(ctx context.Context, name, address string) (*page, error) {
	var contextResult struct {
		ID string `json:"browserContextId"`
	}
	if err := b.control.call(ctx, "Target.createBrowserContext", map[string]any{}, &contextResult); err != nil {
		return nil, err
	}
	var targetResult struct {
		ID string `json:"targetId"`
	}
	if err := b.control.call(ctx, "Target.createTarget", map[string]any{"url": address, "browserContextId": contextResult.ID}, &targetResult); err != nil {
		return nil, err
	}
	var wsAddress string
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline) && ctx.Err() == nil; {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/json/list", b.port), nil)
		response, err := (&http.Client{Timeout: 3 * time.Second}).Do(request)
		if err == nil {
			var targets []struct {
				ID           string `json:"id"`
				WebSocketURL string `json:"webSocketDebuggerUrl"`
			}
			decodeErr := json.NewDecoder(response.Body).Decode(&targets)
			_ = response.Body.Close()
			if decodeErr == nil {
				for _, target := range targets {
					if target.ID == targetResult.ID {
						wsAddress = target.WebSocketURL
						break
					}
				}
			}
		}
		if wsAddress != "" {
			break
		}
		time.Sleep(75 * time.Millisecond)
	}
	if wsAddress == "" {
		return nil, fmt.Errorf("DevTools page %s did not appear", targetResult.ID)
	}
	cdp, err := dialCDP(wsAddress)
	if err != nil {
		return nil, err
	}
	p := &page{name: name, cdp: cdp, debugURL: wsAddress}
	b.pages = append(b.pages, p)
	if err := cdp.call(ctx, "Runtime.enable", map[string]any{}, nil); err != nil {
		return nil, err
	}
	if err := cdp.call(ctx, "Page.enable", map[string]any{}, nil); err != nil {
		return nil, err
	}
	return p, nil
}
