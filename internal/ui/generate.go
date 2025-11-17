package ui

//go:generate sh -c "cd ../../alGUI && GOOS=js GOARCH=wasm go build -o ../live-stream-alerts/internal/ui/dist/main.wasm"
//go:generate cp ../../alGUI/index.html internal/ui/dist/index.html
//go:generate cp ../../alGUI/styles.css internal/ui/dist/styles.css
//go:generate cp ../../alGUI/wasm_exec.js internal/ui/dist/wasm_exec.js
//go:generate cp ../../alGUI/streamers.json internal/ui/dist/streamers.json
