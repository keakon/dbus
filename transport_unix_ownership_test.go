//go:build linux

package dbus

import (
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

func ownershipTestSockets(t *testing.T) (*net.UnixConn, *net.UnixConn) {
	t.Helper()
	pair, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	var sockets [2]*net.UnixConn
	for i, fd := range pair {
		f := os.NewFile(uintptr(fd), "socket")
		conn, err := net.FileConn(f)
		_ = f.Close()
		if err != nil {
			t.Fatal(err)
		}
		sockets[i] = conn.(*net.UnixConn)
		t.Cleanup(func() { _ = conn.Close() })
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	}
	return sockets[0], sockets[1]
}

func TestIncomingUnixFDsBeforeDecode(t *testing.T) {
	client, server := ownershipTestSockets(t)
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	received := make(chan []int, 1)
	conn, err := DialUnix(client, WithIncomingUnixFDs(func(fds []int) { received <- fds }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	result := make(chan error, 1)
	go func() { _, err := conn.ReadMessage(); result <- err }()
	if _, _, err := server.WriteMsgUnix([]byte{'l'}, syscall.UnixRights(int(write.Fd())), nil); err != nil {
		t.Fatal(err)
	}
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	var fds []int
	select {
	case fds = <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("FD was not delivered before decoding completed")
	}
	_ = conn.Close()
	if err := <-result; err == nil {
		t.Fatal("truncated message succeeded")
	}
	if len(fds) != 1 {
		t.Fatalf("descriptors = %v", fds)
	}
	for _, fd := range fds {
		if err := syscall.Close(fd); err != nil {
			t.Fatal(err)
		}
	}
	_ = read.SetReadDeadline(time.Now().Add(time.Second))
	var b [1]byte
	if _, err := read.Read(b[:]); err != io.EOF {
		t.Fatalf("descriptor remained open: %v", err)
	}
}

func TestReadDoneWaitsForFDHandler(t *testing.T) {
	client, server := ownershipTestSockets(t)
	entered, proceed := make(chan struct{}), make(chan struct{})
	conn, err := DialUnix(client, WithIncomingUnixFDs(func(fds []int) {
		close(entered)
		<-proceed
		for _, fd := range fds {
			_ = syscall.Close(fd)
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	go conn.inWorker()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	if _, _, err := server.WriteMsgUnix([]byte{'l'}, syscall.UnixRights(int(write.Fd())), nil); err != nil {
		t.Fatal(err)
	}
	<-entered
	_ = conn.Close()
	select {
	case <-conn.ReadDone():
		t.Error("receive worker completed while its handler was blocked")
	default:
	}
	close(proceed)
	select {
	case <-conn.ReadDone():
	case <-time.After(5 * time.Second):
		t.Fatal("receive worker did not finish")
	}
}
