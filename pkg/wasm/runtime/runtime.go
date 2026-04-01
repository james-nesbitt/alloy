package runtime

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/james-nesbitt/alloy/api"
	"github.com/james-nesbitt/alloy/pkg/storage"
	"github.com/tetratelabs/wazero"
	wazeroapi "github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

var i32le = binary.LittleEndian

// Runtime manages the WASM runtime environment.
type Runtime struct {
	baseRuntime      wazero.Runtime
	rtConfig         wazero.RuntimeConfig
	compilationCache wazero.CompilationCache
	logger           *slog.Logger
	kv               storage.StateStore
	dataDir          string
	routerFn         func(ctx context.Context, msg api.Message)
	callFn           func(ctx context.Context, msg api.Message) (api.Message, error)
	plugins          map[string]*Instance
	mu               sync.RWMutex
	hostModule       wazeroapi.Module

	// Integrated Buffer Management
	buffers api.MmapRegistry

	// Buffer Views (pluginID -> bufferID -> guestPtr)
	bufferViews map[string]map[string]uint32
}

// Instance represents a WASM plugin instance.
type Instance struct {
	id            string
	ctx           context.Context
	cancel        context.CancelFunc
	mod           wazeroapi.Module
	pluginRuntime wazero.Runtime
	logger        *slog.Logger
	msgChan       chan api.Message
	capabilities  []api.Capability
	status        Status
	metadata      api.PluginMetadata

	startedCh chan struct{}

	// pending responses: msgID -> channel
	pmu     sync.Mutex
	pending map[string]chan api.Message

	// Resource limits
	maxMemoryBytes uint32
	msgPerSecond   int
	bytesPerSecond int // New byte rate limit
	fuelLimit      int // Execution limit proxy (ms)

	// Throttling state
	msgCount     int
	byteCount    int
	lastMsgReset time.Time
	rmu          sync.Mutex

	// Circuit breaker
	crashCount  int
	lastCrash   time.Time
	circuitOpen bool
}

func (i *Instance) checkThrottle(msgSize int) error {
	if i.msgPerSecond <= 0 && i.bytesPerSecond <= 0 {
		return nil
	}
	i.rmu.Lock()
	defer i.rmu.Unlock()
	if i.circuitOpen {
		return fmt.Errorf("plugin circuit breaker is OPEN")
	}
	now := time.Now()
	if now.Sub(i.lastMsgReset) > time.Second {
		i.msgCount = 0
		i.byteCount = 0
		i.lastMsgReset = now
	}
	if i.msgPerSecond > 0 && i.msgCount >= i.msgPerSecond {
		return fmt.Errorf("message rate limit exceeded")
	}
	if i.bytesPerSecond > 0 && i.byteCount+msgSize >= i.bytesPerSecond {
		return fmt.Errorf("byte rate limit exceeded")
	}
	i.msgCount++
	i.byteCount += msgSize
	return nil
}

func (i *Instance) recordCrash() {
	i.rmu.Lock()
	defer i.rmu.Unlock()
	now := time.Now()
	if now.Sub(i.lastCrash) > 60*time.Second {
		i.crashCount = 0
	}
	i.crashCount++
	i.lastCrash = now
	if i.crashCount >= 3 {
		i.circuitOpen = true
		i.status = StatusCrashed
	}
}

func (i *Instance) Metadata() api.PluginMetadata {
	i.pmu.Lock()
	defer i.pmu.Unlock()
	return i.metadata
}

func (i *Instance) Close(ctx context.Context) error {
	i.cancel()
	if i.mod != nil {
		_ = i.mod.Close(ctx)
	}
	if i.pluginRuntime != nil {
		return i.pluginRuntime.Close(ctx)
	}
	return nil
}

type Status int

const (
	StatusRunning Status = iota
	StatusPaused
	StatusStopped
	StatusCrashed
)

func NewRuntime(ctx context.Context, logger *slog.Logger, kv storage.StateStore, dataDir string, bufferRegistry api.MmapRegistry, router func(ctx context.Context, msg api.Message), call func(ctx context.Context, msg api.Message) (api.Message, error)) (*Runtime, error) {
	cache := wazero.NewCompilationCache()
	rtConfig := wazero.NewRuntimeConfig().WithCompilationCache(cache)
	r := wazero.NewRuntimeWithConfig(ctx, rtConfig)
	logger.Info("creating new WIT-based runtime (v2.9-async)")
	rt := &Runtime{
		baseRuntime:      r,
		rtConfig:         rtConfig,
		compilationCache: cache,
		logger:           logger,
		kv:               kv,
		dataDir:          dataDir,
		buffers:          bufferRegistry,
		routerFn:         router,
		callFn:           call,
		plugins:          make(map[string]*Instance),
		bufferViews:      make(map[string]map[string]uint32),
	}
	hostMod, err := rt.instantiateHostModuleInRuntime(ctx, rt.baseRuntime)
	if err != nil {
		return nil, err
	}
	rt.hostModule = hostMod
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt.baseRuntime); err != nil {
		return nil, fmt.Errorf("failed to instantiate WASI: %w", err)
	}
	_, _ = rt.baseRuntime.NewHostModuleBuilder("asyncify").
		NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start").
		NewFunctionBuilder().WithFunc(func() {}).Export("stop").
		NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start_unwind").
		NewFunctionBuilder().WithFunc(func() {}).Export("stop_unwind").
		NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start_rewind").
		NewFunctionBuilder().WithFunc(func() {}).Export("stop_rewind").
		Instantiate(ctx)
	return rt, nil
}

