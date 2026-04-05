package runtime

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/james-nesbitt/alloy/api"
	wazeroapi "github.com/tetratelabs/wazero/api"
)

type mockMemory struct {
	wazeroapi.Memory
	data []byte
}

func (m *mockMemory) ReadUint32Le(offset uint32) (uint32, bool) {
	return binary.LittleEndian.Uint32(m.data[offset:]), true
}

func (m *mockMemory) Read(offset uint32, byteCount uint32) ([]byte, bool) {
	return m.data[offset : offset+byteCount], true
}

type mockModule struct {
	wazeroapi.Module
	name string
	mem  *mockMemory
}

func (m *mockModule) Name() string             { return m.name }
func (m *mockModule) Memory() wazeroapi.Memory { return m.mem }

func TestRuntime_HostExports_Propose(t *testing.T) {
	var routedMsg api.Message
	router := func(ctx context.Context, msg api.Message) {
		routedMsg = msg
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	rt := &Runtime{
		logger:   logger,
		routerFn: router,
	}

	// Setup mock memory with ProposeIntent layout:
	// id (8), name (8), desc (8), payload (8), context_id (option: 4 discriminant + 8 string)
	memData := make([]byte, 100)
	le := binary.LittleEndian

	// id: ptr=0, len=4 ("test")
	le.PutUint32(memData[0:], 50)
	le.PutUint32(memData[4:], 4)
	copy(memData[50:], "test")

	// name: ptr=8, len=4 ("goal")
	le.PutUint32(memData[8:], 55)
	le.PutUint32(memData[12:], 4)
	copy(memData[55:], "goal")

	// desc: ptr=16, len=4 ("desc")
	le.PutUint32(memData[16:], 60)
	le.PutUint32(memData[20:], 4)
	copy(memData[60:], "desc")

	// payload: ptr=24, len=2 ("{}")
	le.PutUint32(memData[24:], 65)
	le.PutUint32(memData[28:], 2)
	copy(memData[65:], "{}")

	// context_id: discriminant=0 (None)
	le.PutUint32(memData[32:], 0)

	mod := &mockModule{name: "test-plugin", mem: &mockMemory{data: memData}}

	rt.internalProposeIntent(context.Background(), mod, 0)

	if routedMsg.Method != "intent:dispatch" {
		t.Errorf("expected method intent:dispatch, got %s", routedMsg.Method)
	}

	var intent api.Intent
	json.Unmarshal(routedMsg.Payload, &intent)

	if intent.Name != "intent:propose" {
		t.Errorf("expected intent name intent:propose, got %s", intent.Name)
	}

	var pData map[string]interface{}
	json.Unmarshal(intent.Payload, &pData)

	if pData["intent"] != "goal" {
		t.Errorf("expected goal, got %v", pData["intent"])
	}
	if pData["description"] != "desc" {
		t.Errorf("expected desc, got %v", pData["description"])
	}
}
