import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// 開発サーバー専用のプロキシー。本番の配信成果物には含まれず、
// 接続先は環境変数 LMU_API_ORIGIN で指定する（既定は CLI serve の既定アドレス）。
// 本番環境では Gateway が /api を Go API へ振り分けるため、
// ホスト名・サービス名・ポートをコードに焼き込まない。
const apiOrigin = process.env.LMU_API_ORIGIN ?? "http://127.0.0.1:8080";

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/api": { target: apiOrigin, changeOrigin: true },
    },
  },
});