func (r *Runtime) instantiateHostModuleInRuntime(ctx context.Context, rt wazero.Runtime) (wazeroapi.Module, error) {
	builder := rt.NewHostModuleBuilder("alloy")
	builder.NewFunctionBuilder().WithFunc(r.internalInit).Export("init")
	builder.NewFunctionBuilder().WithFunc(r.internalHandleMessage).Export("handle-message")
	builder.NewFunctionBuilder().WithFunc(r.internalLog).Export("log")
	builder.NewFunctionBuilder().WithFunc(r.internalKVSet).Export("kv-set")
	builder.NewFunctionBuilder().WithFunc(r.internalKVGet).Export("kv-get")
	builder.NewFunctionBuilder().WithFunc(r.internalKVDelete).Export("kv-delete")
	builder.NewFunctionBuilder().WithFunc(r.internalKVList).Export("kv-list")
	builder.NewFunctionBuilder().WithFunc(r.internalRouteMessage).Export("route-message")
	builder.NewFunctionBuilder().WithFunc(r.internalCall).Export("call")
	builder.NewFunctionBuilder().WithFunc(r.internalStarted).Export("started")
	builder.NewFunctionBuilder().WithFunc(r.internalGetNextMessage).Export("get-next-message")
	builder.NewFunctionBuilder().WithFunc(r.internalSendResponse).Export("send-response")
	builder.NewFunctionBuilder().WithFunc(r.internalRegisterCapability).Export("register-capability")
	builder.NewFunctionBuilder().WithFunc(r.internalUnregisterCapability).Export("unregister-capability")
	builder.NewFunctionBuilder().WithFunc(r.internalFindProviders).Export("find-providers")
	builder.NewFunctionBuilder().WithFunc(r.internalGetAllCapabilities).Export("get-all-capabilities")
	builder.NewFunctionBuilder().WithFunc(r.internalReadBuffer).Export("read-buffer")
	builder.NewFunctionBuilder().WithFunc(r.internalWriteBuffer).Export("write-buffer")
	builder.NewFunctionBuilder().WithFunc(r.internalListBuffers).Export("list-buffers")
	builder.NewFunctionBuilder().WithFunc(r.internalGetBufferView).Export("get-buffer-view")
	builder.NewFunctionBuilder().WithFunc(r.internalRegisterWidget).Export("register-widget")
	builder.NewFunctionBuilder().WithFunc(r.internalUnregisterWidget).Export("unregister-widget")
	builder.NewFunctionBuilder().WithFunc(r.internalUpdateWidget).Export("update-widget")
	builder.NewFunctionBuilder().WithFunc(r.internalDispatchIntent).Export("dispatch-intent")
	builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod wazeroapi.Module, resPtr uint32) {
		mod.Memory().WriteUint32Le(resPtr, 0)
		mod.Memory().WriteUint32Le(resPtr+4, 0)
	}).Export("save-state")
	builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, mod wazeroapi.Module, ptr, len uint32) {}).Export("load-state")
	return builder.Instantiate(ctx)
}

func (r *Runtime) internalInit(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, capsPtr, capsLen, background uint32) {
	id := r.readString(mod, idPtr, idLen)
	isBackground := background != 0
	if capsLen > 0 {
		data, _ := mod.Memory().Read(capsPtr, capsLen*52) // Capability size is now 52 (Phase 10)
		for i := uint32(0); i < capsLen; i++ {
			p := i * 52
			cap := api.Capability{
				Method:      r.readString(mod, i32le.Uint32(data[p:]), i32le.Uint32(data[p+4:])),
				Description: r.readString(mod, i32le.Uint32(data[p+8:]), i32le.Uint32(data[p+12:])),
			}
			if i32le.Uint32(data[p+16:]) != 0 {
				cap.Shortcut = r.readString(mod, i32le.Uint32(data[p+20:]), i32le.Uint32(data[p+24:]))
			}
			if i32le.Uint32(data[p+28:]) != 0 {
				cap.Annotations = make(map[string]string)
				aPtr := i32le.Uint32(data[p+32:])
				aLen := i32le.Uint32(data[p+36:])
				if aLen > 0 {
					ad, _ := mod.Memory().Read(aPtr, aLen*16)
					for j := uint32(0); j < aLen; j++ {
						o := j * 16
						k := r.readString(mod, i32le.Uint32(ad[o:]), i32le.Uint32(ad[o+4:]))
						v := r.readString(mod, i32le.Uint32(ad[o+8:]), i32le.Uint32(ad[o+12:]))
						cap.Annotations[k] = v
					}
				}
			}
			// Phase 10: Intents support in Capability struct
			if i32le.Uint32(data[p+40:]) != 0 {
				intentsPtr := i32le.Uint32(data[p+44:])
				intentsLen := i32le.Uint32(data[p+48:])
				if intentsLen > 0 {
					id, _ := mod.Memory().Read(intentsPtr, intentsLen*8) // Each string is 8 bytes (ptr+len)
					for j := uint32(0); j < intentsLen; j++ {
						o := j * 8
						sPtr := i32le.Uint32(id[o:])
						sLen := i32le.Uint32(id[o+4:])
						cap.Intents = append(cap.Intents, r.readString(mod, sPtr, sLen))
					}
				}
			}
			payload, _ := json.Marshal(cap)
			r.routerFn(ctx, api.Message{
				ID:     fmt.Sprintf("init-cap-%d-%d", time.Now().UnixNano(), i),
				Sender: id, Target: "command-manager", Method: "register-capability", Payload: payload,
			})
		}
	}

	// Update instance metadata with background status discovered during init (Phase 10)
	r.mu.RLock()
	instance, ok := r.plugins[id]
	r.mu.RUnlock()
	if ok {
		r.mu.Lock()
		instance.metadata.Background = isBackground
		r.mu.Unlock()
	}
}

func (r *Runtime) internalStarted(ctx context.Context, mod wazeroapi.Module) {
	r.logger.Info("wasm plugin ready", "plugin", mod.Name())
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()
	if ok && instance.startedCh != nil {
		select {
		case <-instance.startedCh:
		default:
			close(instance.startedCh)
		}
	}
}

func (r *Runtime) internalLog(ctx context.Context, mod wazeroapi.Module, levelPtr, levelLen, msgPtr, msgLen uint32) {
	level, _ := mod.Memory().Read(levelPtr, levelLen)
	msg, _ := mod.Memory().Read(msgPtr, msgLen)
	r.logger.Info("plugin_log", "id", mod.Name(), "level", string(level), "msg", string(msg))
}

