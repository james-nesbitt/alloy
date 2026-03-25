//go:build wasip1 || wasm

package guest

import (
	wit "github.com/james-nesbitt/alloy/build/gen/bindings/guest"
)

// WasmHost implements HostInterface by calling the WIT-generated bindings.
type WasmHost struct{}

func createDefaultHost() HostInterface {
	return &WasmHost{}
}

func (w *WasmHost) Init(id string, caps []AlloyCapability) {
	witCaps := make([]wit.AlloyCapability, len(caps))
	for i, c := range caps {
		witCaps[i] = toWitCapability(c)
	}
	wit.AlloyInit(id, witCaps)
}

func (w *WasmHost) Started() {
	wit.AlloyStarted()
}

func (w *WasmHost) GetNextMessage() Option[AlloyMessage] {
	opt := wit.AlloyGetNextMessage()
	if opt.IsNone() {
		return None[AlloyMessage]()
	}
	return Some(fromWitMessage(opt.Unwrap()))
}

func (w *WasmHost) SendResponse(msg AlloyMessage) {
	wit.AlloySendResponse(toWitMessage(msg))
}

func (w *WasmHost) Log(level string, msg string) {
	wit.AlloyLog(level, msg)
}

func (w *WasmHost) RouteMessage(msg AlloyMessage) {
	wit.AlloyRouteMessage(toWitMessage(msg))
}

func (w *WasmHost) Call(msg AlloyMessage) AlloyMessage {
	return fromWitMessage(wit.AlloyCall(toWitMessage(msg)))
}

func (w *WasmHost) KvSet(key string, val []byte) bool {
	return wit.AlloyKvSet(key, val)
}

func (w *WasmHost) KvGet(key string) Option[[]byte] {
	opt := wit.AlloyKvGet(key)
	if opt.IsNone() {
		return None[[]byte]()
	}
	return Some(opt.Unwrap())
}

func (w *WasmHost) KvDelete(key string) bool {
	return wit.AlloyKvDelete(key)
}

func (w *WasmHost) KvList(prefix string) []string {
	return wit.AlloyKvList(prefix)
}

func (w *WasmHost) GetActiveWorkspace() Option[AlloyWorkspace] {
	opt := wit.AlloyGetActiveWorkspace()
	if opt.IsNone() {
		return None[AlloyWorkspace]()
	}
	return Some(fromWitWorkspace(opt.Unwrap()))
}

func (w *WasmHost) SetActiveWorkspace(id string) {
	wit.AlloySetActiveWorkspace(id)
}

func (w *WasmHost) ListWorkspaces() []AlloyWorkspace {
	list := wit.AlloyListWorkspaces()
	res := make([]AlloyWorkspace, len(list))
	for i, ws := range list {
		res[i] = fromWitWorkspace(ws)
	}
	return res
}

func (w *WasmHost) RegisterWorkspace(ws AlloyWorkspace) {
	wit.AlloyRegisterWorkspace(toWitWorkspace(ws))
}

func (w *WasmHost) UnregisterWorkspace(id string) {
	wit.AlloyUnregisterWorkspace(id)
}

func (w *WasmHost) RegisterCapability(cap AlloyCapability) {
	wit.AlloyRegisterCapability(toWitCapability(cap))
}

func (w *WasmHost) UnregisterCapability(method string) {
	wit.AlloyUnregisterCapability(method)
}

func (w *WasmHost) FindProviders(method string) []string {
	return wit.AlloyFindProviders(method)
}

func (w *WasmHost) GetAllCapabilities() []AlloyCapability {
	list := wit.AlloyGetAllCapabilities()
	res := make([]AlloyCapability, len(list))
	for i, c := range list {
		res[i] = fromWitCapability(c)
	}
	return res
}

func (w *WasmHost) ReadBuffer(id string) Option[AlloyBuffer] {
	opt := wit.AlloyReadBuffer(id)
	if opt.IsNone() {
		return None[AlloyBuffer]()
	}
	return Some(fromWitBuffer(opt.Unwrap()))
}

func (w *WasmHost) WriteBuffer(id string, content []byte) bool {
	return wit.AlloyWriteBuffer(id, content)
}

func (w *WasmHost) ListBuffers() []string {
	return wit.AlloyListBuffers()
}

func (w *WasmHost) GetBufferView(id string) (ptr, size uint32, ok bool) {
	opt := wit.AlloyGetBufferView(id)
	if opt.IsNone() {
		return 0, 0, false
	}
	val := opt.Unwrap()
	return val.F0, val.F1, true
}

func (w *WasmHost) RegisterWidget(wg AlloyWidget) {
	wit.AlloyRegisterWidget(toWitWidget(wg))
}

