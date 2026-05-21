# Contribution & Design Principles

In general, `*-sys` libraries should keep only `unsafe` C interface bindings and avoid redundant abstraction layers.

The interfaces exposed by `*-sys` crates are expected to be stable. When C interfaces change, update the `*-sys` layer first so upper SDK layers can remain stable.

The [`wasmedge-sys`](https://crates.io/crates/wasmedge-sys) crate follows this approach by providing low-level wrappers around WasmEdge C APIs and safe counterparts that high-level libraries can build upon.
