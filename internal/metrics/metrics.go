// Package metrics renders a live, one-line resource gauge (memory, CPU, elapsed
// time) for a running sandbox container. It is used only for non-interactive
// runs — during an interactive agent TUI the container owns the terminal, so no
// bar is drawn.
//
// The gauge is a "sticky footer": forwarded container output scrolls above while
// the status line stays pinned to the bottom, redrawn on each output write and
// on a timer. Everything is guarded by a single mutex so output and status
// never clobber each other.
package metrics

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TermFooter keeps a status line pinned to the bottom of a terminal while other
// output scrolls above it.
type TermFooter struct {
	term   *os.File // terminal the footer is drawn on (typically os.Stderr)
	mu     sync.Mutex
	status string
	shown  bool
}

// NewTermFooter returns a footer that draws on term.
func NewTermFooter(term *os.File) *TermFooter { return &TermFooter{term: term} }

// eraseLocked clears the current footer line (caller holds mu).
func (f *TermFooter) eraseLocked() {
	if f.shown {
		fmt.Fprint(f.term, "\r\033[K")
	}
}

// drawLocked writes the footer text without a trailing newline (caller holds mu).
func (f *TermFooter) drawLocked() {
	if f.status != "" {
		fmt.Fprint(f.term, f.status)
		f.shown = true
	}
}

// Print forwards a chunk of container output to dst, keeping the footer pinned.
func (f *TermFooter) Print(dst *os.File, p []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.eraseLocked()
	f.shown = false
	dst.Write(p)
	f.drawLocked()
}

// SetStatus updates and redraws the footer text.
func (f *TermFooter) SetStatus(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = s
	f.eraseLocked()
	f.shown = false
	f.drawLocked()
}

// Finish erases the footer, leaving the cursor on a clean line.
func (f *TermFooter) Finish() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.eraseLocked()
	f.status = ""
	f.shown = false
}

// reading is the latest sampled resource usage.
type reading struct {
	usedBytes float64 // memory used, bytes
	memText   string  // e.g. "512MiB / 7.6GiB"
	memFrac   float64 // used/limit, 0 if unknown
	cpu       string  // e.g. "82.00%"
	cpuVal    float64 // parsed CPU percent
	ok        bool
}

