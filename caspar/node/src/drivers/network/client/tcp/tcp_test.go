package tcp

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

type mockConn struct {
	writes bytes.Buffer
	remote string
}

func (m *mockConn) Read(_ []byte) (int, error)         { return 0, nil }
func (m *mockConn) Write(b []byte) (int, error)        { return m.writes.Write(b) }
func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return testAddr("127.0.0.1:1") }
func (m *mockConn) RemoteAddr() net.Addr               { return testAddr(m.remote) }
func (m *mockConn) SetDeadline(_ time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(_ time.Time) error { return nil }

func TestWriteUpdateAndResponseBufferFraming(t *testing.T) {
	s := &Socket{Ack: false}
	s.writeUpdate("stores/update", []byte(`{"x":1}`), true)
	s.writeResponse("req-1", 200, []byte(`{"ok":true}`), true)
	if len(s.Buffer) != 2 {
		t.Fatalf("expected 2 queued packets, got %d", len(s.Buffer))
	}
	if s.Buffer[0][0] != 0x01 {
		t.Fatalf("expected update packet type 0x01 got %x", s.Buffer[0][0])
	}
	if s.Buffer[1][0] != 0x02 {
		t.Fatalf("expected response packet type 0x02 got %x", s.Buffer[1][0])
	}
}

func TestProcessPacketAckDequeuesAndPushesNext(t *testing.T) {
	c := &mockConn{remote: "1.2.3.4:99"}
	s := &Socket{Conn: c, Ack: false, Buffer: [][]byte{[]byte("first"), []byte("next")}}
	s.processPacket([]byte{0x01})
	if len(s.Buffer) != 1 || string(s.Buffer[0]) != "next" {
		t.Fatalf("unexpected buffer after ack: %#v", s.Buffer)
	}
	w := c.writes.Bytes()
	if len(w) < 4 {
		t.Fatalf("expected framed write, got %d bytes", len(w))
	}
	n := binary.BigEndian.Uint32(w[:4])
	if int(n) != len("next") {
		t.Fatalf("frame length mismatch got=%d", n)
	}
	if string(w[4:]) != "next" {
		t.Fatalf("frame payload mismatch got=%q", string(w[4:]))
	}
}

func TestProcessPacketRejectsHugeSignatureLength(t *testing.T) {
	s := &Socket{}
	p := make([]byte, 4)
	binary.BigEndian.PutUint32(p, 20000001)
	// should return early without panic
	s.processPacket(p)
}
