package tsproxy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"syscall"
	"testing"
)

func TestReportErrorSkipsNil(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ReportError(nil, "context", "should not log")
	if buf.Len() != 0 {
		t.Fatalf("expected no log for nil error, got %q", buf.String())
	}
}

func TestReportErrorSkipsExpected(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	cases := []error{
		context.Canceled,
		net.ErrClosed,
		io.EOF,
		io.ErrClosedPipe,
		syscall.ECONNRESET,
		syscall.EPIPE,
		syscall.ECONNABORTED,
		// Wrapped forms (fmt.Errorf / errors.Join / net.OpError) still match via errors.Is.
		errors.Join(io.EOF, net.ErrClosed),
		errors.Join(errors.New("wrap"), context.Canceled),
		&net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET},
		fmt.Errorf("copy: %w", syscall.EPIPE),
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
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(prev) })

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
	ReportError(errors.New("boom"), "context", "server failed")
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
	if !isExpectedError(io.EOF) {
		t.Error("EOF should be expected (clean peer close)")
	}
	if !isExpectedError(io.ErrClosedPipe) {
		t.Error("ErrClosedPipe should be expected")
	}
	if !isExpectedError(syscall.ECONNRESET) {
		t.Error("ECONNRESET should be expected")
	}
	if !isExpectedError(syscall.EPIPE) {
		t.Error("EPIPE should be expected")
	}
	if !isExpectedError(syscall.ECONNABORTED) {
		t.Error("ECONNABORTED should be expected")
	}
	if isExpectedError(context.DeadlineExceeded) {
		t.Error("DeadlineExceeded should not be expected")
	}
	if isExpectedError(errors.New("other")) {
		t.Error("generic error should not be expected")
	}
}
