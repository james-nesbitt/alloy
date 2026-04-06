module github.com/james-nesbitt/alloy/plugins/wasm/filesystem

go 1.25.8

replace github.com/james-nesbitt/alloy/build/gen/bindings/guest => ../../../build/gen/bindings/guest

replace github.com/james-nesbitt/alloy/pkg/wasm/guest => ../../../pkg/wasm/guest

require github.com/james-nesbitt/alloy/pkg/wasm/guest v0.0.0-00010101000000-000000000000

require github.com/james-nesbitt/alloy/build/gen/bindings/guest v0.0.0-00010101000000-000000000000 // indirect
