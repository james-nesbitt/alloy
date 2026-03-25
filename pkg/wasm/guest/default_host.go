//go:build !(wasip1 || wasm)

package guest

func createDefaultHost() HostInterface {
	return NewMockHost()
}
