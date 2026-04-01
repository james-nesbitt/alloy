package guest

import (
	alloy "github.com/james-nesbitt/alloy/build/gen/bindings/guest"
)

func createDefaultHost() HostInterface {
	return &wasmHost{}
}

type wasmHost struct{}

func (w *wasmHost) Init(id string, caps []AlloyCapability, background bool) {
	wCaps := make([]alloy.AlloyCapability, len(caps))
	for i, c := range caps {
		wCaps[i] = w.toWitCap(c)
	}
	alloy.AlloyInit(id, wCaps, background)
}

func (w *wasmHost) Started() {
	alloy.AlloyStarted()
}

func (w *wasmHost) GetNextMessage() Option[AlloyMessage] {
	opt := alloy.AlloyGetNextMessage()
	if opt.IsNone() {
		return None[AlloyMessage]()
	}
	return Some(w.fromWitMsg(opt.Unwrap()))
}

func (w *wasmHost) SendResponse(msg AlloyMessage) {
	alloy.AlloySendResponse(w.toWitMsg(msg))
}

func (w *wasmHost) Log(level string, message string) {
	alloy.AlloyLog(level, message)
}

func (w *wasmHost) KvSet(key string, value []byte) bool {
	return alloy.AlloyKvSet(key, value)
}

func (w *wasmHost) KvGet(key string) Option[[]byte] {
	opt := alloy.AlloyKvGet(key)
	if opt.IsNone() {
		return None[[]byte]()
	}
	return Some(opt.Unwrap())
}

func (w *wasmHost) KvDelete(key string) bool {
	return alloy.AlloyKvDelete(key)
}

func (w *wasmHost) KvList(prefix string) []string {
	return alloy.AlloyKvList(prefix)
}

func (w *wasmHost) RouteMessage(msg AlloyMessage) {
	alloy.AlloyRouteMessage(w.toWitMsg(msg))
}

func (w *wasmHost) DispatchIntent(intent AlloyIntent) {
	alloy.AlloyDispatchIntent(w.toWitIntent(intent))
}

func (w *wasmHost) Call(msg AlloyMessage) AlloyMessage {
	res := alloy.AlloyCall(w.toWitMsg(msg))
	return w.fromWitMsg(res)
}

func (w *wasmHost) RegisterCapability(cap AlloyCapability) {
	alloy.AlloyRegisterCapability(w.toWitCap(cap))
}

func (w *wasmHost) UnregisterCapability(method string) {
	alloy.AlloyUnregisterCapability(method)
}

func (w *wasmHost) FindProviders(method, actor string, contextID string) []string {
	var witContext alloy.Option[string]
	if contextID != "" {
		witContext = alloy.Some(contextID)
	} else {
		witContext = alloy.None[string]()
	}
	return alloy.AlloyFindProviders(method, actor, witContext)
}

func (w *wasmHost) GetAllCapabilities(actor string, contextID string) []AlloyCapability {
	var witContext alloy.Option[string]
	if contextID != "" {
		witContext = alloy.Some(contextID)
	} else {
		witContext = alloy.None[string]()
	}

	witCaps := alloy.AlloyGetAllCapabilities(actor, witContext)
	res := make([]AlloyCapability, len(witCaps))
	for i, c := range witCaps {
		res[i] = w.fromWitCap(c)
	}
	return res
}

func (w *wasmHost) ReadBuffer(id string) Option[AlloyBuffer] {
	opt := alloy.AlloyReadBuffer(id)
	if opt.IsNone() {
		return None[AlloyBuffer]()
	}
	witB := opt.Unwrap()
	return Some(AlloyBuffer{
		Id:           witB.Id,
		Name:         witB.Name,
		Content:      witB.Content,
		LastModified: witB.LastModified,
		MimeType:     witB.MimeType,
	})
}

func (w *wasmHost) WriteBuffer(id string, content []byte) bool {
	return alloy.AlloyWriteBuffer(id, content)
}

func (w *wasmHost) ListBuffers() []string {
	return alloy.AlloyListBuffers()
}

func (w *wasmHost) GetBufferView(id string) (ptr, size uint32, ok bool) {
	opt := alloy.AlloyGetBufferView(id)
	if opt.IsNone() {
		return 0, 0, false
	}
	res := opt.Unwrap()
	return res.F0, res.F1, true
}

func (w *wasmHost) RegisterWidget(widget AlloyWidget) {
	alloy.AlloyRegisterWidget(alloy.AlloyWidget{
		Id:                widget.Id,
		Title:             widget.Title,
		ContentType:       widget.ContentType,
		Content:           widget.Content,
		RefreshIntervalMs: widget.RefreshIntervalMs,
	})
}

