// Example JavaScript module for testing

import fs from 'fs';

/**
 * A utility class for processing data.
 */
export class DataProcessor {
  constructor(options = {}) {
    this.options = options;
    this.cache = new Map();
  }

  /**
   * Process the given input data.
   * @param {string} input - Input string
   * @returns {string} Processed output
   */
  process(input) {
    const key = input;
    if (this.cache.has(key)) {
      return this.cache.get(key);
    }
    const result = input.toUpperCase();
    this.cache.set(key, result);
    return result;
  }

  clearCache() {
    this.cache.clear();
  }
}

/**
 * A standalone helper function.
 */
export function formatOutput(value) {
  return `Result: ${value}`;
}
