import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config";

// Unit tests for derived UI rules (money display, dues states, dates, search
// ranking). jsdom gives components a DOM; nothing here talks to the API.
export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      environment: "jsdom",
      include: ["src/**/*.test.ts"],
      env: { TZ: "UTC" },
    },
  }),
);
