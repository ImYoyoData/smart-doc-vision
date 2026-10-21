import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { projectApi } from '@/api'
import type { Project } from '@/api/types'

const PROJECT_KEY = 'sdv.project'

export const useWorkspaceStore = defineStore('workspace', () => {
  const projects = ref<Project[]>([])
  const currentProjectId = ref<number | null>(readId())
  const loading = ref(false)
  const loaded = ref(false)

  const currentProject = computed(() => projects.value.find((p) => p.id === currentProjectId.value) || null)

  const projectOptions = computed(() =>
    projects.value.map((p) => ({
      label: p.name,
      value: p.id,
      env: p.env,
      status: p.status,
    })),
  )

  function readId(): number | null {
    const raw = localStorage.getItem(PROJECT_KEY)
    const id = raw ? Number(raw) : NaN
    return Number.isFinite(id) && id > 0 ? id : null
  }

  function setCurrentProject(id: number | null) {
    currentProjectId.value = id
    if (id) {
      localStorage.setItem(PROJECT_KEY, String(id))
    } else {
      localStorage.removeItem(PROJECT_KEY)
    }
  }

  async function loadProjects(force = false) {
    if (loaded.value && !force) return projects.value
    loading.value = true
    try {
      const data = await projectApi.list({ page: 1, page_size: 100, sort: 'updated_at:desc' })
      projects.value = data.items
      loaded.value = true
      if (!currentProjectId.value && projects.value.length > 0) {
        setCurrentProject(projects.value[0].id)
      }
      if (currentProjectId.value && !projects.value.some((p) => p.id === currentProjectId.value)) {
        setCurrentProject(projects.value[0]?.id ?? null)
      }
      return projects.value
    } finally {
      loading.value = false
    }
  }

  function projectName(id: number) {
    return projects.value.find((p) => p.id === id)?.name || `项目 #${id}`
  }

  return {
    projects,
    currentProjectId,
    currentProject,
    projectOptions,
    loading,
    loaded,
    loadProjects,
    setCurrentProject,
    projectName,
  }
})
