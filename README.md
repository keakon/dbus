# Chord's godbus fork

This fork adds explicit receive-time ownership of Unix file descriptors and a
receive-worker completion signal. A caller can release descriptors even when a
message is truncated, malformed, unreferenced, or arrives after cancellation,
and wait for cleanup to finish after closing its private connection.

Use `WithIncomingUnixFDs` before authentication. The handler owns every delivered
descriptor; decoded message values are references, not additional owners. After
successful `Auth`, close the connection and wait on `ReadDone()` before collecting
final cleanup results. Do not wait on `ReadDone()` from the receiver's callbacks.

The module path is `github.com/keakon/dbus/v5`. The protocol implementation and
upstream license are retained; no Darwin-specific changes are included.

Run the focused Linux regressions with:

```sh
go test -run 'TestIncomingUnixFDsBeforeDecode|TestReadDoneWaitsForFDHandler' .
```

---

dbus
----


dbus is a simple library that implements native Go client bindings for the
D-Bus message bus system.

### Features

* Complete native implementation of the D-Bus message protocol
* Go-like API (channels for signals / asynchronous method calls, Goroutine-safe connections)
* Subpackages that help with the introspection / property interfaces

### Usage

The complete package documentation and some simple examples are available at
[pkg.go.dev](https://pkg.go.dev/github.com/godbus/dbus/v5). Also, the
[\_examples](https://github.com/godbus/dbus/tree/master/_examples) directory
gives a short overview over the basic usage.

#### Projects using godbus
- [bluetooth](https://github.com/tinygo-org/bluetooth): cross-platform Bluetooth API for Go and TinyGo.
- [fyne](https://github.com/fyne-io/fyne): a cross platform GUI in Go inspired by Material Design.
- [fynedesk](https://github.com/fyne-io/fynedesk): a full desktop environment for Linux/Unix using Fyne.
- [go-systemd](https://github.com/coreos/go-systemd): Go bindings to systemd.
- [notify](https://github.com/esiqveland/notify) provides desktop notifications over dbus into a library.

Please note that the API is considered unstable for now and may change without
further notice.

### License

The library is available under the Simplified BSD License; see LICENSE for the full
text.

Nearly all of the credit for this library goes to https://github.com/guelfey.
