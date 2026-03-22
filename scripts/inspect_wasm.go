package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tetratelabs/wazero"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: inspect <wasm-file>")
		os.Exit(1)
	}

	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	defer r.Close(ctx)

	wasmBytes, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	compiled, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		fmt.Printf("Error compiling module: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Imports for %s:\n", os.Args[1])
	for _, imp := range compiled.ImportedFunctions() {
		mod, name, _ := imp.Import()
		if mod == "alloy" {
			params := imp.ParamTypes()
			results := imp.ResultTypes()
			fmt.Printf("  alloy.%s params: %v results: %v\n", name, params, results)
		}
	}
}
