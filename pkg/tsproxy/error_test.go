package tsproxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
)

// Tabled test errors (prefer Err* over inline errors.New).
var (
	ErrWrap  = errors.New("wrap")
	ErrBoom  = errors.New("boom")
	ErrOther = errors.New("other")
)

// captureErrorLog installs a text handler on slog.Default and restores it.
func captureErrorLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestReportErrorSkipsNil(t *testing.T) {
	buf := captureErrorLog(t)

	ReportError(nil, "context", "should not log")
	if buf.Len() != 0 {
		t.Fatalf("expected no log for nil error, got %q", buf.String())
	}
}

func TestReportErrorSkipsExpected(t *testing.T) {
	buf := captureErrorLog(t)

	cases := []error{
		context.Canceled,
		net.ErrClosed,
		// Wrapped forms (fmt.Errorf / errors.Join) still match via errors.Is.
		errors.Join(io.EOF, net.ErrClosed),
		errors.Join(ErrWrap, context.Canceled),
	}

	for _, err := range cases {
		buf.Reset()
		ReportError(err, "context", "shutdown")
		if buf.Len() != 0 {
			t.Errorf("expected no log for %v, got %q", err, buf.String())
		}
	}
}

func TestReportErrorLogsUnexpected(t *testing.T) {
	buf := captureErrorLog(t)

	// DeadlineExceeded must still be reported (e.g. dial timeout).
	ReportError(context.DeadlineExceeded, "context", "tcp dial upstream")
	out := buf.String()
	if !strings.Contains(out, "unexpected error") {
		t.Fatalf("expected unexpected error log, got %q", out)
	}
	if !strings.Contains(out, "tcp dial upstream") {
		t.Fatalf("expected context attrs in log, got %q", out)
	}

	buf.Reset()
	ReportError(ErrBoom, "context", "server failed")
	if !strings.Contains(buf.String(), "boom") {
		t.Fatalf("expected boom in log, got %q", buf.String())
	}
}

func TestIsExpectedError(t *testing.T) {
	if !isExpectedError(context.Canceled) {
		t.Error("Canceled should be expected")
	}
	if !isExpectedError(net.ErrClosed) {
		t.Error("ErrClosed should be expected")
	}
	if isExpectedError(context.DeadlineExceeded) {
		t.Error("DeadlineExceeded should not be expected")
	}
	if isExpectedError(ErrOther) {
		t.Error("generic error should not be expected")
	}
}
