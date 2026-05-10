import { defineConfig } from "vitest/config";
import { fileURLToPath } from "node:url";

export default defineConfig({
    test: {
        environment: "happy-dom",
        globals: true,
        include: ["composables/**/*.test.ts", "utils/**/*.test.ts", "app/**/*.test.ts"],
        setupFiles: ["./test/setup.ts"],
    },
    resolve: {
        alias: {
            "~~": fileURLToPath(new URL("./", import.meta.url)),
            "~": fileURLToPath(new URL("./app", import.meta.url)),
        },
    },
});
