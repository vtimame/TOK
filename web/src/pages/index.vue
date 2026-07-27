<script lang="ts" setup="">
import { version } from "../../package.json";
import Logo from "@/components/Logo.vue";
import { Button } from "@/components/ui/button";
import { useRoute, useRouter } from "vue-router";
import { cn } from "@/lib/utils.ts";

type NavItem = { href: string; label: string };

const route = useRoute();
const router = useRouter();

const navItems: NavItem[] = [
  { href: "/projects", label: "Projects" },
  { href: "/tasks", label: "Tasks" },
  { href: "/agents", label: "Agents" },
];
</script>

<template>
  <header class="fixed top-0 left-0 right-0 z-10 border-b bg-card">
    <div class="h-14 mx-auto w-full max-w-6xl px-4 flex items-center gap-x-6">
      <Logo />
      <nav class="flex items-center gap-x-0.5 ml-auto">
        <Button
          v-for="ni in navItems"
          :key="ni.href"
          :class="cn(route.path.startsWith(ni.href) && 'bg-accent/50')"
          size="sm"
          variant="ghost"
          as-child
          @click.prevent="router.push(ni.href)"
        >
          <a :href="ni.href">{{ ni.label }}</a>
        </Button>
      </nav>
    </div>
  </header>

  <RouterView />

  <footer class="fixed bottom-0 left-0 right-0 z-10 border-t bg-card">
    <div class="h-14 mx-auto w-full max-w-6xl px-4 flex items-center gap-x-6">
      <div>
        <div class="text-xs text-muted-foreground">ver. {{ version }}</div>
      </div>
    </div>
  </footer>
</template>
