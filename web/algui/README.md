# alGUI

A tiny WebAssembly dashboard that calls the alert server's configuration endpoint and renders the values inside a static card.

## Building

```bash
cd web/algui
GOOS=js GOARCH=wasm go build -o main.wasm
```

The `main.wasm` artifact and `wasm_exec.js` loader are both served directly by the alert server from the `web/algui` directory.
