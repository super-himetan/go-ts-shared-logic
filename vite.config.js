import { goWasm } from "go-mapper/vite";

export default {
  plugins: [goWasm()],
  server: {
    proxy: { "/api": "http://localhost:8787" },
  },
  test: {
    include: ["tests/*.test.ts"],
    // The first test builds the Wasm module, and a cold Go build cache is slow.
    hookTimeout: 180_000,
    testTimeout: 180_000,
    // A cached transform would skip the Go build and run whichever Wasm was built last.
    fsModuleCache: false,
  },
};
