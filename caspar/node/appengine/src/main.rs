mod prelude;
mod globals;
mod models;
mod bridge;
mod controllers;
mod host;
mod network;
mod bootstrap;

fn main() {
    bootstrap::run();
}
