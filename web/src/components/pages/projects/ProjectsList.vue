<script lang="ts" setup="">
import type { Project } from "@/components/pages/projects/index.ts";
import ProjectRow from "@/components/pages/projects/ProjectRow.vue";
import { cn } from "@/lib/utils.ts";
import { RouterLink } from "vue-router";

const props = defineProps<{
  projects: Project[];
}>();
</script>

<template>
  <div class="divide-y">
    <RouterLink
      v-for="project in props.projects"
      :key="project.id"
      :to="`/projects/${project.name}`"
      :class="cn('cursor-pointer hover:bg-foreground/5 first:rounded-t-lg')"
      class="block"
    >
      <ProjectRow :project="project" />
    </RouterLink>
  </div>
</template>