func (r *Runtime) internalKVSet(ctx context.Context, mod wazeroapi.Module, keyPtr, keyLen, valuePtr, valueLen uint32) uint32 {
	key, _ := mod.Memory().Read(keyPtr, keyLen)
	val, _ := mod.Memory().Read(valuePtr, valueLen)
	if err := r.kv.Set(mod.Name(), string(key), val); err != nil {
		return 0
	}
	return 1
}

func (r *Runtime) internalKVGet(ctx context.Context, mod wazeroapi.Module, keyPtr, keyLen, resultPtr uint32) {
	key, _ := mod.Memory().Read(keyPtr, keyLen)
	val, err := r.kv.Get(mod.Name(), string(key))
	if err != nil || val == nil {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		return
	}
	alloc := mod.ExportedFunction("cabi_realloc")
	res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(val)))
	mod.Memory().Write(uint32(res[0]), val)
	mod.Memory().WriteUint32Le(resultPtr, 1)
	mod.Memory().WriteUint32Le(resultPtr+4, uint32(res[0]))
	mod.Memory().WriteUint32Le(resultPtr+8, uint32(len(val)))
}

func (r *Runtime) internalKVDelete(ctx context.Context, mod wazeroapi.Module, keyPtr, keyLen uint32) uint32 {
	key, _ := mod.Memory().Read(keyPtr, keyLen)
	if err := r.kv.Delete(mod.Name(), string(key)); err != nil {
		return 0
	}
	return 1
}

func (r *Runtime) internalKVList(ctx context.Context, mod wazeroapi.Module, prefixPtr, prefixLen, resultPtr uint32) {
	prefix, _ := mod.Memory().Read(prefixPtr, prefixLen)
	keys, err := r.kv.List(mod.Name(), string(prefix))
	if err != nil {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		return
	}
	alloc := mod.ExportedFunction("cabi_realloc")
	stringStructs := make([]byte, len(keys)*8)
	for i, key := range keys {
		sRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(key)))
		mod.Memory().Write(uint32(sRes[0]), []byte(key))
		i32le.PutUint32(stringStructs[i*8:], uint32(sRes[0]))
		i32le.PutUint32(stringStructs[i*8+4:], uint32(len(key)))
	}
	lRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(stringStructs)))
	mod.Memory().Write(uint32(lRes[0]), stringStructs)
	mod.Memory().WriteUint32Le(resultPtr, uint32(lRes[0]))
	mod.Memory().WriteUint32Le(resultPtr+4, uint32(len(keys)))
}

func (r *Runtime) readString(mod wazeroapi.Module, ptr, length uint32) string {
	if length == 0 {
		return ""
	}
	data, _ := mod.Memory().Read(ptr, length)
	return string(data)
}

func (r *Runtime) readMessage(mod wazeroapi.Module, ptr uint32) api.Message {
	readStr := func(p uint32) string {
		sPtr, _ := mod.Memory().ReadUint32Le(p)
		sLen, _ := mod.Memory().ReadUint32Le(p + 4)
		return r.readString(mod, sPtr, sLen)
	}
	ts, _ := mod.Memory().ReadUint64Le(ptr + 64)
	msg := api.Message{
		ID: readStr(ptr), Type: api.MessageType(readStr(ptr + 8)), Method: readStr(ptr + 16),
		Sender: readStr(ptr + 24), Actor: readStr(ptr + 32), Timestamp: int64(ts),
	}
	isSome, _ := mod.Memory().ReadUint32Le(ptr + 40)
	if isSome != 0 {
		msg.Target = readStr(ptr + 44)
	}
	pPtr, _ := mod.Memory().ReadUint32Le(ptr + 52)
	pLen, _ := mod.Memory().ReadUint32Le(ptr + 56)
	if pLen > 0 {
		data, _ := mod.Memory().Read(pPtr, pLen)
		msg.Payload = json.RawMessage(data)
	}
	mPtr, _ := mod.Memory().ReadUint32Le(ptr + 72)
	mLen, _ := mod.Memory().ReadUint32Le(ptr + 76)
	if mLen > 0 {
		msg.Metadata = make(map[string]any)
		md, _ := mod.Memory().Read(mPtr, mLen*16)
		for i := uint32(0); i < mLen; i++ {
			o := i * 16
			k := r.readString(mod, i32le.Uint32(md[o:]), i32le.Uint32(md[o+4:]))
			v := r.readString(mod, i32le.Uint32(md[o+8:]), i32le.Uint32(md[o+12:]))
			msg.Metadata[k] = v
		}
	}
	return msg
}

