import { defineConfig, devices } from "@playwright/test";

// End-to-end acceptance flows against a real API and database.
//
// Each run starts its own API on :8091 with a brand-new database in the
// disposable test replica set (docker compose -f docker-compose.test.yml
// up -d --wait), migrates it, and a Vite dev server on :5191 proxying to
// it. Nothing touches the dev database. Uses the installed Chrome.
//
//   npm run e2e
const run = `bronze_e2e_${Date.now()}`;
const mongo = process.env.E2E_MONGODB_URI ?? "mongodb://localhost:27027/?replicaSet=rs0";
const apiEnv = { MONGODB_URI: mongo, DB_NAME: run, PORT: "8091", STUDIO_TZ: "Asia/Beirut", CORS_ORIGINS: "*" };

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  timeout: 45_000,
  use: {
    baseURL: "http://localhost:5191",
    channel: "chrome",
    trace: "retain-on-failure",
  },
  projects: [
    { name: "phone", use: { ...devices["Pixel 7"], channel: "chrome" } },
    { name: "desktop", use: { viewport: { width: 1280, height: 800 }, channel: "chrome" } },
  ],
  webServer: [
    {
      command: "go run ./cmd/migrate && go run ./cmd/server",
      cwd: "../backend",
      env: apiEnv,
      url: "http://localhost:8091/api/health",
      timeout: 180_000,
      reuseExistingServer: false,
    },
    {
      command: "npx vite --port 5191 --strictPort",
      env: { API_TARGET: "http://localhost:8091" },
      url: "http://localhost:5191",
      timeout: 60_000,
      reuseExistingServer: false,
    },
  ],
});
