//go:build linux

package dbus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"syscall"
	"testing"
	"time"
)

func TestUnixTransportFDOwnershipCallback(t *testing.T) {
	for _, scenario := range []string{"partial-header", "invalid-index", "overstated-count", "unreferenced", "extra-rights", "success", "multiple-reads"} {
		t.Run(scenario, func(t *testing.T) {
			sender, receiver := fdCleanupSocketPair(t)
			readers, writers := fdCleanupPipes(t, 2)
			tr := newUnixTransportFromConn(receiver).(*unixTransport)
			var received []int
			tr.setIncomingUnixFDs(func(fds []int) { received = append(received, fds...) })
			t.Cleanup(func() {
				for _, fd := range received {
					_ = syscall.Close(fd)
				}
			})
			body := any(UnixFDIndex(0))
			count := uint32(2)
			wantError := false
			switch scenario {
			case "partial-header":
				wantError = true
			case "invalid-index":
				body, wantError = UnixFDIndex(2), true
			case "overstated-count":
				count, wantError = 3, true
			case "unreferenced":
				body = uint32(0)
			case "extra-rights":
				count = 1
			}
			msg := &Message{
				Type: TypeMethodReply,
				Headers: map[HeaderField]Variant{
					FieldReplySerial: MakeVariant(uint32(1)),
					FieldSignature:   MakeVariant(SignatureOf(body)),
					FieldUnixFDs:     MakeVariant(count),
				},
				Body: []any{body}, serial: 1,
			}
			var wire bytes.Buffer
			if err := msg.EncodeTo(&wire, binary.LittleEndian); err != nil {
				t.Fatal(err)
			}
			payload := wire.Bytes()
			if scenario == "partial-header" {
				payload = payload[:1]
			}
			rights := []int{int(writers[0].Fd()), int(writers[1].Fd())}
			if err := sender.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if scenario == "multiple-reads" {
				fdCleanupSend(t, sender, payload[:16], rights[:1])
				fdCleanupSend(t, sender, payload[16:], rights[1:])
			} else {
				fdCleanupSend(t, sender, payload, rights)
			}
			for _, w := range writers {
				if err := w.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if err := sender.Close(); err != nil {
				t.Fatal(err)
			}
			if err := receiver.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			_, err := tr.ReadMessage()
			if (err != nil) != wantError {
				t.Fatalf("ReadMessage error = %v, want error %v", err, wantError)
			}
			if _, err := tr.ReadMessage(); !errors.Is(err, io.EOF) {
				t.Fatalf("next read: %v", err)
			}
			if err := tr.Close(); err != nil {
				t.Fatal(err)
			}
			if len(received) != 2 || received[0] == received[1] {
				t.Fatalf("callback descriptors = %v, want two distinct descriptors", received)
			}
			for i, fd := range received {
				if _, err := syscall.Write(fd, []byte{'x'}); err != nil {
					t.Fatalf("transport closed callback-owned FD: %v", err)
				}
				if err := readers[i].SetReadDeadline(time.Now().Add(time.Second)); err != nil {
					t.Fatal(err)
				}
				var data [1]byte
				if _, err := io.ReadFull(readers[i], data[:]); err != nil || data[0] != 'x' {
					t.Fatalf("callback FD targets wrong pipe: %q, %v", data, err)
				}
			}
		})
	}
}

func TestUnixFDCallbackRetainsMalformedPrefix(t *testing.T) {
	var batches [][]int
	rdr := oobReader{incomingUnixFDs: func(fds []int) { batches = append(batches, fds) }}
	// These buffers exercise parsing only; the callback does not close the
	// synthetic descriptors or transfer them to a real connection.
	first := syscall.UnixRights(10)
	truncated := syscall.UnixRights(20, 30)[:syscall.CmsgLen(4)]
	if err := rdr.readFDs(append(first, truncated...)); err == nil {
		t.Fatal("expected a truncated control message error")
	}
	if len(batches) != 1 || len(batches[0]) != 2 || batches[0][0] != 10 || batches[0][1] != 20 {
		t.Fatalf("callback batches = %v", batches)
	}
	rdr.fds = rdr.fds[:0]
	if err := rdr.readFDs(syscall.UnixRights(40)); err != nil {
		t.Fatal(err)
	}
	if batches[0][0] != 10 || batches[0][1] != 20 {
		t.Fatalf("later read overwrote retained callback slice: %v", batches[0])
	}
}
