package compose

import (
	"bufio"
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// buildLogCapture captures Docker CLI stdout/stderr into a single stream,
// then in a goroutine either buffers (when=error) or streams (when=always) to tflog.
type buildLogCapture struct {
	w         io.Writer // serialized writer for both Out and Err
	pipeR     *io.PipeReader
	pipeW     *io.PipeWriter
	timestamp bool
	lines     int
	ringbuf   []string
	bufstart  int
	bufnext   int
	log       string // trace, debug, info, warn, error
	done      chan struct{}
}

// syncWriter serializes writes so both WithOutputStream and WithErrorStream can share one pipe.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// logBuildLine writes a build log line at the given level (trace, debug, info, warn, error).
func logBuildLine(ctx context.Context, level, line string) {
	msg := "[build] " + line
	switch strings.ToLower(level) {
	case "trace":
		tflog.Trace(ctx, msg)
	case "debug":
		tflog.Debug(ctx, msg)
	case "info":
		tflog.Info(ctx, msg)
	case "warn":
		tflog.Warn(ctx, msg)
	case "error":
		tflog.Error(ctx, msg)
	default:
		tflog.Warn(ctx, msg)
	}
}

func newBuildLogCapture(_ context.Context, timestamp bool, lines int, log string) *buildLogCapture {
	if lines <= 0 {
		lines = 1
	}
	pipeR, pipeW := io.Pipe()
	cap := &buildLogCapture{
		w:         &syncWriter{w: pipeW},
		pipeR:     pipeR,
		pipeW:     pipeW,
		timestamp: timestamp,
		lines:     lines,
		ringbuf:   make([]string, lines),
		bufstart:  0,
		bufnext:   0,
		log:       log,
		done:      make(chan struct{}),
	}

	return cap
}

// Writer returns an io.Writer to pass to command.WithOutputStream and command.WithErrorStream.
// Both can use the same writer; writes are serialized.
func (c *buildLogCapture) Writer() io.Writer {
	return c.w
}

// Start begins the goroutine that reads from the pipe and buffers or streams.
func (c *buildLogCapture) Start(ctx context.Context) {
	go c.run(ctx)
}

// run reads lines from the pipe and either buffers or streams.
func (c *buildLogCapture) run(ctx context.Context) {
	defer close(c.done)
	scanner := bufio.NewScanner(c.pipeR)
	// Allow long lines (e.g. progress lines)
	scanner.Buffer(nil, 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if c.timestamp {
			line = time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00") + " " + line
		}
		if c.log != "" {
			logBuildLine(ctx, c.log, line)
		}
		func() {
			c.ringbuf[c.bufnext] = line
			c.bufnext = (c.bufnext + 1) % len(c.ringbuf)
			if c.bufnext == c.bufstart {
				c.bufstart = (c.bufstart + 1) % len(c.ringbuf)
			}
		}()
	}
	err := scanner.Err()
	if err != nil && err != io.ErrClosedPipe {
		tflog.Debug(
			ctx,
			"build log capture read error",
			map[string]interface{}{"error": err.Error()},
		)
	}
}

// Close closes the pipe writer so the reader goroutine exits.
func (c *buildLogCapture) Close() error {
	return c.pipeW.Close()
}

// Wait blocks until the capture goroutine has finished (e.g. after Close).
func (c *buildLogCapture) Wait() {
	<-c.done
}

// GetLastLines returns the last buffered lines.
// Call after Wait().
func (c *buildLogCapture) GetLastLines() []string {
	start := c.bufstart
	end := c.bufnext
	if start <= end {
		return c.ringbuf[start:end]
	}
	return append(c.ringbuf[start:], c.ringbuf[:end]...)
}
