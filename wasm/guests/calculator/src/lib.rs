//! Calculator guest for mcp-gateway WASM runtime.
//!
//! Exports pure C ABI functions (no filesystem / network). Built for
//! `wasm32-unknown-unknown` (no WASI imports required). The Go host still
//! instantiates WASI preview1 for future guests that need it.

#![no_std]

use core::panic::PanicInfo;

#[panic_handler]
fn panic(_info: &PanicInfo) -> ! {
    loop {}
}

/// Saturating add — intentional sandbox-safe arithmetic.
#[no_mangle]
pub extern "C" fn add(a: i32, b: i32) -> i32 {
    a.saturating_add(b)
}

/// Saturating multiply.
#[no_mangle]
pub extern "C" fn mul(a: i32, b: i32) -> i32 {
    a.saturating_mul(b)
}

/// Health probe used by the host after instantiate.
#[no_mangle]
pub extern "C" fn ping() -> i32 {
    1
}