// Meter samples a container's resource usage. With a non-nil footer it also
// draws a live gauge; with a nil footer it samples silently (used to compute a
// post-run summary during an interactive session).
type Meter struct {
	dockerBin string
	name      string
	start     time.Time
	footer    *TermFooter // nil => silent (no live gauge)

	mu          sync.Mutex
	cur         reading
	peakBytes   float64 // peak memory used, bytes
	peakMemText string  // docker's formatting of the peak sample's used memory
	peakCPU     float64 // peak CPU percent
	sampled     bool    // at least one successful sample was taken
	stop        chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

// NewMeter builds a Meter for the named container.
func NewMeter(dockerBin, name string, footer *TermFooter) *Meter {
	ctx, cancel := context.WithCancel(context.Background())
	return &Meter{
		dockerBin: dockerBin,
		name:      name,
		start:     time.Now(),
		footer:    footer,
		stop:      make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start launches the sampler (and the renderer, if a footer is set).
func (m *Meter) Start() {
	m.wg.Add(1)
	go m.sampleLoop()
	if m.footer != nil {
		m.wg.Add(1)
		go m.renderLoop()
	}
}

// Stop halts the goroutines and erases the footer. It cancels any in-flight
// `docker stats` so exit is immediate (no waiting on a blocked sample).
func (m *Meter) Stop() {
	m.cancel()
	close(m.stop)
	m.wg.Wait()
	if m.footer != nil {
		m.footer.Finish()
	}
}

// sampleLoop polls `docker stats` (which itself blocks ~1-2s per sample) and
// tracks the running peak.
func (m *Meter) sampleLoop() {
	defer m.wg.Done()
	for {
		r := m.sample()
		if r.ok {
			m.mu.Lock()
			m.cur = r
			m.sampled = true
			if r.usedBytes > m.peakBytes {
				m.peakBytes = r.usedBytes
				m.peakMemText = strings.SplitN(r.memText, "/", 2)[0]
			}
			if r.cpuVal > m.peakCPU {
				m.peakCPU = r.cpuVal
			}
			m.mu.Unlock()
		}
		select {
		case <-m.stop:
			return
		case <-time.After(time.Second):
		}
	}
}

// Summary returns a one-line resource summary, or "" if no sample was captured
// (e.g. a container too short-lived for docker stats to observe).
func (m *Meter) Summary() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.sampled {
		return ""
	}
	mem := strings.TrimSpace(m.peakMemText)
	if mem == "" {
		mem = humanBytes(m.peakBytes)
	}
	return fmt.Sprintf("sandbox-cli: peak mem %s · cpu peak %.0f%% · %s",
		mem, m.peakCPU, formatDuration(time.Since(m.start)))
}

// renderLoop keeps the elapsed clock ticking smoothly between samples.
func (m *Meter) renderLoop() {
	defer m.wg.Done()
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	m.footer.SetStatus(m.format())
	for {
		select {
		case <-m.stop:
			return
		case <-t.C:
			m.footer.SetStatus(m.format())
		}
	}
}

// sample runs a single `docker stats --no-stream` for the container.
func (m *Meter) sample() reading {
	out, err := exec.CommandContext(m.ctx, m.dockerBin, "stats", "--no-stream",
		"--format", "{{.MemUsage}}|{{.CPUPerc}}", m.name).Output()
	if err != nil {
		return reading{} // container not running yet, or gone
	}
	fields := strings.SplitN(strings.TrimSpace(string(out)), "|", 2)
	if len(fields) != 2 {
		return reading{}
	}
	memText, frac := parseMemUsage(fields[0])
	usedStr := strings.TrimSpace(strings.SplitN(fields[0], "/", 2)[0])
	usedBytes, _ := parseBytes(usedStr)
	cpu := strings.TrimSpace(fields[1])
	cpuVal, _ := strconv.ParseFloat(strings.TrimSuffix(cpu, "%"), 64)
	return reading{
		usedBytes: usedBytes,
		memText:   memText,
		memFrac:   frac,
		cpu:       cpu,
		cpuVal:    cpuVal,
		ok:        true,
	}
}

// format builds the status line from the latest reading and elapsed time.
func (m *Meter) format() string {
	m.mu.Lock()
	r := m.cur
	m.mu.Unlock()

	elapsed := formatDuration(time.Since(m.start))
	if !r.ok {
		return fmt.Sprintf("\033[2m sandbox-cli │ starting… · %s \033[0m", elapsed)
	}
	return fmt.Sprintf("\033[2m sandbox-cli │ mem %s %s cpu %s · %s \033[0m",
		r.memText, bar(r.memFrac, 8), r.cpu, elapsed)
}

// bar renders a fixed-width unicode meter for a 0..1 fraction.
func bar(frac float64, width int) string {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := int(frac*float64(width) + 0.5)
	return "▕" + strings.Repeat("▓", filled) + strings.Repeat("░", width-filled) + "▏"
}

// parseMemUsage takes docker's "512MiB / 7.633GiB" and returns a compact display
// string plus the used/limit fraction (0 if it can't be computed).
func parseMemUsage(s string) (string, float64) {
	parts := strings.SplitN(s, "/", 2)
	usedStr := strings.TrimSpace(parts[0])
	display := usedStr
	var frac float64
	if len(parts) == 2 {
		limitStr := strings.TrimSpace(parts[1])
		display = usedStr + "/" + limitStr
		used, uok := parseBytes(usedStr)
		limit, lok := parseBytes(limitStr)
		if uok && lok && limit > 0 {
			frac = used / limit
		}
	}
	return display, frac
}

// parseBytes parses a docker size like "512MiB", "7.633GiB", "0B", "1.2kB".
func parseBytes(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	if i == 0 {
		return 0, false
	}
	num, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return 0, false
	}
	unit := strings.TrimSpace(s[i:])
	mult, ok := unitBytes[unit]
	if !ok {
		return 0, false
	}
	return num * mult, true
}

var unitBytes = map[string]float64{
	"B": 1, "": 1,
	"KiB": 1 << 10, "MiB": 1 << 20, "GiB": 1 << 30, "TiB": 1 << 40,
	"kB": 1e3, "KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12,
}

// humanBytes renders a byte count as a compact binary size (fallback when
// docker's own formatting isn't available).
func humanBytes(b float64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%.0fB", b)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	v := b / unit
	i := 0
	for v >= unit && i < len(units)-1 {
		v /= unit
		i++
	}
	return fmt.Sprintf("%.1f%s", v, units[i])
}

// formatDuration renders a duration as "1m04s" / "12s".
func formatDuration(d time.Duration) string {
	d = d.Truncate(time.Second)
	m := int(d / time.Minute)
	s := int((d % time.Minute) / time.Second)
	if m == 0 {
		return fmt.Sprintf("%ds", s)
	}
	return fmt.Sprintf("%dm%02ds", m, s)
}
