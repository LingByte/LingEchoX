import { get, post, put, del, type ApiResponse } from '@/utils/request'
import type { Paginated } from '@/api/types'

export interface SIPScriptTemplateRow {
  id: number
  name: string
  scriptId: string
  version?: string
  description?: string
  enabled: boolean
  scriptSpec: unknown
  createdAt?: string
  updatedAt?: string
}

export async function listSIPScriptTemplates(page = 1, size = 20, opts?: { scriptId?: string; name?: string }): Promise<ApiResponse<Paginated<SIPScriptTemplateRow>>> {
  const q = new URLSearchParams({ page: String(page), size: String(size) })
  if (opts?.scriptId) q.set('scriptId', opts.scriptId)
  if (opts?.name) q.set('name', opts.name)
  return get(`/sip-center/scripts?${q.toString()}`)
}

export async function createSIPScriptTemplate(body: {
  name: string
  scriptId?: string
  version?: string
  description?: string
  enabled?: boolean
  scriptSpec: string
}): Promise<ApiResponse<SIPScriptTemplateRow>> {
  return post('/sip-center/scripts', body)
}

export async function updateSIPScriptTemplate(id: number, body: {
  name: string
  scriptId?: string
  version?: string
  description?: string
  enabled?: boolean
  scriptSpec?: string
}): Promise<ApiResponse<SIPScriptTemplateRow>> {
  return put(`/sip-center/scripts/${id}`, body)
}

export async function deleteSIPScriptTemplate(id: number): Promise<ApiResponse<{ id: number }>> {
  return del(`/sip-center/scripts/${id}`)
}
