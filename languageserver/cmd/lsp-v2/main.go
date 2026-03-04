//go:build !wasm
// +build !wasm

/*
 * Cadence languageserver - The Cadence language server
 *
 * Copyright Flow Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/pflag"

	"github.com/onflow/cadence-tools/languageserver/resolver"
	"github.com/onflow/cadence-tools/languageserver/server2"
	"github.com/onflow/cadence-tools/languageserver/transport"
)

func main() {
	cacheCapacity := pflag.IntP("cache-capacity", "c", 256, "Max number of cached checkers")
	debounceMs := pflag.IntP("debounce", "d", 150, "Debounce delay in milliseconds")
	rootDir := pflag.StringP("root-dir", "r", "", "Root directory containing flow.json (defaults to CWD)")
	pflag.Parse()

	// Default root-dir to CWD
	if *rootDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to get working directory: %v\n", err)
			os.Exit(1)
		}
		*rootDir = wd
	}

	// Compose resolvers: FlowJSON (contract names) → File (file paths)
	importResolver := resolver.NewCompositeResolver(
		resolver.NewFlowJSONResolver(*rootDir),
		resolver.NewFileResolver(),
	)

	config := server2.ServerConfig{
		ImportResolver: importResolver,
		CacheCapacity:  *cacheCapacity,
		DebounceDelay:  time.Duration(*debounceMs) * time.Millisecond,
	}

	srv := server2.NewServerV2(config)

	fmt.Fprintln(os.Stderr, "Cadence LSP v2 starting on stdio...")
	<-transport.RunStdio(srv)
}
