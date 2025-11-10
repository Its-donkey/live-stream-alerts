# Web Assets

The `web/` directory houses client-facing assets served directly by the alert server. The primary UI lives in `web/algui`, a Go-based WebAssembly build that now mirrors the original React landing page (hero content, streamer roster, and partner call-to-action).

## Building alGUI
1. Navigate to the folder:
   ```bash
   cd web/algui
   ```
2. Compile the WASM bundle:
   ```bash
   GOOS=js GOARCH=wasm go build -o main.wasm
   ```
3. Ensure `wasm_exec.js` ships beside `main.wasm`. The Go toolchain provides this file (`$(go env GOROOT)/misc/wasm/wasm_exec.js`).

The alert server automatically serves every asset under `web/algui`, including `styles.css`, the static `streamers.json` fallback used when an API is unavailable, and the interactive streamer submission form. Generated artifacts such as `main.wasm` are ignored via `.gitignore`.
