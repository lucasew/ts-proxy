package tsproxy_test

import (
    "bytes"
    "errors"
    "log/slog"
    "testing"

    "github.com/lucasew/ts-proxy/pkg/tsproxy"
)

func TestReportError(t *testing.T) {
    var buf bytes.Buffer
    h := slog.NewTextHandler(&buf, nil)
    slog.SetDefault(slog.New(h))

    err := errors.New("test error")
    tsproxy.ReportError("test message", err, "key", "value")

    out := buf.String()
    if !bytes.Contains([]byte(out), []byte("test message")) {
        t.Errorf("expected 'test message' in log, got %s", out)
    }
    if !bytes.Contains([]byte(out), []byte("test error")) {
        t.Errorf("expected 'test error' in log, got %s", out)
    }
    if !bytes.Contains([]byte(out), []byte("key=value")) {
        t.Errorf("expected 'key=value' in log, got %s", out)
    }
}
