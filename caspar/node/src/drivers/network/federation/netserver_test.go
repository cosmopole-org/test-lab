package net_federation

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"kasper/src/abstract/models/packet"
	inputs_stores "kasper/src/shell/api/inputs/stores"
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

func TestParseInput(t *testing.T) {
	in, err := ParseInput[inputs_stores.JoinInput](`{"storeId":"p@fed"}`)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	j := in.(inputs_stores.JoinInput)
	if j.StoreId != "p@fed" {
		t.Fatalf("store mismatch: %q", j.StoreId)
	}
	if _, err := ParseInput[inputs_stores.JoinInput]("not-json"); err == nil {
		t.Fatal("expected invalid format error")
	}
}

func TestWriteRequestBuildsPacket(t *testing.T) {
	s := &Socket{Ack: false}
	s.writeRequest("req-1", "user-1", "/stores/join", []byte(`{"a":1}`), "sig")
	if len(s.Buffer) != 1 {
		t.Fatalf("expected one buffered packet, got %d", len(s.Buffer))
	}
	p := s.Buffer[0]
	if p[0] != 0x03 {
		t.Fatalf("expected request type 0x03, got %x", p[0])
	}
}

func TestProcessPacketRequestRoutesBridge(t *testing.T) {
	var got packet.OriginPacket
	var gotIP string
	server := &Tcp{}
	server.bridge = func(_ *Socket, srcIp string, pack packet.OriginPacket) {
		gotIP = srcIp
		got = pack
	}
	c := &mockConn{remote: "10.1.2.3:9000"}
	s := &Socket{Conn: c, server: server}

	payload := []byte(`{"k":"v"}`)
	buf := make([]byte, 0)
	appendPart := func(b []byte) { buf = append(buf, b...) }
	appendLenStr := func(v string) {
		l := make([]byte, 4)
		binary.BigEndian.PutUint32(l, uint32(len(v)))
		appendPart(l)
		appendPart([]byte(v))
	}
	buf = append(buf, 0x03)
	appendLenStr("sig")
	appendLenStr("user-1")
	appendLenStr("/x")
	appendLenStr("req-42")
	appendPart(payload)

	s.processPacket(buf)
	if gotIP != "10.1.2.3" {
		t.Fatalf("src ip mismatch: %q", gotIP)
	}
	if got.Type != "request" || got.UserId != "user-1" || got.Key != "/x" || got.RequestId != "req-42" {
		t.Fatalf("unexpected parsed packet: %#v", got)
	}
	if string(got.Binary) != string(payload) {
		t.Fatalf("payload mismatch: %q", string(got.Binary))
	}
}

func TestProcessPacketAckPopsBufferAndSendsNext(t *testing.T) {
	c := &mockConn{remote: "10.1.2.3:9000"}
	s := &Socket{Conn: c, Buffer: [][]byte{[]byte("first"), []byte("second")}, Ack: false}
	s.processPacket([]byte("packet_received"))
	if s.Ack {
		t.Fatal("expected ack false while next buffered packet is in-flight")
	}
	if len(s.Buffer) != 1 || string(s.Buffer[0]) != "second" {
		t.Fatalf("unexpected buffer after ack: %#v", s.Buffer)
	}
	if c.writes.String() != "second" {
		t.Fatalf("expected second packet to be written, got %q", c.writes.String())
	}
}
