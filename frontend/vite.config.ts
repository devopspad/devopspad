import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          editor: [
            "codemirror",
            "@codemirror/lang-sql",
            "@codemirror/lang-json",
          ],
        },
      },
    },
  },
  server: { port: 3415, strictPort: true },
});
