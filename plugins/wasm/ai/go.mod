module github.com/jnesbitt/alloy-go/plugins/wasm/ai-agent

go 1.25.8

replace github.com/jnesbitt/alloy-go => ../../../

replace github.com/jnesbitt/alloy-go/wit => ../../../wit

replace github.com/jnesbitt/alloy-go/pkg/wasm/bindings/guest => ../../../pkg/wasm/bindings/guest

replace github.com/jnesbitt/alloy-go/pkg/wasm/guest => ../../../pkg/wasm/guest

require (
	github.com/jnesbitt/alloy-go/pkg/wasm/bindings/guest v0.0.0-00010101000000-000000000000
	github.com/jnesbitt/alloy-go/pkg/wasm/guest v0.0.0-00010101000000-000000000000
)
