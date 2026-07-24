<script lang="ts" setup="">
import Logo from "@/components/Logo.vue";
import { Button } from "@/components/ui/button";
import { version } from "../../package.json";
</script>

<template>
  <header class="fixed top-0 left-0 right-0 z-10 border-b bg-card">
    <div class="h-14 mx-auto w-full max-w-5xl px-4 flex items-center gap-x-6">
      <Logo />
      <nav class="flex items-center gap-x-0.5 ml-auto">
        <Button size="sm" variant="ghost">Projects</Button>
        <Button size="sm" variant="ghost">Tasks</Button>
        <Button size="sm" variant="ghost">Agents</Button>
      </nav>
    </div>
  </header>

  <RouterView />

  <footer class="fixed bottom-0 left-0 right-0 z-10 border-t bg-card">
    <div class="h-14 mx-auto w-full max-w-5xl px-4 flex items-center gap-x-6">
      <div>
        <div class="text-sm font-medium">KB SITEMATIKA-26</div>
        <div class="text-xs text-muted-foreground">
          TOK — TASK OPERATIONS KERNEL ver. {{ version }}
        </div>
      </div>
    </div>
  </footer>
</template>