func (r *Runtime) writeMessage(ctx context.Context, mod wazeroapi.Module, ptr uint32, msg api.Message) {
	alloc := mod.ExportedFunction("cabi_realloc")
	if alloc == nil {
		return
	}
	writeStr := func(fieldPtr uint32, s string) {
		if s == "" {
			mod.Memory().WriteUint32Le(fieldPtr, 0)
			mod.Memory().WriteUint32Le(fieldPtr+4, 0)
			return
		}
		res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(s)))
		mod.Memory().Write(uint32(res[0]), []byte(s))
		mod.Memory().WriteUint32Le(fieldPtr, uint32(res[0]))
		mod.Memory().WriteUint32Le(fieldPtr+4, uint32(len(s)))
	}
	writeStr(ptr, msg.ID)
	writeStr(ptr+8, string(msg.Type))
	writeStr(ptr+16, msg.Method)
	writeStr(ptr+24, msg.Sender)
	writeStr(ptr+32, msg.Actor)
	if msg.Target != "" {
		mod.Memory().WriteUint32Le(ptr+40, 1)
		writeStr(ptr+44, msg.Target)
	} else {
		mod.Memory().WriteUint32Le(ptr+40, 0)
	}
	if len(msg.Payload) > 0 {
		res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(msg.Payload)))
		mod.Memory().Write(uint32(res[0]), msg.Payload)
		mod.Memory().WriteUint32Le(ptr+52, uint32(res[0]))
		mod.Memory().WriteUint32Le(ptr+56, uint32(len(msg.Payload)))
	} else {
		mod.Memory().WriteUint32Le(ptr+52, 0)
		mod.Memory().WriteUint32Le(ptr+56, 0)
	}
	mod.Memory().WriteUint64Le(ptr+64, uint64(msg.Timestamp))
	if len(msg.Metadata) > 0 {
		mod.Memory().WriteUint32Le(ptr+72, 1) // Using 72 for list ptr? No, list is ptr(4), len(4) at 72.
		metaData := make([]byte, len(msg.Metadata)*16)
		i := 0
		for k, v := range msg.Metadata {
			vStr := fmt.Sprintf("%v", v)
			kr, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(k)))
			mod.Memory().Write(uint32(kr[0]), []byte(k))
			i32le.PutUint32(metaData[i*16:], uint32(kr[0]))
			i32le.PutUint32(metaData[i*16+4:], uint32(len(k)))
			vr, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(vStr)))
			mod.Memory().Write(uint32(vr[0]), []byte(vStr))
			i32le.PutUint32(metaData[i*16+8:], uint32(vr[0]))
			i32le.PutUint32(metaData[i*16+12:], uint32(len(vStr)))
			i++
		}
		res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(metaData)))
		mod.Memory().Write(uint32(res[0]), metaData)
		mod.Memory().WriteUint32Le(ptr+72, uint32(res[0]))
		mod.Memory().WriteUint32Le(ptr+76, uint32(len(msg.Metadata)))
	} else {
		mod.Memory().WriteUint32Le(ptr+72, 0)
		mod.Memory().WriteUint32Le(ptr+76, 0)
	}
}

func (r *Runtime) internalHandleMessage(ctx context.Context, mod wazeroapi.Module, msgPtr, resultPtr uint32) {
	msg := r.readMessage(mod, msgPtr)
	r.writeMessage(ctx, mod, resultPtr, api.Message{ID: msg.ID + "-resp", Type: api.TypeResponse, Method: "unimplemented"})
}

func (r *Runtime) internalRouteMessage(ctx context.Context, mod wazeroapi.Module, msgPtr uint32) {
	msg := r.readMessage(mod, msgPtr)
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()
	if ok {
		if err := instance.checkThrottle(len(msg.Payload)); err != nil {
			return
		}
	}
	r.routerFn(ctx, msg)
}

func (r *Runtime) internalCall(ctx context.Context, mod wazeroapi.Module, msgPtr, resultPtr uint32) {
	msg := r.readMessage(mod, msgPtr)
	r.mu.RLock()
	instance, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()
	if ok {
		if err := instance.checkThrottle(len(msg.Payload)); err != nil {
			return
		}
	}
	callCtx := ctx
	if instance != nil && instance.fuelLimit > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(instance.fuelLimit)*time.Millisecond)
		defer cancel()
	}
	resp, err := r.callFn(callCtx, msg)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && instance != nil {
			instance.recordCrash()
		}
		resp = api.Message{ID: msg.ID + "-resp", Type: api.TypeResponse, Method: msg.Method, Sender: "kernel", Payload: []byte(fmt.Sprintf(`{"error":%q}`, err.Error()))}
	}
	r.writeMessage(ctx, mod, resultPtr, resp)
}

func (r *Runtime) internalGetNextMessage(ctx context.Context, mod wazeroapi.Module, resultPtr uint32) {
	r.mu.RLock()
	inst, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()
	if !ok {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		return
	}
	select {
	case msg := <-inst.msgChan:
		mod.Memory().WriteUint32Le(resultPtr, 1)
		r.writeMessage(ctx, mod, resultPtr+8, msg)
	case <-time.After(100 * time.Millisecond):
		mod.Memory().WriteUint32Le(resultPtr, 0)
	case <-ctx.Done():
		mod.Memory().WriteUint32Le(resultPtr, 0)
	}
}

func (r *Runtime) internalSendResponse(ctx context.Context, mod wazeroapi.Module, msgPtr uint32) {
	msg := r.readMessage(mod, msgPtr)
	msg.Type = api.TypeResponse
	r.mu.RLock()
	inst, ok := r.plugins[mod.Name()]
	r.mu.RUnlock()
	if ok {
		inst.pmu.Lock()
		defer inst.pmu.Unlock()
		if ch, ok := inst.pending[msg.ID]; ok {
			select {
			case ch <- msg:
			default:
			}
			return
		}
		reqID := strings.TrimSuffix(msg.ID, "-resp")
		if ch, ok := inst.pending[reqID]; ok {
			select {
			case ch <- msg:
			default:
			}
			return
		}
		go r.routerFn(ctx, msg)
	}
}

