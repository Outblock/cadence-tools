/*
 * Cadence Language Server v2 — Web Worker
 *
 * This script runs inside a Web Worker. It:
 *   1. Loads the Go WASM runtime (wasm_exec.js shim)
 *   2. Instantiates the Cadence LSP WASM binary
 *   3. Bridges postMessage <==> Go global functions
 *
 * Communication protocol (all via postMessage):
 *   Main -> Worker: { type: "init",   wasmUrl: string }
 *   Main -> Worker: { type: "toServer", message: string }
 *   Worker -> Main: { type: "fromServer", message: string }
 *   Worker -> Main: { type: "ready" }
 *   Worker -> Main: { type: "error", error: string }
 */

// Worker global scope — `self` is already declared by the WebWorker lib.

// The Go WASM runtime shim sets this on globalThis.
declare class Go {
  argv: string[];
  env: Record<string, string>;
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

/**
 * Minimal fs polyfill for Go WASM (only writeSync is needed).
 */
function installFsPolyfill(): void {
  const g = globalThis as Record<string, unknown>;
  if (!g.fs) {
    const decoder = new TextDecoder("utf-8");
    let outputBuf = "";
    g.fs = {
      constants: { O_WRONLY: -1, O_RDWR: -1, O_CREAT: -1, O_TRUNC: -1, O_APPEND: -1, O_EXCL: -1 },
      writeSync(fd: number, buf: Uint8Array): number {
        outputBuf += decoder.decode(buf);
        const nl = outputBuf.lastIndexOf("\n");
        if (nl !== -1) {
          console.debug(`[cadence-lsp fd=${fd}]`, outputBuf.substring(0, nl));
          outputBuf = outputBuf.substring(nl + 1);
        }
        return buf.length;
      },
      write(fd: number, buf: Uint8Array, offset: number, length: number, position: null, callback: (err: Error | null, n?: number) => void) {
        if (offset !== 0 || length !== buf.length || position !== null) {
          callback(new Error("not implemented"));
          return;
        }
        const n = (g.fs as { writeSync: (fd: number, buf: Uint8Array) => number }).writeSync(fd, buf);
        callback(null, n);
      },
      chmod(_path: string, _mode: number, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      chown(_path: string, _uid: number, _gid: number, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      close(_fd: number, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      fchmod(_fd: number, _mode: number, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      fchown(_fd: number, _uid: number, _gid: number, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      fstat(_fd: number, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      fsync(_fd: number, callback: (err: null) => void) { callback(null); },
      ftruncate(_fd: number, _length: number, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      lchown(_path: string, _uid: number, _gid: number, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      link(_path: string, _link: string, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      lstat(_path: string, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      mkdir(_path: string, _perm: number, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      open(_path: string, _flags: number, _mode: number, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      read(_fd: number, _buffer: Uint8Array, _offset: number, _length: number, _position: number, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      readdir(_path: string, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      readlink(_path: string, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      rename(_from: string, _to: string, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      rmdir(_path: string, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      stat(_path: string, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      symlink(_path: string, _link: string, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      truncate(_path: string, _length: number, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      unlink(_path: string, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
      utimes(_path: string, _atime: number, _mtime: number, callback: (err: Error) => void) { callback(new Error("ENOSYS")); },
    };
  }

  if (!g.process) {
    g.process = {
      getuid() { return -1; },
      getgid() { return -1; },
      geteuid() { return -1; },
      getegid() { return -1; },
      getgroups() { throw new Error("ENOSYS"); },
      pid: -1,
      ppid: -1,
      umask() { throw new Error("ENOSYS"); },
      cwd() { throw new Error("ENOSYS"); },
      chdir() { throw new Error("ENOSYS"); },
    };
  }
}

async function startLSP(wasmUrl: string): Promise<void> {
  installFsPolyfill();

  // Import the Go WASM runtime shim.
  // The consumer must ensure wasm_exec.js is available (e.g. via importScripts
  // or bundled). We try importScripts first (classic worker), then check globalThis.
  const g = globalThis as Record<string, unknown>;
  if (typeof g.Go === "undefined") {
    // Classic worker: try importScripts with the standard Go shim path.
    // The host page should set __WASM_EXEC_URL__ or we fall back to a
    // sibling path relative to the worker script.
    const shimUrl = (g.__WASM_EXEC_URL__ as string) ?? new URL("wasm_exec.js", self.location.href).href;
    importScripts(shimUrl);
  }

  const go = new (g.Go as typeof Go)();

  // Fetch and instantiate the WASM binary.
  const result = await WebAssembly.instantiateStreaming(fetch(wasmUrl), go.importObject);

  // Start Go (non-blocking -- go.run returns a promise that resolves on exit).
  go.run(result.instance).catch((err: unknown) => {
    self.postMessage({ type: "error", error: String(err) });
  });

  // Wait for Go to register its global functions and signal readiness.
  await waitForReady();

  // Provide the toClient callback to Go. Go registered __CADENCE_LSP_SET_CLIENT__
  // during RunWASM init; we now call it with a function that forwards messages
  // to the main thread via postMessage.
  const setClient = g.__CADENCE_LSP_SET_CLIENT__ as (fn: (msg: string) => void) => void;
  setClient((msg: string) => {
    self.postMessage({ type: "fromServer", message: msg });
  });

  self.postMessage({ type: "ready" });
}

/**
 * Poll until Go sets __CADENCE_LSP_READY__ = true.
 */
function waitForReady(): Promise<void> {
  const g = globalThis as Record<string, unknown>;
  return new Promise((resolve) => {
    const check = () => {
      if (g.__CADENCE_LSP_READY__ === true) {
        resolve();
      } else {
        setTimeout(check, 10);
      }
    };
    check();
  });
}

// Handle messages from the main thread.
self.addEventListener("message", (event: MessageEvent) => {
  const data = event.data;
  if (!data || typeof data !== "object") return;

  switch (data.type) {
    case "init":
      startLSP(data.wasmUrl).catch((err) => {
        self.postMessage({ type: "error", error: String(err) });
      });
      break;

    case "toServer": {
      const g = globalThis as Record<string, unknown>;
      const toServer = g.__CADENCE_LSP_TO_SERVER__ as ((msg: string) => void) | undefined;
      if (typeof toServer === "function") {
        toServer(data.message);
      }
      break;
    }
  }
});
