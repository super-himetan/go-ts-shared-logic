import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "e2e",
  use: { baseURL: "http://localhost:8787" },
  projects: [{ name: "chromium", use: { browserName: "chromium" } }],
  webServer: [
    // The production path: what `vite build` writes, served by the Go server.
    { command: "pnpm build && go run ./server", url: "http://localhost:8787", timeout: 180_000 },
    {
      command:
        "vite build --config e2e/multi/vite.config.js && vite preview --config e2e/multi/vite.config.js --port 5186 --strictPort",
      url: "http://localhost:5186",
      timeout: 180_000,
    },
  ],
});
