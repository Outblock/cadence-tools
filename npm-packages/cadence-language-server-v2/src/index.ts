/*
 * Cadence Language Server v2 — Main-thread API
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

/**
 * Options for creating a CadenceLanguageServer instance.
 */
export interface CadenceLanguageServerOptions {
  /**
   * URL to the compiled WASM binary (`cadence-language-server.wasm`).
   * Can be a relative or absolute URL.
   */
  wasmUrl: string;

  /**
   * URL to the Web Worker script (`worker.js`).
   * If omitted, the caller must provide a pre-constructed Worker
   * via the `worker` option instead.
   */
  workerUrl?: string;

  /**
   * A pre-constructed Web Worker. When provided, `workerUrl` is ignored.
   * This is useful when the host bundles the worker via Vite:
   * `new Worker(new URL('./worker', import.meta.url))`
   */
  worker?: Worker;

  /**
   * Flow REST API access node URL.
   * Defaults to mainnet: "https://rest-mainnet.onflow.org"
   */
  accessNode?: string;

  /**
   * Called when the LSP sends a JSON-RPC message to the client.
   * The `message` is a plain JSON string.
   */
  onMessage?: (message: string) => void;

  /**
   * Called when the worker signals an error.
   */
  onError?: (error: string) => void;

  /**
   * Called when the WASM LSP is ready to receive messages.
   */
  onReady?: () => void;
}

/**
 * CadenceLanguageServer manages a Cadence LSP v2 instance running
 * inside a Web Worker + WASM binary.
 *
 * Address imports (e.g. `import X from 0xADDR`) are resolved inside the
 * Worker via synchronous XHR to the Flow REST API, so the main thread
 * stays unblocked.
 *
 * Usage:
 * ```ts
 * import { CadenceLanguageServer } from "@outblock/cadence-language-server";
 *
 * const lsp = await CadenceLanguageServer.create({
 *   wasmUrl: "/cadence-language-server.wasm",
 *   workerUrl: "/cadence-lsp-worker.js",
 *   onMessage(msg) { console.log("from server:", msg); },
 * });
 *
 * lsp.sendToServer(jsonRpcMessage);
 * lsp.dispose();
 * ```
 */
export class CadenceLanguageServer {
  private worker: Worker;
  private disposed = false;

  private constructor(worker: Worker) {
    this.worker = worker;
  }

  /**
   * Create and initialize a new CadenceLanguageServer instance.
   * Resolves once the WASM binary is loaded and the LSP is ready.
   */
  static create(options: CadenceLanguageServerOptions): Promise<CadenceLanguageServer> {
    const worker = options.worker ?? new Worker(options.workerUrl!, { type: "classic" });
    const instance = new CadenceLanguageServer(worker);

    return new Promise<CadenceLanguageServer>((resolve, reject) => {
      const onMessage = (event: MessageEvent) => {
        const data = event.data;
        if (!data || typeof data !== "object") return;

        switch (data.type) {
          case "ready":
            options.onReady?.();
            resolve(instance);
            break;
          case "fromServer":
            options.onMessage?.(data.message);
            break;
          case "error":
            options.onError?.(data.error);
            reject(new Error(data.error));
            break;
        }
      };

      const onError = (event: ErrorEvent) => {
        const msg = event.message ?? "Worker error";
        options.onError?.(msg);
        reject(new Error(msg));
      };

      worker.addEventListener("message", onMessage);
      worker.addEventListener("error", onError);

      worker.postMessage({
        type: "init",
        wasmUrl: options.wasmUrl,
        accessNode: options.accessNode,
      });
    });
  }

  /**
   * Send a JSON-RPC message (plain JSON string) to the LSP server.
   */
  sendToServer(message: string): void {
    if (this.disposed) {
      throw new Error("CadenceLanguageServer has been disposed");
    }
    this.worker.postMessage({ type: "toServer", message });
  }

  /**
   * Update the Flow REST API access node URL.
   */
  setAccessNode(accessNode: string): void {
    this.worker.postMessage({ type: "setConfig", accessNode });
  }

  /**
   * Push local file content for string import resolution.
   * Call this for each local .cdc file so the LSP can resolve
   * `import "MyContract"` style imports.
   */
  setStringCode(location: string, code: string): void {
    this.worker.postMessage({ type: "setStringCode", location, code });
  }

  /**
   * Clear all string code mappings.
   */
  clearStringCode(): void {
    this.worker.postMessage({ type: "clearStringCode" });
  }

  /**
   * Pre-populate the address code cache so the worker doesn't
   * need to fetch from the REST API.
   */
  preloadAddressCode(address: string, contractName: string, code: string): void {
    this.worker.postMessage({ type: "preloadAddressCode", address, contractName, code });
  }

  /**
   * Terminate the Web Worker and release resources.
   */
  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.worker.terminate();
  }
}
