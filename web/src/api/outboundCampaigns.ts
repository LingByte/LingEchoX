import { get, post, del, type ApiResponse } from '@/utils/request'
import type { Paginated } from '@/api/types'

export interface OutboundCampaignRow {
  id: number
  name: string
  status: string
  scenario?: string
  mediaProfile?: string
  scriptId?: string
  createdAt?: string
  updatedAt?: string
}

export interface OutboundCampaignMetrics {
  invited_total: number
  answered_total: number
  failed_total: number
  retrying_total: number
  suppressed_total: number
}

export interface OutboundCampaignWorkerMetrics {
  invited_total: number
  answered_total: number
  failed_total: number
  retrying_total: number
  suppressed_total: number
  task_queued?: number
  task_channel_len?: number
  task_running?: number
  task_unfinished?: number
  per_campaign_queued?: Record<string, number>
  per_campaign_running?: Record<string, number>
}

export interface OutboundCampaignLogRow {
  id: number
  at: string
  type: string
  contactId?: number
  attemptId?: number
  attemptNo?: number
  phone?: string
  callId?: string
  correlationId?: string
  level?: string
  message: string
}

export interface OutboundCampaignContactRow {
  id: number
  campaignId: number
  phone: string
  status: string
  attemptCount?: number
  maxAttempts?: number
  failureReason?: string
  nextRunAt?: string
  lastDialAt?: string
  createdAt?: string
  updatedAt?: string
}

export async function createOutboundCampaign(body: {
  name: string
  scenario: string
  media_profile: string
  script_id?: string
  script_version?: string
  script_spec?: string
}): Promise<ApiResponse<OutboundCampaignRow>> {
  return post('/sip-center/campaigns', body)
}

export async function listOutboundCampaigns(page = 1, size = 20, opts?: { status?: string; name?: string }): Promise<ApiResponse<Paginated<OutboundCampaignRow>>> {
  const q = new URLSearchParams({ page: String(page), size: String(size) })
  if (opts?.status) q.set('status', opts.status)
  if (opts?.name) q.set('name', opts.name)
  return get(`/sip-center/campaigns?${q.toString()}`)
}

export async function enqueueOutboundCampaignContacts(
  campaignId: number,
  contacts: Array<{ phone: string; display?: string; priority?: number; caller_user?: string }>,
): Promise<ApiResponse<{ accepted: number }>> {
  return post(`/sip-center/campaigns/${campaignId}/contacts`, contacts)
}

export async function listOutboundCampaignContacts(campaignId: number, page = 1, size = 50): Promise<ApiResponse<Paginated<OutboundCampaignContactRow>>> {
  const q = new URLSearchParams({ page: String(page), size: String(size) })
  return get(`/sip-center/campaigns/${campaignId}/contacts?${q.toString()}`)
}

export async function resetOutboundCampaignSuppressedContacts(campaignId: number): Promise<ApiResponse<{ updated: number }>> {
  return post(`/sip-center/campaigns/${campaignId}/contacts/reset-suppressed`, {})
}

export async function startOutboundCampaign(campaignId: number): Promise<ApiResponse<null>> {
  return post(`/sip-center/campaigns/${campaignId}/start`, {})
}

export async function pauseOutboundCampaign(campaignId: number): Promise<ApiResponse<null>> {
  return post(`/sip-center/campaigns/${campaignId}/pause`, {})
}

export async function resumeOutboundCampaign(campaignId: number): Promise<ApiResponse<null>> {
  return post(`/sip-center/campaigns/${campaignId}/resume`, {})
}

export async function stopOutboundCampaign(campaignId: number): Promise<ApiResponse<null>> {
  return post(`/sip-center/campaigns/${campaignId}/stop`, {})
}

export async function deleteOutboundCampaign(campaignId: number): Promise<ApiResponse<{ id: number }>> {
  return del(`/sip-center/campaigns/${campaignId}`)
}

export async function getOutboundCampaignMetrics(): Promise<ApiResponse<OutboundCampaignMetrics>> {
  return get('/sip-center/campaigns/metrics')
}

export async function getOutboundCampaignWorkerMetrics(): Promise<ApiResponse<OutboundCampaignWorkerMetrics>> {
  return get('/sip-center/campaigns/worker-metrics')
}

export async function getOutboundCampaignLogs(campaignId: number, limit = 100): Promise<ApiResponse<{ list: OutboundCampaignLogRow[]; total: number }>> {
  const q = new URLSearchParams({ limit: String(limit) })
  return get(`/sip-center/campaigns/${campaignId}/logs?${q.toString()}`)
}
