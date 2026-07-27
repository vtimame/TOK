/// <reference types="vitest/config" />

import path from "node:path";
import { defineConfig } from "vite";
import tailwindcss from "@tailwindcss/vite";
import vue from "@vitejs/plugin-vue";
import VueRouter from "vue-router/vite";
import Icons from "unplugin-icons/vite";

export default defineConfig({
  plugins: [
    VueRouter(),
    vue(),
    tailwindcss(),
    Icons({
      compiler: "vue3",
      autoInstall: true,
    }),
  ],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    proxy: {
      "/api": {
        target: process.env.VITE_TOK_API_PROXY_TARGET || "http://127.0.0.1:7654",
        changeOrigin: true,
      },
      "/swagger": {
        target: process.env.VITE_TOK_API_PROXY_TARGET || "http://127.0.0.1:7654",
        changeOrigin: true,
      },
    },
  },
  test: {
    include: ["src/**/*.test.ts", "src/**/*.spec.ts"],
    environment: "node",
    globals: true,
  },
});
