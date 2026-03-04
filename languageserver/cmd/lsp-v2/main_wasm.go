//go:build wasm
// +build wasm

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
	"time"

	"github.com/onflow/cadence-tools/languageserver/server2"
	"github.com/onflow/cadence-tools/languageserver/transport"
)

func main() {
	config := server2.ServerConfig{
		// In WASM mode we start with no import resolver (nil).
		// The JS host will provide imports via a JS-backed resolver
		// wired through the WASM bridge in a follow-up change.
		ImportResolver: nil,
		CacheCapacity:  128,
		DebounceDelay:  200 * time.Millisecond,
	}

	srv := server2.NewServerV2(config)
	<-transport.RunWASM(srv)
}