func (r *Runtime) LoadPlugin(ctx context.Context, id string, wasmBytes []byte, maxMemoryMB uint32, msgPerSec int, caps []api.Capability, background bool) (*Instance, error) {
	pluginDir := filepath.Join(r.dataDir, id)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return nil, err
	}
	instCtx, instCancel := context.WithCancel(context.Background())
	startedCh := make(chan struct{})
	instance := &Instance{
		id: id, ctx: instCtx, cancel: instCancel, logger: r.logger, msgChan: make(chan api.Message, 1024),
		capabilities: caps, status: StatusRunning, metadata: api.PluginMetadata{ID: id, Capabilities: caps, Background: background},
		startedCh: startedCh, pending: make(map[string]chan api.Message),
		maxMemoryBytes: maxMemoryMB * 1024 * 1024, msgPerSecond: msgPerSec,
		bytesPerSecond: 10 * 1024 * 1024, fuelLimit: 1000, lastMsgReset: time.Now(),
	}
	r.mu.Lock()
	r.plugins[id] = instance
	r.mu.Unlock()
	go func() {
		maxPages := instance.maxMemoryBytes / 65536
		if maxPages == 0 {
			maxPages = 2048
		}
		instRtConfig := wazero.NewRuntimeConfig().WithCompilationCache(r.compilationCache).WithMemoryLimitPages(maxPages)
		pluginRuntime := wazero.NewRuntimeWithConfig(instCtx, instRtConfig)
		instance.pluginRuntime = pluginRuntime
		compiled, err := pluginRuntime.CompileModule(instCtx, wasmBytes)
		if err != nil {
			instCancel()
			return
		}
		if _, err := r.instantiateHostModuleInRuntime(instCtx, pluginRuntime); err != nil {
			instCancel()
			return
		}
		if _, err := wasi_snapshot_preview1.Instantiate(instCtx, pluginRuntime); err != nil {
			instCancel()
			return
		}
		_, _ = pluginRuntime.NewHostModuleBuilder("asyncify").
			NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start").
			NewFunctionBuilder().WithFunc(func() {}).Export("stop").
			NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start_unwind").
			NewFunctionBuilder().WithFunc(func() {}).Export("stop_unwind").
			NewFunctionBuilder().WithFunc(func(ptr uint32) {}).Export("start_rewind").
			NewFunctionBuilder().WithFunc(func() {}).Export("stop_rewind").
			Instantiate(instCtx)

		var fs wazero.FSConfig
		fs = wazero.NewFSConfig().WithDirMount(pluginDir, "/")
		config := wazero.NewModuleConfig().WithName(id).WithStdout(newLoggerWriter(r.logger, id, "stdout")).WithStderr(newLoggerWriter(r.logger, id, "stderr")).WithFSConfig(fs)
		mod, err := pluginRuntime.InstantiateModule(instCtx, compiled, config)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				instance.recordCrash()
			}
			instCancel()
			return
		}
		r.mu.Lock()
		instance.mod = mod
		r.mu.Unlock()
		f := mod.ExportedFunction("_start")
		if f != nil {
			f.Call(instCtx)
		}
	}()
	select {
	case <-startedCh:
	case <-time.After(10 * time.Second):
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		metadataJSON, err := r.kv.Get(id, "plugin:metadata:"+id)
		if err == nil && metadataJSON != nil {
			var meta api.PluginMetadata
			if err := json.Unmarshal(metadataJSON, &meta); err == nil {
				r.mu.Lock()
				instance.metadata = meta
				r.mu.Unlock()
			}
		}
	}()
	return instance, nil
}

func (r *Runtime) Close(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, inst := range r.plugins {
		_ = inst.Close(ctx)
	}
	return r.baseRuntime.Close(ctx)
}

func (r *Runtime) UnloadPlugin(ctx context.Context, id string) error {
	r.mu.Lock()
	inst, ok := r.plugins[id]
	if ok {
		delete(r.plugins, id)
	}
	r.mu.Unlock()
	if ok {
		inst.cancel()
		if inst.mod != nil {
			return inst.mod.Close(ctx)
		}
	}
	return nil
}

func (r *Runtime) RouteMessage(ctx context.Context, pluginID string, msg api.Message) error {
	r.mu.RLock()
	inst, ok := r.plugins[pluginID]
	r.mu.RUnlock()
	if !ok {
		return errors.New("plugin not found")
	}
	if msg.Type == api.TypeRequest {
		inst.pmu.Lock()
		inst.pending[msg.ID] = make(chan api.Message, 1)
		inst.pmu.Unlock()
	}
	select {
	case inst.msgChan <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errors.New("channel full")
	}
}

func (r *Runtime) GetResponse(ctx context.Context, pluginID string, requestID string) (api.Message, error) {
	r.mu.RLock()
	inst, ok := r.plugins[pluginID]
	r.mu.RUnlock()
	if !ok {
		return api.Message{}, errors.New("plugin not found")
	}
	inst.pmu.Lock()
	ch, ok := inst.pending[requestID]
	inst.pmu.Unlock()
	if !ok {
		ch = make(chan api.Message, 1)
		inst.pmu.Lock()
		inst.pending[requestID] = ch
		inst.pmu.Unlock()
	}
	defer func() { inst.pmu.Lock(); delete(inst.pending, requestID); inst.pmu.Unlock() }()
	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		return api.Message{}, ctx.Err()
	case <-time.After(30 * time.Second):
		return api.Message{}, errors.New("timeout waiting for WASM response")
	}
}

func (r *Runtime) internalRegisterCapability(ctx context.Context, mod wazeroapi.Module, methodPtr, methodLen, descPtr, descLen, shortcutSet, shortcutPtr, shortcutLen, annoSet, annoPtr, annoLen, intentsSet, intentsPtr, intentsLen uint32) {
	cap := api.Capability{Method: r.readString(mod, methodPtr, methodLen), Description: r.readString(mod, descPtr, descLen)}
	if shortcutSet != 0 {
		cap.Shortcut = r.readString(mod, shortcutPtr, shortcutLen)
	}
	if annoSet != 0 && annoLen > 0 {
		cap.Annotations = make(map[string]string)
		md, _ := mod.Memory().Read(annoPtr, annoLen*16)
		for i := uint32(0); i < annoLen; i++ {
			o := i * 16
			k := r.readString(mod, i32le.Uint32(md[o:]), i32le.Uint32(md[o+4:]))
			v := r.readString(mod, i32le.Uint32(md[o+8:]), i32le.Uint32(md[o+12:]))
			cap.Annotations[k] = v
		}
	}
	// Phase 10: Intents support in register-capability
	if intentsSet != 0 && intentsLen > 0 {
		id, _ := mod.Memory().Read(intentsPtr, intentsLen*8)
		for i := uint32(0); i < intentsLen; i++ {
			o := i * 8
			sPtr := i32le.Uint32(id[o:])
			sLen := i32le.Uint32(id[o+4:])
			cap.Intents = append(cap.Intents, r.readString(mod, sPtr, sLen))
		}
	}
	payload, _ := json.Marshal(cap)
	r.routerFn(ctx, api.Message{ID: fmt.Sprintf("reg-cap-%d", time.Now().UnixNano()), Sender: mod.Name(), Target: "command-manager", Method: "register-capability", Payload: payload})
}

