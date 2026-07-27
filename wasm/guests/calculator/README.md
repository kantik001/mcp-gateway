# Calculator WASM guest

Sandboxed arithmetic tools for mcp-gateway (`runtime: wasm`).

## Build

```bash
rustup target add wasm32-unknown-unknown
cargo build --release --target wasm32-unknown-unknown
cp target/wasm32-unknown-unknown/release/calculator.wasm ../../../internal/mcp/testdata/calculator.wasm
cp target/wasm32-unknown-unknown/release/calculator.wasm ../../../wasm/calculator.wasm
```

Or: `make wasm-calculator` from repo root.

## Exports (C ABI)

| Symbol | Signature |
|--------|-----------|
| `add` | `(i32, i32) -> i32` |
| `mul` | `(i32, i32) -> i32` |
| `ping` | `() -> i32` |

No WASI imports are required for these functions; the Go host still configures a WASI context for future guests that need clocks/args.
