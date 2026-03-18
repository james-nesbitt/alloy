package tests

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/jnesbitt/alloy-go/api"
)

func sendMsg(t *testing.T, conn net.Conn, msg api.Message) {
	data, _ := json.Marshal(msg)
	if _, err := conn.Write(append(data, '\n')); err != nil {
		t.Fatalf("failed to send message %s: %v", msg.ID, err)
	}
}

func awaitResponse(t *testing.T, dec *json.Decoder, id string) api.Message {
	for i := 0; i < 1000; i++ {
		var m api.Message
		if err := dec.Decode(&m); err != nil {
			t.Fatalf("failed to decode while waiting for %s: %v", id, err)
		}
		if m.ID == id {
			return m
		}
	}
	t.Fatalf("timed out waiting for message %s", id)
	return api.Message{}
}
