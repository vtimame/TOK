<script lang="ts" setup="">
const props = defineProps<{
  label?: string;
  hint?: string;
  errors?: string[];
  required?: boolean;
}>();
</script>

<template>
  <div class="space-y-2">
    <div v-if="props.label" class="text-sm font-medium">
      {{ props.label }}<span v-if="props.required" class="text-destructive">*</span>
    </div>

    <slot />

    <div v-if="props.hint || props?.errors?.[0]" class="text-xs">
      <div v-if="props.hint" class="text-muted-foreground">{{ props.hint }}</div>
      <div v-else-if="props.errors?.[0]" class="text-destructive">{{ props.errors?.[0] }}</div>
    </div>
  </div>
</template>
