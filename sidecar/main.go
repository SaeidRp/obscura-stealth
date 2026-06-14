// Command obscura-sidecar exposes a tiny HTTP wrapper around `obscura fetch`.
//
// Why a wrapper: obscura only accepts a *per-request* proxy through the `fetch`
// CLI (`--proxy`), never through `serve`/CDP (its per-context proxy is a no-op
// stub). The backend can't shell out to docker, so this sidecar runs alongside
// the obscura binary and turns `POST /fetch {url, proxy, ...}` into a one-shot
// `obscura fetch` invocation — giving per-request proxy + per-request cookies
// (via --storage-dir) and process isolation per request.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const obscuraBin = "/usr/local/bin/obscura"

// fetchRequest is the JSON body accepted by POST /fetch.
type fetchRequest struct {
	URL         string            `json:"url"`
	Proxy       string            `json:"proxy"`
	UserAgent   string            `json:"user_agent"`
	WaitUntil   string            `json:"wait_until"`
	Timeout     int               `json:"timeout"`     // obscura nav timeout (seconds)
	Wait        int               `json:"wait"`        // extra settle wait (seconds)
	Timezone    string            `json:"timezone"`    // OBSCURA_TIMEZONE for this request
	Geolocation string            `json:"geolocation"` // OBSCURA_GEOLOCATION ("lat,lon")
	Cookies     []json.RawMessage `json:"cookies"`     // seeded verbatim into cookies.json
}

// fetchResponse is the JSON body returned by POST /fetch.
type fetchResponse struct {
	Status  int               `json:"status"`
	HTML    string            `json:"html"`
	Cookies []json.RawMessage `json:"cookies"`
	Error   string            `json:"error,omitempty"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("/fetch", handleFetch)

	port := getenv("SIDECAR_PORT", "9222")
	srv := &http.Server{
		Addr:        ":" + port,
		Handler:     mux,
		ReadTimeout: 30 * time.Second,
		// No WriteTimeout: a fetch can legitimately take a couple of minutes.
	}
	log.Printf("obscura-sidecar listening on :%s", port)
	log.Fatal(srv.ListenAndServe())
}

func handleFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, fetchResponse{Error: "POST only"})
		return
	}

	var req fetchRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, fetchResponse{Error: "invalid JSON body: " + err.Error()})
		return
	}
	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, fetchResponse{Error: "url is required"})
		return
	}

	html, cookies, err := runFetch(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, fetchResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, fetchResponse{Status: http.StatusOK, HTML: html, Cookies: cookies})
}

// runFetch invokes `obscura fetch` in an isolated storage dir and returns the
// rendered HTML plus the resulting cookies.
func runFetch(ctx context.Context, req fetchRequest) (string, []json.RawMessage, error) {
	storageDir, err := os.MkdirTemp("", "obscura-")
	if err != nil {
		return "", nil, errors.New("could not create storage dir: " + err.Error())
	}
	defer os.RemoveAll(storageDir)

	if len(req.Cookies) > 0 {
		if err := writeCookies(filepath.Join(storageDir, "cookies.json"), req.Cookies); err != nil {
			return "", nil, errors.New("could not seed cookies: " + err.Error())
		}
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 60
	}
	wait := req.Wait
	if wait < 0 {
		wait = 0
	}
	waitUntil := req.WaitUntil
	if waitUntil == "" {
		waitUntil = "networkidle0"
	}

	args := []string{
		"fetch", req.URL,
		"--stealth",
		"--dump", "html",
		"--storage-dir", storageDir,
		"--wait-until", waitUntil,
		"--timeout", strconv.Itoa(timeout),
		"--wait", strconv.Itoa(wait),
	}
	if req.Proxy != "" {
		args = append(args, "--proxy", req.Proxy)
	}
	if req.UserAgent != "" {
		args = append(args, "--user-agent", req.UserAgent)
	}

	// Give obscura the nav timeout plus a generous buffer before the hard kill.
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout+wait+30)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, obscuraBin, args...)
	cmd.Env = append(os.Environ(), buildEnv(req)...)

	var stdout, stderr capBuffer
	stdout.limit = 32 << 20 // 32 MiB ceiling on page HTML
	stderr.limit = 64 << 10
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = err.Error()
		}
		return "", nil, errors.New("obscura fetch failed: " + msg)
	}

	cookies := readCookies(filepath.Join(storageDir, "cookies.json"))

	return stdout.String(), cookies, nil
}

// buildEnv maps per-request stealth knobs to obscura's process env so the
// presented identity (timezone/geolocation) can match the proxy's region.
func buildEnv(req fetchRequest) []string {
	var env []string
	if req.Timezone != "" {
		env = append(env, "OBSCURA_TIMEZONE="+req.Timezone)
	}
	if req.Geolocation != "" {
		env = append(env, "OBSCURA_GEOLOCATION="+req.Geolocation)
	}
	return env
}

func writeCookies(path string, cookies []json.RawMessage) error {
	data, err := json.Marshal(cookies)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readCookies(path string) []json.RawMessage {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cookies []json.RawMessage
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil
	}
	return cookies
}

func writeJSON(w http.ResponseWriter, status int, body fetchResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// capBuffer is an io.Writer that retains at most `limit` bytes, so a runaway
// page can't exhaust memory.
type capBuffer struct {
	limit int
	buf   []byte
}

func (b *capBuffer) Write(p []byte) (int, error) {
	if room := b.limit - len(b.buf); room > 0 {
		if len(p) <= room {
			b.buf = append(b.buf, p...)
		} else {
			b.buf = append(b.buf, p[:room]...)
		}
	}
	return len(p), nil
}

func (b *capBuffer) String() string { return string(b.buf) }