func (w *wasmHost) UnregisterWidget(id string) {
	alloy.AlloyUnregisterWidget(id)
}

func (w *wasmHost) UpdateWidget(id string, content []byte) {
	alloy.AlloyUpdateWidget(id, content)
}

// Internal Converters

func (w *wasmHost) toWitMsg(msg AlloyMessage) alloy.AlloyMessage {
	return alloy.AlloyMessage{
		Id:        msg.Id,
		MsgType:   msg.MsgType,
		Method:    msg.Method,
		Sender:    msg.Sender,
		Actor:     msg.Actor,
		Target:    w.toWitOptionString(msg.Target),
		Payload:   msg.Payload,
		Timestamp: msg.Timestamp,
		Metadata:  w.toWitMetadata(msg.Metadata),
	}
}

func (w *wasmHost) fromWitMsg(msg alloy.AlloyMessage) AlloyMessage {
	return AlloyMessage{
		Id:        msg.Id,
		MsgType:   msg.MsgType,
		Method:    msg.Method,
		Sender:    msg.Sender,
		Actor:     msg.Actor,
		Target:    w.fromWitOptionString(msg.Target),
		Payload:   msg.Payload,
		Timestamp: msg.Timestamp,
		Metadata:  w.fromWitMetadata(msg.Metadata),
	}
}

func (w *wasmHost) toWitCap(c AlloyCapability) alloy.AlloyCapability {
	return alloy.AlloyCapability{
		Method:      c.Method,
		Description: c.Description,
		Shortcut:    w.toWitOptionString(c.Shortcut),
		Annotations: w.toWitOptionMetadata(c.Annotations),
		Intents:     w.toWitOptionStringList(c.Intents),
	}
}

func (w *wasmHost) fromWitCap(c alloy.AlloyCapability) AlloyCapability {
	return AlloyCapability{
		Method:      c.Method,
		Description: c.Description,
		Shortcut:    w.fromWitOptionString(c.Shortcut),
		Annotations: w.fromWitOptionMetadata(c.Annotations),
		Intents:     w.fromWitOptionStringList(c.Intents),
	}
}

func (w *wasmHost) toWitIntent(intent AlloyIntent) alloy.AlloyIntent {
	return alloy.AlloyIntent{
		Id:        intent.Id,
		Name:      intent.Name,
		Sender:    intent.Sender,
		Payload:   intent.Payload,
		ContextId: w.toWitOptionString(intent.ContextID),
	}
}

func (w *wasmHost) toWitOptionString(opt Option[string]) alloy.Option[string] {
	if opt.IsNone() {
		return alloy.None[string]()
	}
	return alloy.Some(opt.Unwrap())
}

func (w *wasmHost) fromWitOptionString(opt alloy.Option[string]) Option[string] {
	if opt.IsNone() {
		return None[string]()
	}
	return Some(opt.Unwrap())
}

func (w *wasmHost) toWitOptionMetadata(opt Option[[]AlloyTuple2StringStringT]) alloy.Option[[]alloy.AlloyTuple2StringStringT] {
	if opt.IsNone() {
		return alloy.None[[]alloy.AlloyTuple2StringStringT]()
	}
	return alloy.Some(w.toWitMetadata(opt.Unwrap()))
}

func (w *wasmHost) fromWitOptionMetadata(opt alloy.Option[[]alloy.AlloyTuple2StringStringT]) Option[[]AlloyTuple2StringStringT] {
	if opt.IsNone() {
		return None[[]AlloyTuple2StringStringT]()
	}
	return Some(w.fromWitMetadata(opt.Unwrap()))
}

func (w *wasmHost) toWitOptionStringList(opt Option[[]string]) alloy.Option[[]string] {
	if opt.IsNone() {
		return alloy.None[[]string]()
	}
	return alloy.Some(opt.Unwrap())
}

func (w *wasmHost) fromWitOptionStringList(opt alloy.Option[[]string]) Option[[]string] {
	if opt.IsNone() {
		return None[[]string]()
	}
	return Some(opt.Unwrap())
}

func (w *wasmHost) toWitMetadata(meta []AlloyTuple2StringStringT) []alloy.AlloyTuple2StringStringT {
	res := make([]alloy.AlloyTuple2StringStringT, len(meta))
	for i, m := range meta {
		res[i] = alloy.AlloyTuple2StringStringT{F0: m.F0, F1: m.F1}
	}
	return res
}

func (w *wasmHost) fromWitMetadata(meta []alloy.AlloyTuple2StringStringT) []AlloyTuple2StringStringT {
	res := make([]AlloyTuple2StringStringT, len(meta))
	for i, m := range meta {
		res[i] = AlloyTuple2StringStringT{F0: m.F0, F1: m.F1}
	}
	return res
}
