// https://nuxt.com/docs/api/configuration/nuxt-config
declare const process: { env: Record<string, string | undefined> };

export default defineNuxtConfig({
    compatibilityDate: "2025-07-15",
    ssr: false,
    devtools: { enabled: true },
    devServer: { port: 4000 },
    modules: ["@nuxt/ui", "@nuxt/eslint"],
    eslint: {
        config: {
            stylistic: false,
        },
    },
    css: ["~/assets/css/main.css"],
    experimental: {
        viteEnvironmentApi: true,
    },
    nitro: {
        preset: "static",
        devProxy: {
            "/api": {
                target: process.env.NUXT_DEV_API_TARGET || "http://localhost:8080/api",
                changeOrigin: true,
            },
        },
    },

    // Указать, какие страницы нужно пререндерить
    routeRules: {
        "/**": { prerender: true },
    },
});
