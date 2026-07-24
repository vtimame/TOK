import { fetch as tokApiClient } from "@/api/generated/.kubb/fetch.ts";

export function setupTokApi() {
  tokApiClient.setConfig({
    baseURL: import.meta.env.VITE_TOK_API_BASE_URL || "",
  });
}
