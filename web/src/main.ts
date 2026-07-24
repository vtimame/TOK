import "@/style.css";
import appEl from "./app.vue";
import { createApp } from "vue";
import { router } from "@/router";
import { VueQueryPlugin } from "@tanstack/vue-query";
import { queryClient } from "@/api/tanstack-query.ts";
import { RegleVuePlugin } from "@regle/core";
import { setupTokApi } from "@/api/client.ts";

async function main() {
  setupTokApi();

  const app = createApp(appEl)
    .use(router)
    .use(VueQueryPlugin, {
      queryClient,
    })
    .use(RegleVuePlugin);

  await router.isReady();
  app.mount("#app");
}

main().catch(console.error);
