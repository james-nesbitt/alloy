package guest

type mockHost struct {
	caps []AlloyCapability
}

func (m *mockHost) Init(id string, caps []AlloyCapability) {
	m.caps = caps
}

func (m *mockHost) Started() {}

func (m *mockHost) GetNextMessage() Option[AlloyMessage] {
	return None[AlloyMessage]()
}

func (m *mockHost) SendResponse(msg AlloyMessage) {}

func (m *mockHost) Log(level string, message string) {}

func (m *mockHost) KvSet(key string, value []byte) bool {
	return true
}

func (m *mockHost) KvGet(key string) Option[[]byte] {
	return None[[]byte]()
}

func (m *mockHost) KvDelete(key string) bool {
	return true
}

func (m *mockHost) KvList(prefix string) []string {
	return []string{}
}

func (m *mockHost) RouteMessage(msg AlloyMessage) {}

func (m *mockHost) Call(msg AlloyMessage) AlloyMessage {
	return AlloyMessage{}
}

func (m *mockHost) RegisterCapability(cap AlloyCapability) {}

func (m *mockHost) UnregisterCapability(method string) {}

func (m *mockHost) FindProviders(method, actor string, contextID string) []string {
	return []string{}
}

func (m *mockHost) GetAllCapabilities(actor string, contextID string) []AlloyCapability {
	return []AlloyCapability{}
}

func (m *mockHost) ReadBuffer(id string) Option[AlloyBuffer] {
	return None[AlloyBuffer]()
}

func (m *mockHost) WriteBuffer(id string, content []byte) bool {
	return true
}

func (m *mockHost) ListBuffers() []string {
	return []string{}
}

func (m *mockHost) GetBufferView(id string) (ptr, size uint32, ok bool) {
	return 0, 0, false
}

func (m *mockHost) RegisterWidget(w AlloyWidget) {}

func (m *mockHost) UnregisterWidget(id string) {}

func (m *mockHost) UpdateWidget(id string, content []byte) {}
