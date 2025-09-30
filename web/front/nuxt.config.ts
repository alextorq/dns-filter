// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',
  ssr: false,
  devtools: { enabled: true },
  devServer: { port: 4000},
  modules: ['@nuxt/ui'],
  css: ['~/assets/css/main.css'],
  nitro: {
     preset: 'static'
  },

  // Указать, какие страницы нужно пререндерить
  routeRules: {
     '/**': { prerender: true }
  }
})
