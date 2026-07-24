<script lang="ts" setup="">
import type { Project } from "@/components/pages/projects/index.ts";
import ProjectRow from "@/components/pages/projects/ProjectRow.vue";
import { cn } from "@/lib/utils.ts";

const props = defineProps<{
  projects: Project[];
}>();
</script>

<template>
  <div class="divide-y">
    <div
      v-for="project in props.projects"
      :key="project.id"
      :class="cn('cursor-pointer hover:bg-foreground/5 first:rounded-t-lg')"
    >
      <ProjectRow :project="project" />
    </div>
  </div>
</template>
