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

package transport

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/onflow/cadence-tools/languageserver/protocol"
	"github.com/onflow/cadence-tools/languageserver/server"
)

// RunWASM starts the LSP v2 server in WASM mode.
//
// It registers global JS functions for bidirectional message passing:
//
//   - __CADENCE_LSP_TO_SERVER__(jsonString) — JS calls this to send a
//     JSON-RPC request/response/notification to the Go LSP server.
//
//   - __CADENCE_LSP_SET_CLIENT__(callback) — JS calls this once to provide
//     the callback function that receives outgoing JSON-RPC messages.
//     The callback signature is: (jsonString: string) => void.
//
// After the callbacks are registered, Go sets __CADENCE_LSP_READY__ = true
// so the JS host knows the server is available.
//
// This function blocks until the server connection closes.
func RunWASM(handler protocol.Handler) <-chan struct{} {
	incoming := make(chan json.RawMessage, 64)

	var toClientFn js.Value

	// JS calls this once to provide the server->client callback.
	js.Global().Set("__CADENCE_LSP_SET_CLIENT__", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			return nil
		}
		toClientFn = args[0]
		return nil
	}))

	// JS calls this to send a JSON-RPC message to the server.
	js.Global().Set("__CADENCE_LSP_TO_SERVER__", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) < 1 {
			return nil
		}
		msg := args[0].String()
		incoming <- json.RawMessage(msg)
		return nil
	}))

	// Signal that the server is ready to accept messages.
	js.Global().Set("__CADENCE_LSP_READY__", true)

	fmt.Println("Cadence LSP v2 WASM transport ready")

	stream := server.NewObjectStream(
		// writeObject: Go -> JS
		func(obj any) error {
			data, err := json.Marshal(obj)
			if err != nil {
				return fmt.Errorf("marshal: %w", err)
			}
			toClientFn.Invoke(string(data))
			return nil
		},
		// readObject: JS -> Go (blocks on channel)
		func(v any) error {
			msg, ok := <-incoming
			if !ok {
				return fmt.Errorf("stream closed")
			}
			return json.Unmarshal(msg, v)
		},
		// close
		func() error {
			close(incoming)
			return nil
		},
	)

	protocolServer := protocol.NewServer(handler)
	return protocolServer.Start(stream)
}
