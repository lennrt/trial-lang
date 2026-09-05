package counsel

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestReadMessageDistinguishesTruncationFromCleanEOF(t *testing.T) {
	for _, wire := range []string{
		"Content-Length: 2",
		"Content-Length: 2\r\n",
		"Content-Length: 2\r\n\r",
		"Content-Length: 2\r\n\r\n",
		"Content-Length: 2\r\n\r\n{",
	} {
		t.Run(fmt.Sprintf("%q", wire), func(t *testing.T) {
			_, err := readMessage(bufio.NewReaderSize(strings.NewReader(wire), maxHeaderLineBytes))
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("truncated frame returned %v, want io.ErrUnexpectedEOF", err)
			}
		})
	}
	_, err := readMessage(bufio.NewReader(strings.NewReader("")))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("clean disconnect returned %v, want io.EOF", err)
	}
}

func TestReadMessageRejectsInvalidLengths(t *testing.T) {
	for _, header := range []string{
		"Content-Length: -1\r\nContent-Length: 2",
		"Content-Length: -2\r\nContent-Length: 2",
		"Content-Length: 2\r\nContent-Length: 2",
		"Content-Length: +2",
		"Content-Length: -0",
		"Content-Length: 2.0",
		"Content-Length: 18446744073709551616",
		"Content-Length: ",
	} {
		t.Run(header, func(t *testing.T) {
			wire := header + "\r\n\r\n{}"
			if _, err := readMessage(bufio.NewReader(strings.NewReader(wire))); err == nil {
				t.Fatal("invalid Content-Length was accepted")
			}
		})
	}
}

func TestReadMessageEnforcesHeaderLimitWithLargerReader(t *testing.T) {
	wire := "Content-Type: " + strings.Repeat("x", maxHeaderLineBytes) + "\r\nContent-Length: 2\r\n\r\n{}"
	_, err := readMessage(bufio.NewReaderSize(strings.NewReader(wire), len(wire)))
	if err == nil || !strings.Contains(err.Error(), "header line exceeds") {
		t.Fatalf("oversized header returned %v", err)
	}
}

func TestReadMessageHeaderCountBoundary(t *testing.T) {
	for _, count := range []int{maxHeaders, maxHeaders + 1} {
		wire := "Content-Length: 0\r\n" + strings.Repeat("Content-Type: application/vscode-jsonrpc\r\n", count-1) + "\r\n"
		_, err := readMessage(bufio.NewReaderSize(strings.NewReader(wire), maxHeaderLineBytes))
		if count == maxHeaders && err != nil {
			t.Errorf("exactly %d headers returned %v", count, err)
		}
		if count > maxHeaders && (err == nil || !strings.Contains(err.Error(), "more than")) {
			t.Errorf("%d headers returned %v", count, err)
		}
	}
}

func TestReadMessageAggregateHeaderLimit(t *testing.T) {
	line := "Content-Type: " + strings.Repeat("x", maxHeaderLineBytes-len("Content-Type: \r\n")) + "\r\n"
	wire := "Content-Length: 0\r\n" + strings.Repeat(line, maxHeaderBytes/maxHeaderLineBytes+1) + "\r\n"
	_, err := readMessage(bufio.NewReaderSize(strings.NewReader(wire), maxHeaderLineBytes))
	if err == nil || !strings.Contains(err.Error(), "headers exceed") {
		t.Fatalf("oversized combined headers returned %v", err)
	}
}

type framingShortWriter struct {
	calls  int
	failAt int
	err    error
}

func (w *framingShortWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == w.failAt {
		return len(p) - 1, w.err
	}
	return len(p), nil
}

func TestCounselWritePropagatesShortWrites(t *testing.T) {
	for _, failAt := range []int{1, 2} {
		for _, cause := range []error{nil, errors.New("writer failed")} {
			w := &framingShortWriter{failAt: failAt, err: cause}
			s := &Server{Out: w}
			err := s.write(rpcResponse{ID: json.RawMessage("1"), Result: "hello"})
			want := cause
			if want == nil {
				want = io.ErrShortWrite
			}
			if !errors.Is(err, want) {
				t.Errorf("short write at call %d returned %v, want %v", failAt, err, want)
			}
			if w.calls != failAt {
				t.Errorf("continued writing after failure: calls = %d, want %d", w.calls, failAt)
			}
		}
	}
}

func TestCounselFramingRoundTrip(t *testing.T) {
	var out bytes.Buffer
	s := &Server{Out: &out}
	for _, text := range []string{"", "court", "⚖️ 審判 😀"} {
		if err := s.write(rpcResponse{ID: json.RawMessage("1"), Result: text}); err != nil {
			t.Fatal(err)
		}
	}
	r := bufio.NewReader(&out)
	for _, want := range []string{"", "court", "⚖️ 審判 😀"} {
		body, err := readMessage(r)
		if err != nil {
			t.Fatal(err)
		}
		var response struct {
			JSONRPC string `json:"jsonrpc"`
			Result  string `json:"result"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatal(err)
		}
		if response.JSONRPC != "2.0" || response.Result != want {
			t.Fatalf("round trip returned %+v, want %q", response, want)
		}
	}
	if _, err := readMessage(r); !errors.Is(err, io.EOF) {
		t.Fatalf("after complete frames got %v, want io.EOF", err)
	}
}

func FuzzCounselReadMessage(f *testing.F) {
	for _, wire := range []string{"", "Content-Length: 2\r\n\r\n{}", "Content-Length: 2\r\n\r\n", "Content-Length: -1\r\nContent-Length: 0\r\n\r\n"} {
		f.Add(wire)
	}
	f.Fuzz(func(t *testing.T, wire string) {
		if len(wire) > maxHeaderBytes {
			t.Skip()
		}
		_, err := readMessage(bufio.NewReaderSize(strings.NewReader(wire), maxHeaderLineBytes))
		if wire != "" && errors.Is(err, io.EOF) {
			t.Fatal("a nonempty truncated frame was treated as a clean disconnect")
		}
	})
}
