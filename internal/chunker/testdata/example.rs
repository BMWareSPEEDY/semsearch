// Example Rust module for testing

use std::collections::HashMap;

pub struct User {
    name: String,
    age: u32,
}

impl User {
    pub fn new(name: &str, age: u32) -> Self {
        User {
            name: name.to_string(),
            age,
        }
    }

    pub fn greet(&self) -> String {
        format!("Hello, {}!", self.name)
    }
}

pub fn compute_hash(data: &str) -> u64 {
    let mut hash = 0u64;
    for byte in data.bytes() {
        hash = hash.wrapping_mul(31).wrapping_add(byte as u64);
    }
    hash
}

#[derive(Debug)]
pub enum Status {
    Active,
    Inactive,
    Pending,
}

pub fn main() {
    let user = User::new("Alice", 30);
    println!("{}", user.greet());
}
