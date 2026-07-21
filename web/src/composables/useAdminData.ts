// Composable managing admin page state: hubs, models, and flattened endpoint rows.
import { ref, computed, type Ref } from 'vue'
import { listHubs } from '@/api/hubs'
import { listModels } from '@/api/models'
import type { Hub, Model, Endpoint } from '@/api/types'

export interface EndpointRow {
  endpoint: Endpoint
  modelName: string
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
        rows.push({ endpoint, modelName: model.model_id, hubName })
      }
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

  return { hubs, models, endpointRows, loading, reloadHubs, reloadModels, reloadAll }
}
