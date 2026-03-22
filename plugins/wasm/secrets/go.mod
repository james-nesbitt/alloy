module github.com/james-nesbitt/alloy/plugins/wasm/secrets

go 1.25.8

replace github.com/james-nesbitt/alloy => ../../../

replace github.com/james-nesbitt/alloy/wit => ../../../wit

replace github.com/james-nesbitt/alloy/build/gen/bindings/guest => ../../../build/gen/bindings/guest

replace github.com/james-nesbitt/alloy/pkg/wasm/guest => ../../../pkg/wasm/guest

require (
	github.com/james-nesbitt/alloy/build/gen/bindings/guest v0.0.0-00010101000000-000000000000
	github.com/james-nesbitt/alloy/pkg/wasm/guest v0.0.0-00010101000000-000000000000
)
