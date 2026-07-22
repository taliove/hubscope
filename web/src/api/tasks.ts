// Task center API calls (ticket 18).
import { http } from './client'
import type { TaskDetail, TaskPage, TaskStatus, TaskType } from './types'

export interface TaskListQuery {
  type?: TaskType
  status?: TaskStatus
  page?: number
  page_size?: number
}

export async function listTasks(query: TaskListQuery = {}): Promise<TaskPage> {
  const params = new URLSearchParams()
  if (query.type) params.set('type', query.type)
  if (query.status) params.set('status', query.status)
  if (query.page) params.set('page', String(query.page))
  if (query.page_size) params.set('page_size', String(query.page_size))
  const qs = params.toString()
  return http.get<TaskPage>(`/tasks${qs ? `?${qs}` : ''}`)
}

export async function getTask(id: number): Promise<TaskDetail> {
  return http.get<TaskDetail>(`/tasks/${id}`)
}
