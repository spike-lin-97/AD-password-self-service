import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  base: "/",
  build: {
    outDir: "../feishu-login/web",
    emptyOutDir: false,
    assetsDir: "assets/login",
  },
});
