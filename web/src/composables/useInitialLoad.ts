import { computed, ref, watch, type Ref } from 'vue'

// Skeleton is only for the first loading cycle after a page component mounts.
// Later searches, filters and pagination keep the existing content instead of
// flashing the placeholder again.
export function useInitialLoad(loading: Ref<boolean>) {
  const initialized = ref(false)
  watch(
    loading,
    (value) => {
      if (!value) initialized.value = true
    },
    { flush: 'sync' },
  )
  const showSkeleton = computed(() => loading.value && !initialized.value)
  return { showSkeleton }
}
