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
 * Options for creating a CadenceLanguageServerV2 instance.
 */
export interface CadenceLanguageServerOptions {
  /**
   * URL to the compiled WASM binary (`cadence-language-server.wasm`).
   * Can be a relative or absolute URL.
   */
  wasmUrl: string;

  /**
   * URL to the Web Worker script (`worker.js` — the compiled output
   * of `src/worker.ts`). If omitted, the caller must provide a
   * pre-constructed Worker via the `worker` option instead.
   */
  workerUrl?: string;

  /**
   * A pre-constructed Web Worker. When provided, `workerUrl` is ignored.
   * This is useful when the host bundles the worker via a tool like
   * Vite (`new Worker(new URL('./worker.ts', import.meta.url))`).
   */
  worker?: Worker;

  /**
   * Called when the LSP sends a JSON-RPC message to the client.
   * The `message` is a plain JSON string (no Content-Length framing).
   */
  onMessage?: (message: string) => void;

  /**
   * Called when the worker signals an error.
   */
  onError?: (error: string) => void;

  /**
   * Called when the WASM LSP has finished initialization and is
   * ready to receive JSON-RPC messages.
   */
  onReady?: () => void;
}

/**
 * CadenceLanguageServerV2 manages a Cadence LSP instance running
 * inside a Web Worker + WASM binary.
 *
 * Communication uses `postMessage` rather than global function tables,
 * making it safe for multiple instances and compatible with modern
 * CSP policies.
 *
 * Usage:
 * ```ts
 * const lsp = await CadenceLanguageServerV2.create({
 *   wasmUrl: '/cadence-language-server.wasm',
 *   workerUrl: '/cadence-lsp-worker.js',
 *   onMessage(msg) { // forward to your LSP client transport },
 *   onReady() { console.log('LSP ready'); },
 * });
 *
 * // Send JSON-RPC messages from the client to the server:
 * lsp.sendToServer(jsonRpcMessage);
 *
 * // When done:
 * lsp.dispose();
 * ```
 */
export class CadenceLanguageServerV2 {
  private worker: Worker;
  private disposed = false;

  private constructor(worker: Worker) {
    this.worker = worker;
  }

  /**
   * Create and initialize a new CadenceLanguageServerV2 instance.
   * Resolves once the WASM binary is loaded and the LSP is ready.
   */
  static create(options: CadenceLanguageServerOptions): Promise<CadenceLanguageServerV2> {
    const worker = options.worker ?? new Worker(options.workerUrl!, { type: "classic" });
    const instance = new CadenceLanguageServerV2(worker);

    return new Promise<CadenceLanguageServerV2>((resolve, reject) => {
      const onMessage = (event: MessageEvent) => {
        const data = event.data;
        if (!data || typeof data !== "object") return;

        switch (data.type) {
          case "ready":
            resolve(instance);
            break;
          case "fromServer":
            options.onMessage?.(data.message);
            break;
          case "error":
            options.onError?.(data.error);
            // If we haven't resolved yet, this is a startup error.
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

      // Tell the worker to initialize.
      worker.postMessage({ type: "init", wasmUrl: options.wasmUrl });
    });
  }

  /**
   * Send a JSON-RPC message (plain JSON string) to the LSP server.
   */
  sendToServer(message: string): void {
    if (this.disposed) {
      throw new Error("CadenceLanguageServerV2 has been disposed");
    }
    this.worker.postMessage({ type: "toServer", message });
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
