<script lang="ts" setup="">
import type { Project } from "@/components/pages/projects/index.ts";
import AgentIcon from "@/components/common/agent/AgentIcon.vue";
import IcBaselineTaskAlt from "~icons/ic/baseline-task-alt";

const props = defineProps<{
  project: Project;
}>();
</script>

<template>
  <div class="flex items-center gap-4 px-4 py-2">
    <div>
      <div class="font-medium">{{ props.project.displayName }}</div>
      <div class="text-sm text-muted-foreground">{{ props.project.path }}</div>
    </div>
    <div class="ml-auto flex items-center gap-x-4">
      <div class="flex -space-x-1.5">
        <div
          v-for="a in props.project.agents"
          :key="a"
          class="rounded-full ring-1 ring-foreground/10 transition-all hover:z-10 hover:scale-110 flex items-center justify-center"
        >
          <AgentIcon :value="a" class="size-6" />
        </div>
      </div>

      <div class="flex space-x-1.5">
        <IcBaselineTaskAlt class="size-5" />
        <div class="min-w-10 text-sm">{{ props.project.tasksCount }}</div>
      </div>

      <!--      <div class="flex items-center -gap-x-2">-->
      <!--        <div-->
      <!--          v-for="a in props.project.agents"-->
      <!--          class="size-8 rounded-full bg-accent/50 flex items-center justify-center"-->
      <!--        ></div>-->
      <!--      </div>-->
    </div>
  </div>
</template>