func (r *Runtime) internalUnregisterCapability(ctx context.Context, mod wazeroapi.Module, methodPtr, methodLen uint32) {
	method := r.readString(mod, methodPtr, methodLen)
	r.routerFn(ctx, api.Message{ID: fmt.Sprintf("unreg-cap-%d", time.Now().UnixNano()), Sender: mod.Name(), Target: "command-manager", Method: "unregister-capability", Payload: []byte(fmt.Sprintf("{\"method\":\"%s\"}", method))})
}

func (r *Runtime) internalFindProviders(ctx context.Context, mod wazeroapi.Module, methodPtr, methodLen, actorPtr, actorLen, contextSet, contextPtr, contextLen, resultPtr uint32) {
	method := r.readString(mod, methodPtr, methodLen)
	actor := r.readString(mod, actorPtr, actorLen)
	var contextID string
	if contextSet != 0 {
		contextID = r.readString(mod, contextPtr, contextLen)
	}

	metadata := make(map[string]any)
	if contextID != "" {
		metadata["context"] = contextID
	}

	resp, _ := r.callFn(ctx, api.Message{
		ID:       fmt.Sprintf("find-prov-%d", time.Now().UnixNano()),
		Type:     api.TypeRequest,
		Sender:   mod.Name(),
		Actor:    actor,
		Target:   "command-manager",
		Method:   "list",
		Payload:  []byte("{}"),
		Metadata: metadata,
	})

	var data struct {
		Targets []api.Registration `json:"targets"`
	}
	if err := json.Unmarshal(resp.Payload, &data); err != nil {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		mod.Memory().WriteUint32Le(resultPtr+4, 0)
		return
	}

	var providers []string
	for _, t := range data.Targets {
		matches := false
		if method == "*" {
			matches = true
		} else {
			for _, c := range t.Capabilities {
				if c.Method == method {
					matches = true
					break
				}
			}
		}
		if matches {
			providers = append(providers, t.ID)
		}
	}

	alloc := mod.ExportedFunction("cabi_realloc")
	stringStructs := make([]byte, len(providers)*8)
	for i, p := range providers {
		sRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(p)))
		mod.Memory().Write(uint32(sRes[0]), []byte(p))
		i32le.PutUint32(stringStructs[i*8:], uint32(sRes[0]))
		i32le.PutUint32(stringStructs[i*8+4:], uint32(len(p)))
	}
	lRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(stringStructs)))
	mod.Memory().Write(uint32(lRes[0]), stringStructs)
	mod.Memory().WriteUint32Le(resultPtr, uint32(lRes[0]))
	mod.Memory().WriteUint32Le(resultPtr+4, uint32(len(providers)))
}

func (r *Runtime) internalGetAllCapabilities(ctx context.Context, mod wazeroapi.Module, actorPtr, actorLen, contextSet, contextPtr, contextLen, resultPtr uint32) {
	actor := r.readString(mod, actorPtr, actorLen)
	var contextID string
	if contextSet != 0 {
		contextID = r.readString(mod, contextPtr, contextLen)
	}

	callPayload := []byte("{}")
	metadata := make(map[string]any)
	if contextID != "" {
		metadata["context"] = contextID
	}

	msg := api.Message{
		ID:       fmt.Sprintf("get-caps-%d", time.Now().UnixNano()),
		Type:     api.TypeRequest,
		Sender:   mod.Name(),
		Actor:    actor,
		Target:   "command-manager",
		Method:   "list",
		Payload:  callPayload,
		Metadata: metadata,
	}

	resp, _ := r.callFn(ctx, msg)
	var data struct {
		Targets []api.Registration `json:"targets"`
	}
	if err := json.Unmarshal(resp.Payload, &data); err != nil {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		mod.Memory().WriteUint32Le(resultPtr+4, 0)
		return
	}
	var allCaps []api.Capability
	for _, t := range data.Targets {
		allCaps = append(allCaps, t.Capabilities...)
	}
	alloc := mod.ExportedFunction("cabi_realloc")
	res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(allCaps)*40))
	basePtr := uint32(res[0])
	for i, c := range allCaps {
		r.writeCapability(ctx, mod, basePtr+uint32(i*40), c)
	}
	mod.Memory().WriteUint32Le(resultPtr, basePtr)
	mod.Memory().WriteUint32Le(resultPtr+4, uint32(len(allCaps)))
}

func (r *Runtime) writeCapability(ctx context.Context, mod wazeroapi.Module, ptr uint32, cap api.Capability) {
	alloc := mod.ExportedFunction("cabi_realloc")
	writeStr := func(fieldPtr uint32, s string) {
		if s == "" {
			mod.Memory().WriteUint32Le(fieldPtr, 0)
			mod.Memory().WriteUint32Le(fieldPtr+4, 0)
			return
		}
		res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(s)))
		mod.Memory().Write(uint32(res[0]), []byte(s))
		mod.Memory().WriteUint32Le(fieldPtr, uint32(res[0]))
		mod.Memory().WriteUint32Le(fieldPtr+4, uint32(len(s)))
	}
	writeStr(ptr, cap.Method)
	writeStr(ptr+8, cap.Description)
	if cap.Shortcut != "" {
		mod.Memory().WriteUint32Le(ptr+16, 1)
		writeStr(ptr+20, cap.Shortcut)
	} else {
		mod.Memory().WriteUint32Le(ptr+16, 0)
	}
	if len(cap.Annotations) > 0 {
		mod.Memory().WriteUint32Le(ptr+28, 1)
		meta := make([]byte, len(cap.Annotations)*16)
		i := 0
		for k, v := range cap.Annotations {
			kr, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(k)))
			mod.Memory().Write(uint32(kr[0]), []byte(k))
			i32le.PutUint32(meta[i*16:], uint32(kr[0]))
			i32le.PutUint32(meta[i*16+4:], uint32(len(k)))
			vr, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(v)))
			mod.Memory().Write(uint32(vr[0]), []byte(v))
			i32le.PutUint32(meta[i*16+8:], uint32(vr[0]))
			i32le.PutUint32(meta[i*16+12:], uint32(len(v)))
			i++
		}
		res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(meta)))
		mod.Memory().Write(uint32(res[0]), meta)
		mod.Memory().WriteUint32Le(ptr+32, uint32(res[0]))
		mod.Memory().WriteUint32Le(ptr+36, uint32(len(cap.Annotations)))
	} else {
		mod.Memory().WriteUint32Le(ptr+28, 0)
	}
}

