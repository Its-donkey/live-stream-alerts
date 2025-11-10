# Web Assets

The `web/` directory houses client-facing assets that the alert server can serve directly. The current implementation lives in `web/algui`, a tiny WebAssembly UI built with Go.

## Building alGUI
1. Navigate to the folder:
   ```bash
   cd web/algui
   ```
2. Compile the WASM bundle:
   ```bash
   GOOS=js GOARCH=wasm go build -o main.wasm
   ```
3. Ensure `wasm_exec.js` is deployed alongside `main.wasm`. The Go toolchain provides this file (`$(go env GOROOT)/misc/wasm/wasm_exec.js`).

The alert server automatically serves everything under `web/algui`. Generated artifacts like `main.wasm` should not be committed; they are ignored via `.gitignore`.
