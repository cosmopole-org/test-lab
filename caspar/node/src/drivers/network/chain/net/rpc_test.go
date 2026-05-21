package net

import "testing"

func TestRPCRespondDeliversResponseAndError(t *testing.T) {
	ch := make(chan RPCResponse, 1)
	r := &RPC{Command: "ping", RespChan: ch}
	r.Respond("pong", nil)

	resp := <-ch
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.Response.(string) != "pong" {
		t.Fatalf("response mismatch: %#v", resp.Response)
	}
}