func (w *WasmHost) UnregisterWidget(id string) {
	wit.AlloyUnregisterWidget(id)
}

func (w *WasmHost) UpdateWidget(id string, content []byte) {
	wit.AlloyUpdateWidget(id, content)
}

// Conversion helpers

func toWitOptionString(opt Option[string]) wit.Option[string] {
	if opt.IsNone() {
		return wit.None[string]()
	}
	return wit.Some(opt.Unwrap())
}

func fromWitOptionString(opt wit.Option[string]) Option[string] {
	if opt.IsNone() {
		return None[string]()
	}
	return Some(opt.Unwrap())
}

func toWitTuple2(list []AlloyTuple2StringStringT) []wit.AlloyTuple2StringStringT {
	if list == nil {
		return nil
	}
	res := make([]wit.AlloyTuple2StringStringT, len(list))
	for i, t := range list {
		res[i] = wit.AlloyTuple2StringStringT{F0: t.F0, F1: t.F1}
	}
	return res
}

func fromWitTuple2(list []wit.AlloyTuple2StringStringT) []AlloyTuple2StringStringT {
	if list == nil {
		return nil
	}
	res := make([]AlloyTuple2StringStringT, len(list))
	for i, t := range list {
		res[i] = AlloyTuple2StringStringT{F0: t.F0, F1: t.F1}
	}
	return res
}

func toWitMessage(m AlloyMessage) wit.AlloyMessage {
	return wit.AlloyMessage{
		Id:        m.Id,
		MsgType:   m.MsgType,
		Method:    m.Method,
		Sender:    m.Sender,
		Target:    toWitOptionString(m.Target),
		Payload:   m.Payload,
		Timestamp: m.Timestamp,
	}
}

func fromWitMessage(m wit.AlloyMessage) AlloyMessage {
	return AlloyMessage{
		Id:        m.Id,
		MsgType:   m.MsgType,
		Method:    m.Method,
		Sender:    m.Sender,
		Target:    fromWitOptionString(m.Target),
		Payload:   m.Payload,
		Timestamp: m.Timestamp,
	}
}

func toWitCapability(c AlloyCapability) wit.AlloyCapability {
	shortcut := toWitOptionString(c.Shortcut)
	var annots wit.Option[[]wit.AlloyTuple2StringStringT]
	if c.Annotations.IsSome() {
		annots = wit.Some(toWitTuple2(c.Annotations.Unwrap()))
	} else {
		annots = wit.None[[]wit.AlloyTuple2StringStringT]()
	}
	return wit.AlloyCapability{
		Method:      c.Method,
		Description: c.Description,
		Shortcut:    shortcut,
		Annotations: annots,
	}
}

func fromWitCapability(c wit.AlloyCapability) AlloyCapability {
	var annots Option[[]AlloyTuple2StringStringT]
	if c.Annotations.IsSome() {
		annots = Some(fromWitTuple2(c.Annotations.Unwrap()))
	} else {
		annots = None[[]AlloyTuple2StringStringT]()
	}
	return AlloyCapability{
		Method:      c.Method,
		Description: c.Description,
		Shortcut:    fromWitOptionString(c.Shortcut),
		Annotations: annots,
	}
}

func toWitWorkspace(w AlloyWorkspace) wit.AlloyWorkspace {
	return wit.AlloyWorkspace{
		Id:       w.Id,
		Name:     w.Name,
		Path:     w.Path,
		TeamId:   toWitOptionString(w.TeamId),
		Layout:   toWitOptionString(w.Layout),
		Metadata: toWitTuple2(w.Metadata),
	}
}

func fromWitWorkspace(w wit.AlloyWorkspace) AlloyWorkspace {
	return AlloyWorkspace{
		Id:       w.Id,
		Name:     w.Name,
		Path:     w.Path,
		TeamId:   fromWitOptionString(w.TeamId),
		Layout:   fromWitOptionString(w.Layout),
		Metadata: fromWitTuple2(w.Metadata),
	}
}

func fromWitBuffer(b wit.AlloyBuffer) AlloyBuffer {
	return AlloyBuffer{
		Id:           b.Id,
		Name:         b.Name,
		Content:      b.Content,
		LastModified: b.LastModified,
		MimeType:     b.MimeType,
	}
}

func toWitWidget(w AlloyWidget) wit.AlloyWidget {
	return wit.AlloyWidget{
		Id:                w.Id,
		Title:             w.Title,
		ContentType:       w.ContentType,
		Content:           w.Content,
		RefreshIntervalMs: w.RefreshIntervalMs,
	}
}
