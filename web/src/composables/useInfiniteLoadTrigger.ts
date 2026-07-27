import { useIntersectionObserver } from "@vueuse/core";
import { computed, ref, toValue, watch } from "vue";
import type { MaybeRefOrGetter } from "vue";

type InfiniteLoadTriggerOptions = {
  hasMore: MaybeRefOrGetter<boolean>;
  loading: MaybeRefOrGetter<boolean>;
  itemCount: MaybeRefOrGetter<number>;
  loadMore: () => Promise<unknown> | unknown;
};

export function useInfiniteLoadTrigger(options: InfiniteLoadTriggerOptions) {
  const trigger = ref<HTMLElement | null>(null);
  const visible = ref(false);
  const pending = ref(false);
  const canLoad = computed(() => toValue(options.hasMore) && !toValue(options.loading));

  async function requestLoadMore() {
    if (!visible.value || !canLoad.value || pending.value) return;

    pending.value = true;
    try {
      await options.loadMore();
    } finally {
      pending.value = false;
    }
  }

  useIntersectionObserver(
    trigger,
    ([entry]) => {
      visible.value = Boolean(entry?.isIntersecting);
      void requestLoadMore();
    },
    {
      rootMargin: "96px 0px",
    },
  );

  watch(
    [canLoad, visible, () => toValue(options.itemCount)],
    () => {
      void requestLoadMore();
    },
    { flush: "post" },
  );

  return {
    trigger,
  };
}
