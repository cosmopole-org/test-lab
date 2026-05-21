# wasmedge-sys Overview

The [`wasmedge-sys`](https://crates.io/crates/wasmedge-sys) crate provides low-level Rust APIs for WasmEdge, a lightweight high-performance WebAssembly runtime for cloud-native, edge, and decentralized applications.

For most developers:

- use `wasmedge-sys` to construct high-level libraries
- use `wasmedge-sdk` for business application development

> Requires **Rust 1.70+** on stable.

## Build

This crate depends on the WasmEdge C API.

- On Linux/macOS, enabling the `standalone` feature can download the C API during build.
- Otherwise, install the C API on your system first.

See [Get Started](https://github.com/WasmEdge/wasmedge-rust-sdk#get-started) for details.

## API Reference

- [wasmedge-sys API Reference](https://wasmedge.github.io/wasmedge-rust-sdk/wasmedge_sys/index.html)
- [wasmedge-sys Async API Reference](https://second-state.github.io/wasmedge-async-rust-sdk/wasmedge_sys/index.html)

## See Also

- [WasmEdge Runtime Official Website](https://wasmedge.org/)
- [WasmEdge Docs](https://wasmedge.org/book/en/)
- [WasmEdge C API Documentation](https://github.com/WasmEdge/WasmEdge/blob/master/docs/c_api.md)