func (r *Runtime) internalReadBuffer(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, resultPtr uint32) {
	id := r.readString(mod, idPtr, idLen)
	if r.buffers != nil {
		if b, ok := r.buffers.GetBuffer(id); ok {
			alloc := mod.ExportedFunction("cabi_realloc")
			writeStr := func(ptr uint32, s string) {
				res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(s)))
				mod.Memory().Write(uint32(res[0]), []byte(s))
				mod.Memory().WriteUint32Le(ptr, uint32(res[0]))
				mod.Memory().WriteUint32Le(ptr+4, uint32(len(s)))
			}
			mod.Memory().WriteUint32Le(resultPtr, 1)
			bufPtr := resultPtr + 8
			writeStr(bufPtr, b.GetID())
			writeStr(bufPtr+8, b.GetName())
			data := b.GetData()
			cRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(data)))
			mod.Memory().Write(uint32(cRes[0]), data)
			mod.Memory().WriteUint32Le(bufPtr+16, uint32(cRes[0]))
			mod.Memory().WriteUint32Le(bufPtr+20, uint32(len(data)))
			mod.Memory().WriteUint64Le(bufPtr+24, uint64(b.GetLastModified()))
			writeStr(bufPtr+32, "application/octet-stream")
			return
		}
	}
	resp, _ := r.callFn(ctx, api.Message{ID: fmt.Sprintf("read-buf-%d", time.Now().UnixNano()), Type: api.TypeRequest, Sender: mod.Name(), Target: "buffer", Method: "read", Payload: []byte(fmt.Sprintf("{\"id\":\"%s\"}", id))})
	if len(resp.Payload) == 0 {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		return
	}
	var buf struct {
		ID, Name     string
		Content      []byte
		LastModified uint64
		MimeType     string
	}
	if err := json.Unmarshal(resp.Payload, &buf); err != nil {
		mod.Memory().WriteUint32Le(resultPtr, 0)
		return
	}
	alloc := mod.ExportedFunction("cabi_realloc")
	writeStr := func(ptr uint32, s string) {
		res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(s)))
		mod.Memory().Write(uint32(res[0]), []byte(s))
		mod.Memory().WriteUint32Le(ptr, uint32(res[0]))
		mod.Memory().WriteUint32Le(ptr+4, uint32(len(s)))
	}
	mod.Memory().WriteUint32Le(resultPtr, 1)
	bufPtr := resultPtr + 8
	writeStr(bufPtr, buf.ID)
	writeStr(bufPtr+8, buf.Name)
	cRes, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(buf.Content)))
	mod.Memory().Write(uint32(cRes[0]), buf.Content)
	mod.Memory().WriteUint32Le(bufPtr+16, uint32(cRes[0]))
	mod.Memory().WriteUint32Le(bufPtr+20, uint32(len(buf.Content)))
	mod.Memory().WriteUint64Le(bufPtr+24, buf.LastModified)
	writeStr(bufPtr+32, buf.MimeType)
}

func (r *Runtime) internalWriteBuffer(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, contentPtr, contentLen uint32) uint32 {
	id := r.readString(mod, idPtr, idLen)
	content, _ := mod.Memory().Read(contentPtr, contentLen)
	if r.buffers != nil {
		if mod.Name() == "buffer" {
			b, err := r.buffers.CreateBuffer(id, id, int(contentLen))
			if err == nil {
				if int(contentLen) > b.GetSize() {
					_ = b.Resize(int(contentLen))
				}
				copy(b.GetData(), content)
				b.Lock()
				b.Unlock()
				return 1
			}
		} else if b, ok := r.buffers.GetBuffer(id); ok {
			copy(b.GetData(), content)
			b.Lock()
			b.Unlock()
			return 1
		}
	}
	payload, _ := json.Marshal(map[string]any{"id": id, "content": content})
	_, err := r.callFn(ctx, api.Message{ID: fmt.Sprintf("write-buf-%d", time.Now().UnixNano()), Type: api.TypeRequest, Sender: mod.Name(), Target: "buffer", Method: "write", Payload: payload})
	if err != nil {
		return 0
	}
	return 1
}

func (r *Runtime) internalListBuffers(ctx context.Context, mod wazeroapi.Module, resultPtr uint32) {
	resp, _ := r.callFn(ctx, api.Message{ID: fmt.Sprintf("list-bufs-%d", time.Now().UnixNano()), Type: api.TypeRequest, Sender: mod.Name(), Target: "buffer", Method: "list", Payload: []byte("{}")})
	var ids []string
	json.Unmarshal(resp.Payload, &ids)
	alloc := mod.ExportedFunction("cabi_realloc")
	res, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(ids)*8))
	basePtr := uint32(res[0])
	for i, id := range ids {
		sr, _ := alloc.Call(ctx, 0, 0, 1, uint64(len(id)))
		mod.Memory().Write(uint32(sr[0]), []byte(id))
		mod.Memory().WriteUint32Le(basePtr+uint32(i*8), uint32(sr[0]))
		mod.Memory().WriteUint32Le(basePtr+uint32(i*8+4), uint32(len(id)))
	}
	mod.Memory().WriteUint32Le(resultPtr, basePtr)
	mod.Memory().WriteUint32Le(resultPtr+4, uint32(len(ids)))
}

