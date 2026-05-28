import { get, post, type ApiResponse } from '@/utils/request'

export type VoiceProvider = 'xunfei' | 'volcengine'

export interface VoiceCloneCapabilities {
  xunfei: { configured: boolean; provider: string; label: string }
  volcengine: { configured: boolean; provider: string; label: string }
}

export interface VoiceCloneRow {
  id: number
  tenantId: number
  createdBy: number
  trainingTaskId?: number
  provider: string
  assetId: string
  trainVid?: string
  voiceName: string
  voiceDescription?: string
  isActive: boolean
  usageCount: number
  lastUsedAt?: string
  createdAt?: string
  updatedAt?: string
}

export interface VoiceTrainingTaskRow {
  id: number
  tenantId: number
  taskId: string
  provider?: string
  taskName: string
  sex: number
  ageGroup: number
  language: string
  status: number
  textId: number
  textSegId?: number
  assetId?: string
  trainVid?: string
  failedReason?: string
  createdAt?: string
  updatedAt?: string
}

export interface VoiceTrainingTextSegment {
  id: number
  textId: number
  segId: string
  segText: string
}

export interface VoiceTrainingTextRow {
  id: number
  textId: number
  textName: string
  language: string
  textSegments?: VoiceTrainingTextSegment[]
}

export interface VoiceSynthesisRow {
  id: number
  voiceCloneId: number
  text: string
  language: string
  audioUrl: string
  status: string
  createdAt?: string
  provider?: string
}

export interface VolcQueryTaskResult {
  speakerId: string
  status: number
  trainVid?: string
  assetId?: string
  failedDesc?: string
}

export async function getVoiceCloneCapabilities(): Promise<ApiResponse<VoiceCloneCapabilities>> {
  return get('/voice/capabilities')
}

export async function listVoiceClones(provider?: string): Promise<ApiResponse<VoiceCloneRow[]>> {
  const q = provider ? `?provider=${encodeURIComponent(provider)}` : ''
  return get(`/voice/clones${q}`)
}

export async function getVoiceClone(id: number): Promise<ApiResponse<VoiceCloneRow>> {
  return get(`/voice/clones/${id}`)
}

export async function createXunfeiTrainingTask(body: {
  taskName: string
  sex?: number
  ageGroup?: number
  language?: string
}): Promise<ApiResponse<VoiceTrainingTaskRow>> {
  return post('/voice/xunfei/training/create', body)
}

export async function queryXunfeiTrainingTask(taskId: string): Promise<ApiResponse<VoiceTrainingTaskRow>> {
  return post('/voice/xunfei/training/query', { taskId })
}

export async function submitXunfeiTrainingAudio(
  taskId: string,
  textSegId: number,
  audio: File,
): Promise<ApiResponse<null>> {
  const fd = new FormData()
  fd.append('taskId', taskId)
  fd.append('textSegId', String(textSegId))
  fd.append('audio', audio)
  return post('/voice/xunfei/training/submit-audio', fd)
}

export async function submitVolcengineTrainingAudio(
  speakerId: string,
  language: string,
  audio: File,
  taskName?: string,
): Promise<ApiResponse<{ speakerId: string; message: string }>> {
  const fd = new FormData()
  fd.append('speakerId', speakerId)
  fd.append('language', language)
  if (taskName) fd.append('taskName', taskName)
  fd.append('audio', audio)
  return post('/voice/volcengine/task/submit-audio', fd)
}

export async function queryVolcengineTrainingTask(
  speakerId: string,
  taskName?: string,
): Promise<ApiResponse<VolcQueryTaskResult>> {
  return post('/voice/volcengine/task/query', { speakerId, taskName })
}

export async function getVoiceTrainingTexts(textId = 5001): Promise<ApiResponse<VoiceTrainingTextRow>> {
  return get(`/voice/training-texts?textId=${textId}`)
}

export async function updateVoiceClone(body: {
  id: number
  voiceName: string
  voiceDescription?: string
}): Promise<ApiResponse<null>> {
  return post('/voice/clones/update', body)
}

export async function deleteVoiceClone(id: number): Promise<ApiResponse<null>> {
  return post('/voice/clones/delete', { id })
}

export async function synthesizeWithVoiceClone(body: {
  voiceCloneId: number
  text: string
  language?: string
}): Promise<ApiResponse<VoiceSynthesisRow>> {
  return post('/voice/synthesize', body)
}

export async function listVoiceSynthesisHistory(limit = 20, provider?: string): Promise<ApiResponse<VoiceSynthesisRow[]>> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (provider) params.set('provider', provider)
  return get(`/voice/synthesis/history?${params.toString()}`)
}

export async function deleteVoiceSynthesisRecord(id: number): Promise<ApiResponse<null>> {
  return post('/voice/synthesis/delete', { id })
}

/** @deprecated use createXunfeiTrainingTask */
export const createVoiceTrainingTask = createXunfeiTrainingTask
/** @deprecated use queryXunfeiTrainingTask */
export const queryVoiceTrainingTask = queryXunfeiTrainingTask
/** @deprecated use submitXunfeiTrainingAudio */
export const submitVoiceTrainingAudio = submitXunfeiTrainingAudio
