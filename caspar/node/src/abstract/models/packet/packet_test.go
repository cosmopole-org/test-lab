package packet

import (
	"encoding/json"
	"testing"
)

func TestBuildErrorJson(t *testing.T) {
	errObj := BuildErrorJson("boom")
	if errObj.Message != "boom" {
		t.Fatalf("message mismatch got=%q", errObj.Message)
	}
}

func TestPacketJsonShapes(t *testing.T) {
	p := Packet{Origin: "fed-a", Data: "payload"}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal packet: %v", err)
	}
	if string(raw) != `{"origin":"fed-a","data":"payload"}` {
		t.Fatalf("unexpected packet json: %s", raw)
	}

	cmd := Command{Value: "ping", Data: "x"}
	if cmd.Value != "ping" || cmd.Data != "x" {
		t.Fatalf("command fields mismatch: %#v", cmd)
	}
}