func (r *Runtime) internalRegisterWidget(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, titlePtr, titleLen, typePtr, typeLen, contentPtr, contentLen, interval uint32) {
	w := api.Widget{ID: r.readString(mod, idPtr, idLen), Title: r.readString(mod, titlePtr, titleLen), ContentType: r.readString(mod, typePtr, typeLen), RefreshIntervalMs: interval}
	if contentLen > 0 {
		w.Content, _ = mod.Memory().Read(contentPtr, contentLen)
	}
	payload, _ := json.Marshal(w)
	r.routerFn(ctx, api.Message{ID: fmt.Sprintf("reg-widget-%d", time.Now().UnixNano()), Type: api.TypeRequest, Sender: mod.Name(), Target: "widget-manager", Method: "register", Payload: payload})
}

func (r *Runtime) internalUnregisterWidget(ctx context.Context, mod wazeroapi.Module, idPtr, idLen uint32) {
	id := r.readString(mod, idPtr, idLen)
	payload, _ := json.Marshal(map[string]string{"id": id})
	r.routerFn(ctx, api.Message{ID: fmt.Sprintf("unreg-widget-%d", time.Now().UnixNano()), Type: api.TypeRequest, Sender: mod.Name(), Target: "widget-manager", Method: "unregister", Payload: payload})
}

func (r *Runtime) internalUpdateWidget(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, contentPtr, contentLen uint32) {
	id := r.readString(mod, idPtr, idLen)
	content, _ := mod.Memory().Read(contentPtr, contentLen)
	payload, _ := json.Marshal(api.WidgetUpdate{ID: id, Content: content})
	r.routerFn(ctx, api.Message{ID: fmt.Sprintf("upd-widget-%d", time.Now().UnixNano()), Type: api.TypeRequest, Sender: mod.Name(), Target: "widget-manager", Method: "update", Payload: payload})
}

func (r *Runtime) internalGetBufferView(ctx context.Context, mod wazeroapi.Module, idPtr, idLen, resultPtr uint32) {
	id := r.readString(mod, idPtr, idLen)
	pluginID := mod.Name()
	if r.buffers != nil {
		if b, ok := r.buffers.GetBuffer(id); ok {
			data := b.GetData()
			size := uint32(len(data))
			r.mu.Lock()
			if pluginViews, ok := r.bufferViews[pluginID]; ok {
				if ptr, ok := pluginViews[id]; ok {
					r.mu.Unlock()
					mod.Memory().WriteUint32Le(resultPtr, 1)
					mod.Memory().WriteUint32Le(resultPtr+4, ptr)
					mod.Memory().WriteUint32Le(resultPtr+8, size)
					return
				}
			} else {
				r.bufferViews[pluginID] = make(map[string]uint32)
			}
			r.mu.Unlock()
			b.OnUpdate(func(updatedID string, offset, length int) {
				if updatedID != id {
					return
				}
				r.mu.RLock()
				instance, ok := r.plugins[pluginID]
				pluginViews := r.bufferViews[pluginID]
				r.mu.RUnlock()
				if !ok || instance.mod == nil {
					return
				}
				guestPtr, ok := pluginViews[id]
				if !ok {
					return
				}
				hostData := b.GetData()
				if offset+length > len(hostData) {
					length = len(hostData) - offset
				}
				if offset < len(hostData) && length > 0 {
					instance.mod.Memory().Write(guestPtr+uint32(offset), hostData[offset:offset+length])
				}
			})
		}
	}
	mod.Memory().WriteUint32Le(resultPtr, 0)
}

type loggerWriter struct {
	logger *slog.Logger
	id     string
	stream string
	buf    []byte
}

func newLoggerWriter(logger *slog.Logger, id, stream string) *loggerWriter {
	return &loggerWriter{logger: logger, id: id, stream: stream}
}

func (l *loggerWriter) Write(p []byte) (n int, err error) {
	l.buf = append(l.buf, p...)
	for {
		idx := -1
		for i, b := range l.buf {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx == -1 {
			break
		}
		line := string(l.buf[:idx])
		l.buf = l.buf[idx+1:]
		if l.stream == "stdout" {
			l.logger.Info("wasm_out", "id", l.id, "msg", line)
		}
		if l.stream == "stderr" {
			l.logger.Error("wasm_err", "id", l.id, "msg", line)
		}
	}
	return len(p), nil
}
// internalDispatchIntent routes a goal-oriented intent (Phase 10)
func (r *Runtime) internalDispatchIntent(ctx context.Context, mod wazeroapi.Module, ptr uint32) {
	readStr := func(p uint32) string {
		sPtr, _ := mod.Memory().ReadUint32Le(p)
		sLen, _ := mod.Memory().ReadUint32Le(p + 4)
		return r.readString(mod, sPtr, sLen)
	}
	intent := api.Intent{
		ID:     readStr(ptr),
		Name:   readStr(ptr + 8),
		Sender: readStr(ptr + 16),
	}
	pPtr, _ := mod.Memory().ReadUint32Le(ptr + 24)
	pLen, _ := mod.Memory().ReadUint32Le(ptr + 28)
	if pLen > 0 {
		data, _ := mod.Memory().Read(pPtr, pLen)
		intent.Payload = json.RawMessage(data)
	}
	isSome, _ := mod.Memory().ReadUint32Le(ptr + 32) // alloy_option_string_t context_id starts at 32 (bool discriminant)
	if isSome != 0 {
		intent.ContextID = readStr(ptr + 36)
	}
	
	payload, _ := json.Marshal(intent)
	r.routerFn(ctx, api.Message{
		ID:      intent.ID,
		Type:    api.TypeEvent,
		Sender:  intent.Sender,
		Target:  "kernel",
		Method:  "intent:dispatch",
		Payload: payload,
	})
}
