// Composable managing admin page state: hubs, models, and flattened endpoint rows.
import { ref, computed, type Ref } from 'vue'
import { listHubs } from '@/api/hubs'
import { listModels } from '@/api/models'
import type { Hub, Model, Endpoint } from '@/api/types'

export interface EndpointRow {
  endpoint: Endpoint
  modelName: string
  hubName: string
  modelDbId: number
  modelOrigin: string // "manual" | "discovered"
  modelFamily: string // vendor series, "other" when unmatched
  modelCapability: string // "chat" | "image" | "video" | ... (editable, GH #105)
}

// A model with zero endpoints: invisible in the endpoint table, managed via
// the endpointless-model section (re-trial / delete).
export interface EndpointlessModelRow {
  model: Model
  hubName: string
}

export function useAdminData() {
  const hubs: Ref<Hub[]> = ref([])
  const models: Ref<Model[]> = ref([])
  const loading = ref(false)

  // Flattened endpoint rows: each row includes endpoint data + model name + hub name.
  const endpointRows = computed<EndpointRow[]>(() => {
    const hubMap = new Map(hubs.value.map(h => [h.id, h.name]))
    const rows: EndpointRow[] = []
    for (const model of models.value) {
      const hubName = hubMap.get(model.hub_id) ?? '(unknown)'
      for (const endpoint of model.endpoints) {
        rows.push({
          endpoint,
          modelName: model.model_id,
          hubName,
          modelDbId: model.id,
          modelOrigin: model.origin,
          modelFamily: model.family,
          modelCapability: model.capability,
        })
      }
    }
    return rows
  })

  // Models with no endpoint at all (all endpoints deleted, or every protocol
  // trial failed at registration). They produce no endpoint row, so they are
  // surfaced separately to stay visible and manageable.
  const endpointlessRows = computed<EndpointlessModelRow[]>(() => {
    const hubMap = new Map(hubs.value.map(h => [h.id, h.name]))
    const rows: EndpointlessModelRow[] = []
    for (const model of models.value) {
      if (model.endpoints.length > 0) continue
      rows.push({ model, hubName: hubMap.get(model.hub_id) ?? '(unknown)' })
    }
    return rows
  })

  async function reloadHubs() {
    loading.value = true
    try {
      hubs.value = await listHubs()
    } finally {
      loading.value = false
    }
  }

  async function reloadModels() {
    loading.value = true
    try {
      models.value = await listModels()
    } finally {
      loading.value = false
    }
  }

  async function reloadAll() {
    await Promise.all([reloadHubs(), reloadModels()])
  }

  return { hubs, models, endpointRows, endpointlessRows, loading, reloadHubs, reloadModels, reloadAll }
}
