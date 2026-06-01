import { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Checkbox,
  Drawer,
  Input,
  Select,
  Space,
  Typography,
} from '@arco-design/web-react'
import { IconDelete } from '@arco-design/web-react/icon'
import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import { TableIdCell } from '@/components/TableIdCell'
import {
  createTrunkNumber,
  deleteTrunkNumber,
  fetchTrunksForSelect,
  listTrunkNumbers,
  updateTrunkNumber,
  uploadTrunkNumberTransferRingingAudio,
  uploadTrunkNumberWelcomeAudio,
  type TrunkNumberRow,
  type TrunkRow,
} from '@/api/trunks'
import { listTenants, type TenantRow } from '@/api/tenants'
import { showAlert } from '@/utils/notification'
import { useTranslation } from '@/i18n'
import { EllipsisHoverCell } from '@/pages/ContactCenter/EllipsisHoverCell'
import { FieldLabel } from '@/components/Form/FieldLabel'
import { FieldHint } from '@/components/Form/FieldHint'

const TRUNK_DIRECTION_OPTIONS = [
  { value: '', label: '未设置' },
  { value: 'inbound', label: '呼入 (inbound)' },
  { value: 'outbound', label: '呼出 (outbound)' },
  { value: 'both', label: '双向 (both)' },
  { value: 'all', label: '全部 (all)' },
] as const

const TRUNK_STATUS_OPTIONS = [
  { value: '', label: '未设置' },
  { value: 'active', label: '启用 (active)' },
  { value: 'disabled', label: '停用 (disabled)' },
] as const

const VALID_TRUNK_DIRECTIONS = new Set(['', 'inbound', 'outbound', 'both', 'all'])

/** 与后端 PickTrunkConfig / 外呼选线语义对齐 */
function normalizeDirectionForForm(raw?: string): string {
  const s = (raw || '').trim().toLowerCase()
  if (!s) return ''
  const aliases: Record<string, string> = {
    bidirectional: 'both',
    duplex: 'both',
  }
  const v = aliases[s] || s
  return VALID_TRUNK_DIRECTIONS.has(v) ? v : ''
}

function normalizeDirectionForSave(raw: string): string {
  const s = raw.trim().toLowerCase()
  if (!s) return ''
  const aliases: Record<string, string> = {
    bidirectional: 'both',
    duplex: 'both',
  }
  return aliases[s] || s
}

function toRFC3339OrUndefined(isoLocal: string): string | undefined {
  const s = isoLocal.trim()
  if (!s) return undefined
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return undefined
  return d.toISOString()
}

const MAX_TRANSFER_AGENT_BRIEF_LEN = 256

type FormState = {
  trunkId: string
  tenantId: string
  number: string
  callerDisplayName: string
  prefix: string
  description: string
  direction: string
  status: string
  concurrent: string
  callInConcurrent: string
  isTransferRelay: boolean
  effectiveTime: string
  expirationTime: string
  voiceDialogWsUrl: string
  // welcomeAudioUrl 入局欢迎语 WAV URL（与「上传 WAV」共用同一个存储字段，
  // 上传成功后后端返回的 url 会写回该字段）。
  welcomeAudioUrl: string
  // transferRingingUrl 转接阶段回铃 WAV URL，语义同 welcomeAudioUrl。
  transferRingingUrl: string
  transferAgentBriefText: string
  transferCallerBriefText: string
  outboundTrunkNumberId: string
}

const defaultForm = (): FormState => ({
  trunkId: '',
  tenantId: '0',
  number: '',
  callerDisplayName: '',
  prefix: '',
  description: '',
  direction: '',
  status: '',
  concurrent: '0',
  callInConcurrent: '0',
  isTransferRelay: false,
  effectiveTime: '',
  expirationTime: '',
  voiceDialogWsUrl: '',
  welcomeAudioUrl: '',
  transferRingingUrl: '',
  transferAgentBriefText: '',
  transferCallerBriefText: '',
  outboundTrunkNumberId: '0',
})

const SIPTrunkNumbers = () => {
  const { t } = useTranslation()
  const [trunks, setTrunks] = useState<TrunkRow[]>([])
  const [tenants, setTenants] = useState<TenantRow[]>([])
  const [tenantFilter, setTenantFilter] = useState('')
  const [rows, setRows] = useState<TrunkNumberRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [trunkFilter, setTrunkFilter] = useState('')
  const [numberQ, setNumberQ] = useState('')
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormState>(defaultForm)
  const [saving, setSaving] = useState(false)
  // *Uploading: 仅控制「上传中」按钮 loading 态；上传成功后由 setForm
  // 把后端返回的 url 写回对应字段，与「直接粘贴 URL」共用同一个保存字段。
  const [welcomeUploading, setWelcomeUploading] = useState(false)
  const [ringingUploading, setRingingUploading] = useState(false)
  const [delOpen, setDelOpen] = useState(false)
  const [delId, setDelId] = useState<number | null>(null)
  const [delLoading, setDelLoading] = useState(false)
  const pageSize = 20

  useEffect(() => {
    void (async () => {
      try {
        const list = await fetchTrunksForSelect()
        setTrunks(list)
      } catch {
        setTrunks([])
      }
    })()
  }, [])

  useEffect(() => {
    void (async () => {
      try {
        const res = await listTenants(1, 500)
        if (res.code === 200 && res.data?.list) setTenants(res.data.list)
        else setTenants([])
      } catch {
        setTenants([])
      }
    })()
  }, [])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const tid = trunkFilter ? parseInt(trunkFilter, 10) : 0
      const assignTid = tenantFilter.trim()
      const res = await listTrunkNumbers(page, pageSize, {
        trunkId: Number.isFinite(tid) && tid > 0 ? tid : undefined,
        tenantId: assignTid && assignTid !== '0' ? assignTid : undefined,
        number: numberQ.trim() || undefined,
      })
      if (res.code === 200 && res.data) {
        setRows(res.data.list || [])
        setTotal(res.data.total || 0)
      } else showAlert(res.msg || '加载失败', 'error')
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '加载失败', 'error')
    } finally {
      setLoading(false)
    }
  }, [page, trunkFilter, tenantFilter, numberQ])

  useEffect(() => {
    void load()
  }, [load])

  const trunkLabel = (id: number) => trunks.find((t) => t.id === id)?.name || `线路 #${id}`

  const openCreate = () => {
    setEditingId(null)
    setForm({
      ...defaultForm(),
      trunkId: trunkFilter || (trunks[0]?.id != null ? String(trunks[0].id) : ''),
      tenantId: tenantFilter || '0',
    })
    setModalOpen(true)
  }

  const openEdit = (r: TrunkNumberRow) => {
    setEditingId(r.id)
    const eff = r.effectiveTime ? new Date(r.effectiveTime).toISOString().slice(0, 16) : ''
    const exp = r.expirationTime ? new Date(r.expirationTime).toISOString().slice(0, 16) : ''
    setForm({
      trunkId: String(r.trunkId),
      tenantId: String(r.tenantId ?? 0),
      number: r.number || '',
      callerDisplayName: r.callerDisplayName || '',
      prefix: r.prefix || '',
      description: r.description || '',
      direction: normalizeDirectionForForm(r.direction),
      status: (r.status || '').trim(),
      concurrent: String(r.concurrent ?? 0),
      callInConcurrent: String(r.callInConcurrent ?? 0),
      isTransferRelay: !!r.isTransferRelay,
      effectiveTime: eff,
      expirationTime: exp,
      voiceDialogWsUrl: r.voiceDialogWsUrl || '',
      welcomeAudioUrl: r.welcomeAudioUrl || '',
      transferRingingUrl: r.transferRingingUrl || '',
      transferAgentBriefText: r.transferAgentBriefText || '',
      transferCallerBriefText: r.transferCallerBriefText || '',
      outboundTrunkNumberId: String(r.outboundTrunkNumberId ?? 0),
    })
    setModalOpen(true)
  }

  const closeModal = () => {
    setModalOpen(false)
    setEditingId(null)
  }

  // uploadTrunkAudio 是所有「上传 WAV」按钮的公共逻辑：
  //   1) 前端拦截非 .wav 扩展名 + 大于 16MiB 文件；
  //   2) 调用传入的 uploadFn（后端再做 RIFF/WAVE magic 校验）；
  //   3) 成功后调用 onUploaded(url) 让调用方决定写回哪个字段。
  // 这样 welcome / ringing 两个按钮不用拷贝代码，取裁黑吃不同的 uploadFn 即可。
  const uploadTrunkAudio = async (
    file: File,
    uploadFn: (f: File) => Promise<{ code: number; msg?: string; data?: { url?: string } | null }>,
    setBusy: (b: boolean) => void,
    onUploaded: (url: string) => void,
    successMsg: string,
  ) => {
    const lower = (file.name || '').toLowerCase()
    if (!lower.endsWith('.wav')) {
      showAlert('仅支持 .wav 文件', 'error')
      return
    }
    if (file.size > 16 * 1024 * 1024) {
      showAlert('WAV 文件不能超过 16MiB', 'error')
      return
    }
    setBusy(true)
    try {
      const res = await uploadFn(file)
      if (res.code === 200 && res.data?.url) {
        onUploaded(res.data.url)
        showAlert(successMsg, 'success')
      } else {
        showAlert(res.msg || '上传失败', 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '上传失败', 'error')
    } finally {
      setBusy(false)
    }
  }

  const uploadWelcome = (file: File) =>
    uploadTrunkAudio(
      file,
      uploadTrunkNumberWelcomeAudio,
      setWelcomeUploading,
      (url) => setForm((f) => ({ ...f, welcomeAudioUrl: url })),
      '上传成功，已写入欢迎语 URL',
    )

  const uploadRinging = (file: File) =>
    uploadTrunkAudio(
      file,
      uploadTrunkNumberTransferRingingAudio,
      setRingingUploading,
      (url) => setForm((f) => ({ ...f, transferRingingUrl: url })),
      '上传成功，已写入转接回铃 URL',
    )

  const save = async () => {
    const trunkId = parseInt(form.trunkId, 10)
    const num = form.number.trim()
    if (!Number.isFinite(trunkId) || trunkId <= 0) {
      showAlert('请选择所属线路', 'error')
      return
    }
    if (!num) {
      showAlert('号码不能为空', 'error')
      return
    }
    const direction = normalizeDirectionForSave(form.direction)
    if (direction && !VALID_TRUNK_DIRECTIONS.has(direction)) {
      showAlert('呼叫用途请选择：呼入、呼出、双向或全部', 'error')
      return
    }
    const voiceWs = form.voiceDialogWsUrl.trim()
    if (voiceWs && !/^wss?:\/\//i.test(voiceWs)) {
      showAlert('呼入语音对话 WebSocket 须以 ws:// 或 wss:// 开头', 'error')
      return
    }
    const conc = parseInt(form.concurrent, 10) || 0
    const cin = parseInt(form.callInConcurrent, 10) || 0
    const eff = toRFC3339OrUndefined(form.effectiveTime)
    const exp = toRFC3339OrUndefined(form.expirationTime)
    // Tenant.ID is a Snowflake (>2^53); preserve as opaque string and never parseInt.
    const assignTenant = (form.tenantId || '').trim() || '0'
    const body = {
      trunkId,
      tenantId: assignTenant,
      number: num,
      callerDisplayName: form.callerDisplayName.trim(),
      prefix: form.prefix.trim(),
      description: form.description.trim(),
      direction,
      status: form.status.trim(),
      concurrent: conc,
      callInConcurrent: cin,
      isTransferRelay: form.isTransferRelay,
      effectiveTime: eff ?? null,
      expirationTime: exp ?? null,
      voiceDialogWsUrl: form.voiceDialogWsUrl.trim(),
      welcomeAudioUrl: form.welcomeAudioUrl.trim(),
      transferRingingUrl: form.transferRingingUrl.trim(),
      transferAgentBriefText: form.transferAgentBriefText.trim().slice(0, MAX_TRANSFER_AGENT_BRIEF_LEN),
      transferCallerBriefText: form.transferCallerBriefText.trim().slice(0, MAX_TRANSFER_AGENT_BRIEF_LEN),
      outboundTrunkNumberId: (() => {
        const v = parseInt(form.outboundTrunkNumberId, 10)
        return Number.isFinite(v) && v > 0 ? v : 0
      })(),
    }
    setSaving(true)
    try {
      const res = editingId == null ? await createTrunkNumber(body) : await updateTrunkNumber(editingId, body)
      if (res.code === 200) {
        showAlert('保存成功', 'success')
        closeModal()
        void load()
      } else showAlert(res.msg || '保存失败', 'error')
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '保存失败', 'error')
    } finally {
      setSaving(false)
    }
  }

  const confirmDelete = async () => {
    if (delId == null) return
    setDelLoading(true)
    try {
      const res = await deleteTrunkNumber(delId)
      if (res.code !== 200) {
        showAlert(res.msg || '删除失败', 'error')
        return
      }
      showAlert('删除成功', 'success')
      setDelOpen(false)
      setDelId(null)
      void load()
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '删除失败', 'error')
    } finally {
      setDelLoading(false)
    }
  }

  const trunkOptions = trunks.map((t) => ({ label: t.name || `ID ${t.id}`, value: String(t.id) }))
  const tenantOptions = [
    { label: '待分配（平台池）', value: '0' },
    ...tenants.map((t) => ({ label: `${t.name} (#${t.id})`, value: String(t.id) })),
  ]
  const tenantLabel = (tid?: string) => {
    if (!tid || tid === '0') return '待分配'
    // TenantRow.id is typed as number for legacy reasons but the runtime value is a Snowflake string.
    const hit = tenants.find((x) => String(x.id) === tid)
    return hit ? `${hit.name} (#${tid})` : `租户 #${tid}`
  }

  const directionLabel = (raw?: string) => {
    const s = (raw || '').trim().toLowerCase()
    const map: Record<string, string> = {
      inbound: '呼入',
      outbound: '呼出',
      both: '双向',
      bidirectional: '双向',
      duplex: '双向',
    }
    if (!raw?.trim()) return '—'
    return map[s] || raw.trim()
  }

  return (
    <BaseLayout title={t('pages.sipTrunkNumbers.title')} description={t('pages.sipTrunkNumbers.description')}>
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Typography.Paragraph style={{ margin: 0, fontSize: 12, padding: '10px 12px', background: 'var(--color-fill-2)', borderRadius: 8 }}>
          维护每条中继线路下的外显 / 入局号码；
            <br/>
          通过「分配租户」将号码划拨给某个租户后，该租户才能在号码池绑定坐席（ACD）。生效时间请使用本地时间选择（将按 RFC3339 提交）。
          「呼入语音对话 WebSocket」按每条号码单独配置：留空则走平台默认网关；填写 ws:// 或 wss:// 完整路径后，该局呼入会在媒体建立后向该地址拨号（自动附带 token、call_id 查询参数）。
            <br/>
          <strong style={{ fontWeight: 600 }}>用途</strong>
          ：在下拉框中选择呼入 / 呼出 / 双向 / 全部，与网关外呼、转人工选线规则一致。
            <br/>
          <strong style={{ fontWeight: 600 }}>呼出并发</strong>
          ：允许该号码同时占用的外呼通道数上限（容量规划用，具体是否在网关侧硬限流以实现为准）。
          <strong style={{ fontWeight: 600 }}>呼入并发</strong>
          ：允许同时并发的入局呼叫路数上限（入局容量）
        </Typography.Paragraph>
        <Space wrap align="end">
          <Space direction="vertical" size={4}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>分配租户</Typography.Text>
            <Select
              placeholder="全部"
              allowClear
              style={{ width: 240 }}
              value={tenantFilter === '' ? undefined : tenantFilter}
              onChange={(v) => setTenantFilter((v as string) ?? '')}
              options={tenants.map((t) => ({ label: `${t.name} (#${t.id})`, value: String(t.id) }))}
            />
          </Space>
          <Space direction="vertical" size={4}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>所属线路</Typography.Text>
            <Select
              placeholder="全部线路"
              allowClear
              style={{ width: 220 }}
              value={trunkFilter === '' ? undefined : trunkFilter}
              onChange={(v) => setTrunkFilter((v as string) ?? '')}
              options={trunkOptions}
            />
          </Space>
          <Space direction="vertical" size={4}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>号码</Typography.Text>
            <Input allowClear placeholder="模糊搜索" style={{ width: 180 }} value={numberQ} onChange={setNumberQ} />
          </Space>
          <Button type="primary" onClick={() => { setPage(1); void load() }}>{t('common.search')}</Button>
          <Button type="outline" onClick={openCreate} disabled={!trunks.length}>{t('sipTrunkNumbers.addNumber')}</Button>
        </Space>

        <Card bordered={false} bodyStyle={{ padding: 0 }}>
          {loading ? (
            <div style={{ padding: 24 }}>
              <Typography.Text type="secondary">{t('common.loading')}</Typography.Text>
            </div>
          ) : (
            <>
              <div
                style={{
                  overflowX: 'auto',
                  overflowY: 'hidden',
                  maxWidth: '100%',
                  WebkitOverflowScrolling: 'touch',
                  scrollbarGutter: 'stable',
                }}
              >
                <table style={{ minWidth: 1520, width: '100%', fontSize: 13 }}>
                  <thead style={{ background: 'var(--color-fill-2)' }}>
                    <tr>
                      <th style={{ textAlign: 'left', padding: 12 }}>ID</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>分配租户</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>线路</th>
                      <th style={{ textAlign: 'left', padding: 12, fontFamily: 'monospace', fontSize: 12 }}>号码</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>主叫显示名</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>用途</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>状态</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>呼出并发</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>呼入并发</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>转人工中继</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>语音对话 WS</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>供应商编码</th>
                      <th style={{ textAlign: 'right', padding: 12 }}>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.length === 0 ? (
                      <tr><td colSpan={13} style={{ padding: 24, textAlign: 'center' }}>{t('common.noData')}</td></tr>
                    ) : rows.map((r) => (
                      <tr key={r.id} style={{ borderTop: '1px solid var(--color-border)' }}>
                        <td style={{ padding: 12 }}><TableIdCell id={r.id} /></td>
                        <td style={{ padding: 12, fontSize: 12, maxWidth: 200 }}>
                          <EllipsisHoverCell text={tenantLabel(r.tenantId)} lines={1} />
                        </td>
                        <td style={{ padding: 12, maxWidth: 140 }}>
                          <EllipsisHoverCell text={trunkLabel(r.trunkId)} lines={1} />
                        </td>
                        <td style={{ padding: 12, fontFamily: 'monospace', fontSize: 12, maxWidth: 140 }}>
                          <EllipsisHoverCell text={r.number} lines={1} mono />
                        </td>
                        <td style={{ padding: 12, maxWidth: 160, fontSize: 12 }}>
                          <EllipsisHoverCell text={r.callerDisplayName?.trim() || '—'} lines={1} />
                        </td>
                        <td style={{ padding: 12, fontSize: 12 }}>{directionLabel(r.direction)}</td>
                        <td style={{ padding: 12, fontSize: 12 }}>{r.status || '—'}</td>
                        <td style={{ padding: 12 }}>{r.concurrent ?? '—'}</td>
                        <td style={{ padding: 12 }}>{r.callInConcurrent ?? '—'}</td>
                        <td style={{ padding: 12, fontSize: 12 }}>{r.isTransferRelay ? '是' : '否'}</td>
                        <td style={{ padding: 12, fontSize: 11, maxWidth: 160, color: 'var(--color-text-2)' }}>
                          <EllipsisHoverCell
                            text={(() => {
                              const u = String(r.voiceDialogWsUrl || '').trim()
                              return u || '默认'
                            })()}
                            lines={1}
                            mono
                          />
                        </td>
                        <td style={{ padding: 12, fontFamily: 'monospace', fontSize: 12, maxWidth: 240 }}>
                          <EllipsisHoverCell text={r.providerCode || '—'} lines={1} mono />
                        </td>
                        <td style={{ padding: 12, textAlign: 'right' }}>
                          <Space>
                            <Button type="outline" size="small" onClick={() => openEdit(r)}>{t('common.edit')}</Button>
                            <Button type="outline" status="danger" size="small" icon={<IconDelete />} onClick={() => { setDelId(r.id); setDelOpen(true) }}>{t('common.delete')}</Button>
                          </Space>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 12, padding: '12px 16px 16px', borderTop: '1px solid var(--color-border)' }}>
                <Typography.Text type="secondary">{t('common.total')}: {total}</Typography.Text>
                <Space>
                  <Button size="small" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>{t('common.previous')}</Button>
                  <Button size="small" disabled={page * pageSize >= total} onClick={() => setPage((p) => p + 1)}>{t('common.next')}</Button>
                </Space>
              </div>
            </>
          )}
        </Card>

        <Drawer
          title={editingId == null ? t('sipTrunkNumbers.drawerCreate') : t('sipTrunkNumbers.drawerEdit')}
          visible={modalOpen}
          placement="right"
          width={640}
          onCancel={closeModal}
          footer={
            <Space>
              <Button onClick={closeModal} disabled={saving}>{t('common.cancel')}</Button>
              <Button type="primary" loading={saving} onClick={() => void save()}>
                {saving ? t('common.saving') : t('common.save')}
              </Button>
            </Space>
          }
        >
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            <div>
              <FieldLabel
                label="分配租户"
                hint="选择租户后，该号码仅对该租户可见，用于号码池坐席绑定与外呼选线。"
              />
              <Select
                placeholder="待分配（平台池）"
                value={form.tenantId}
                onChange={(v) => setForm((f) => ({ ...f, tenantId: v ?? '0' }))}
                options={tenantOptions}
              />
            </div>
            <div>
              <FieldLabel label="所属线路" required hint="号码挂载的中继线路；决定出局网关 local_addr 等线路级配置。" />
              <Select
                placeholder="请选择"
                value={form.trunkId || undefined}
                onChange={(v) => setForm((f) => ({ ...f, trunkId: v ?? '' }))}
                options={trunkOptions}
              />
            </div>
            <div>
              <FieldLabel
                label="号码"
                required
                hint="外呼 / 转呼时 INVITE From 的 user 部分（原 SIP_CALLER_ID）。"
              />
              <Input value={form.number} onChange={(v) => setForm((f) => ({ ...f, number: v }))} />
            </div>
            <div>
              <FieldLabel
                label="主叫显示名"
                hint="INVITE From 头的引号显示名（原 SIP_CALLER_DISPLAY_NAME）；留空则 From 仅含号码，无 display-name。"
              />
              <Input
                placeholder="例如 七牛云客服专线（可留空）"
                value={form.callerDisplayName}
                onChange={(v) => setForm((f) => ({ ...f, callerDisplayName: v }))}
              />
            </div>
            <div>
              <FieldLabel
                label="呼入语音对话 WebSocket"
                hint="与本条「号码」入局匹配时生效；留空走平台默认 loopback。填写 ws:// 或 wss:// 完整路径后，网关会自动追加 token、call_id 查询参数。"
              />
              <Input.TextArea
                placeholder="留空=平台默认 loopback；例如 wss://your-host/dialog/ws"
                autoSize={{ minRows: 2, maxRows: 5 }}
                value={form.voiceDialogWsUrl}
                onChange={(v) => setForm((f) => ({ ...f, voiceDialogWsUrl: v }))}
                style={{ fontFamily: 'monospace', fontSize: 12 }}
              />
            </div>
            <div>
              <FieldLabel
                label="入局欢迎语 WAV"
                hint="呼入匹配本号码时播放的欢迎语音频（PCM WAV，建议 16-bit / 8–48 kHz mono）。可粘贴外链 URL 或点「上传 WAV」托管；保存时后端会做可达性 + RIFF/WAVE 校验。留空则回退 SIP_WELCOME_WAV_PATH / scripts/welcome.wav。"
              />
              <Input
                placeholder="留空=回退到 scripts/welcome.wav；可粘贴外链 URL 或点「上传 WAV」"
                value={form.welcomeAudioUrl}
                onChange={(v) => setForm((f) => ({ ...f, welcomeAudioUrl: v }))}
                style={{ fontFamily: 'monospace', fontSize: 12 }}
              />
              <Space size={8} style={{ marginTop: 8 }}>
                <Button
                  size="small"
                  loading={welcomeUploading}
                  onClick={() => {
                    // 隐藏式 <input type="file"> 弹原生选择器；这样 Drawer 内
                    // 不需要再嵌 Arco Upload 组件，避免它的 action/headers 跨域配置。
                    const input = document.createElement('input')
                    input.type = 'file'
                    input.accept = '.wav,audio/wav,audio/x-wav'
                    input.onchange = () => {
                      const f = input.files?.[0]
                      if (f) void uploadWelcome(f)
                    }
                    input.click()
                  }}
                >
                  上传 WAV
                </Button>
                {form.welcomeAudioUrl.trim() && (
                  <Button
                    size="small"
                    type="text"
                    status="danger"
                    onClick={() => setForm((f) => ({ ...f, welcomeAudioUrl: '' }))}
                  >
                    清除
                  </Button>
                )}
              </Space>
              {form.welcomeAudioUrl.trim() && (
                <audio
                  controls
                  preload="none"
                  src={form.welcomeAudioUrl.trim()}
                  style={{ display: 'block', width: '100%', marginTop: 8 }}
                >
                  当前浏览器不支持 audio 元素，请直接复制 URL 在外部播放器试听。
                </audio>
              )}
            </div>
            <div>
              <FieldLabel
                label="转接回铃 WAV"
                hint="转接 / 转人工阶段播放给主叫的回铃 WAV（SIP ringback 与 voicedialog transfer-loading）。校验与上传流程同欢迎语；留空回退 SIP_TRANSFER_RINGING_WAV_PATH / scripts/ringing.wav。"
              />
              <Input
                placeholder="留空=回退到 scripts/ringing.wav；可粘贴外链 URL 或点「上传 WAV」"
                value={form.transferRingingUrl}
                onChange={(v) => setForm((f) => ({ ...f, transferRingingUrl: v }))}
                style={{ fontFamily: 'monospace', fontSize: 12 }}
              />
              <Space size={8} style={{ marginTop: 8 }}>
                <Button
                  size="small"
                  loading={ringingUploading}
                  onClick={() => {
                    const input = document.createElement('input')
                    input.type = 'file'
                    input.accept = '.wav,audio/wav,audio/x-wav'
                    input.onchange = () => {
                      const f = input.files?.[0]
                      if (f) void uploadRinging(f)
                    }
                    input.click()
                  }}
                >
                  上传 WAV
                </Button>
                {form.transferRingingUrl.trim() && (
                  <Button
                    size="small"
                    type="text"
                    status="danger"
                    onClick={() => setForm((f) => ({ ...f, transferRingingUrl: '' }))}
                  >
                    清除
                  </Button>
                )}
              </Space>
              {form.transferRingingUrl.trim() && (
                <audio
                  controls
                  preload="none"
                  src={form.transferRingingUrl.trim()}
                  style={{ display: 'block', width: '100%', marginTop: 8 }}
                >
                  当前浏览器不支持 audio 元素，请直接复制 URL 在外部播放器试听。
                </audio>
              )}
            </div>
            <div>
              <FieldLabel
                label="坐席桥接前播报（可选）"
                hint={
                  <>
                    坐席接通后、与客户通话前，向坐席侧 TTS 播报一句。占位符：{' '}
                    <Typography.Text code>{'{{N}}'}</Typography.Text> 主叫号码、
                    <Typography.Text code>{'{{NTail4}}'}</Typography.Text> 尾号四位、
                    <Typography.Text code>{'{{Name}}'}</Typography.Text> 坐席名称、
                    <Typography.Text code>{'{{TargetValue}}'}</Typography.Text> 呼叫号码、
                    <Typography.Text code>{'{{Note}}'}</Typography.Text> 坐席备注、
                    <Typography.Text code>{'{{MetaData.FactoryNumber}}'}</Typography.Text>{' '}
                    等（号码池坐席 MetaData）。最多 {MAX_TRANSFER_AGENT_BRIEF_LEN} 字。
                  </>
                }
              />
              <Input.TextArea
                maxLength={MAX_TRANSFER_AGENT_BRIEF_LEN}
                showWordLimit
                autoSize={{ minRows: 2, maxRows: 4 }}
                placeholder="例：您好{{Name}}，工厂{{MetaData.FactoryNumber}}，尾号{{NTail4}}的来电，请接听。"
                value={form.transferAgentBriefText}
                onChange={(v) =>
                  setForm((f) => ({
                    ...f,
                    transferAgentBriefText: String(v).slice(0, MAX_TRANSFER_AGENT_BRIEF_LEN),
                  }))
                }
              />
            </div>
            <div>
              <FieldLabel
                label="主叫桥接前播报（可选）"
                hint={
                  <>
                    向主叫侧 TTS 播报一句（转接音乐会先停止）。留空则与上方坐席播报相同、同步播放；
                    填写不同文案则主叫与坐席同时听到各自内容。占位符同上，最多 {MAX_TRANSFER_AGENT_BRIEF_LEN} 字。
                  </>
                }
              />
              <Input.TextArea
                maxLength={MAX_TRANSFER_AGENT_BRIEF_LEN}
                showWordLimit
                autoSize={{ minRows: 2, maxRows: 4 }}
                placeholder="留空=与坐席播报相同；例：正在为您转接客服，请稍候。"
                value={form.transferCallerBriefText}
                onChange={(v) =>
                  setForm((f) => ({
                    ...f,
                    transferCallerBriefText: String(v).slice(0, MAX_TRANSFER_AGENT_BRIEF_LEN),
                  }))
                }
              />
            </div>
            <div>
              <FieldLabel label="前缀" hint="可选；部分网关外呼拨号前缀，按线路/运营商约定填写。" />
              <Input value={form.prefix} onChange={(v) => setForm((f) => ({ ...f, prefix: v }))} />
            </div>
            <div>
              <FieldLabel label="备注" hint="运维备注，仅管理端展示，不参与 SIP 信令。" />
              <Input value={form.description} onChange={(v) => setForm((f) => ({ ...f, description: v }))} />
            </div>
            <Space style={{ width: '100%' }} size={12}>
              <div style={{ flex: 1 }}>
                <FieldLabel
                  label="呼叫用途"
                  hint="与网关选线一致：呼入仅入局；呼出 / 双向 / 全部可外呼；留空时转人工场景可回退匹配。"
                />
                <Select
                  placeholder="请选择"
                  allowClear
                  value={form.direction || undefined}
                  onChange={(v) => setForm((f) => ({ ...f, direction: (v as string) ?? '' }))}
                  options={[...TRUNK_DIRECTION_OPTIONS]}
                />
              </div>
              <div style={{ flex: 1 }}>
                <FieldLabel label="状态" hint="运维标签，可选 active / disabled；留空表示未标注。" />
                <Select
                  placeholder="请选择"
                  allowClear
                  value={form.status || undefined}
                  onChange={(v) => setForm((f) => ({ ...f, status: (v as string) ?? '' }))}
                  options={[...TRUNK_STATUS_OPTIONS]}
                />
              </div>
            </Space>
            <Space style={{ width: '100%' }} size={12}>
              <div style={{ flex: 1 }}>
                <FieldLabel
                  label="呼出并发"
                  hint="允许该号码同时占用的外呼通道数上限（0 常表示不单独限制或未启用）。"
                />
                <Input type="number" value={form.concurrent} onChange={(v) => setForm((f) => ({ ...f, concurrent: v }))} />
              </div>
              <div style={{ flex: 1 }}>
                <FieldLabel label="呼入并发" hint="允许同时并发的入局呼叫路数上限（入局容量）。" />
                <Input type="number" value={form.callInConcurrent} onChange={(v) => setForm((f) => ({ ...f, callInConcurrent: v }))} />
              </div>
            </Space>
            <div className="flex items-center gap-1">
              <Checkbox checked={form.isTransferRelay} onChange={(c) => setForm((f) => ({ ...f, isTransferRelay: !!c }))}>
                转人工中继号码
              </Checkbox>
              <FieldHint content="优先作为转人工 / 转接出局网关选线；与「呼叫用途」配合使用。" />
            </div>
            <div>
              <FieldLabel
                label="外呼号码（可选）"
                hint="当本号码作为呼入 DID 需要转接 / 外呼时，改用同租户下另一条可外呼号码作为出局网关与主叫。候选：direction 为 outbound / both / all。留空则使用本号码自己。"
              />
              <Select
                allowClear
                placeholder="默认使用本号码自己外呼"
                value={form.outboundTrunkNumberId === '0' ? undefined : form.outboundTrunkNumberId}
                onChange={(v) => setForm((f) => ({ ...f, outboundTrunkNumberId: (v as string) ?? '0' }))}
                options={(() => {
                  const tid = (form.tenantId || '').trim()
                  if (!tid || tid === '0') return []
                  return rows
                    .filter((n) => {
                      if (n.id === editingId) return false
                      if (String(n.tenantId ?? '0') !== tid) return false
                      const d = String(n.direction || '').trim().toLowerCase()
                      return d === 'outbound' || d === 'both' || d === 'all'
                    })
                    .map((n) => ({ label: `${n.number} (#${n.id})`, value: String(n.id) }))
                })()}
              />
            </div>
            <Space style={{ width: '100%' }} size={12}>
              <div style={{ flex: 1 }}>
                <FieldLabel
                  label="生效时间（可选）"
                  hint="号码开始生效的本地时间，保存时按 RFC3339 提交；留空表示不限制。"
                />
                <Input type="datetime-local" value={form.effectiveTime} onChange={(v) => setForm((f) => ({ ...f, effectiveTime: v }))} />
              </div>
              <div style={{ flex: 1 }}>
                <FieldLabel
                  label="失效时间（可选）"
                  hint="号码失效的本地时间，保存时按 RFC3339 提交；留空表示长期有效。"
                />
                <Input type="datetime-local" value={form.expirationTime} onChange={(v) => setForm((f) => ({ ...f, expirationTime: v }))} />
              </div>
            </Space>
            <div className="flex items-center flex-wrap gap-1">
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                供应商编码由系统自动分配，创建后请在列表中查看。
              </Typography.Text>
              <FieldHint content="providerCode 在创建时由后端生成，全局唯一，不可在表单中修改。" />
            </div>
          </Space>
        </Drawer>

        <Drawer
          title={t('sipTrunkNumbers.deleteTitle')}
          visible={delOpen}
          placement="right"
          width={420}
          onCancel={() => { if (!delLoading) { setDelOpen(false); setDelId(null) } }}
          footer={
            <Space>
              <Button onClick={() => { if (!delLoading) { setDelOpen(false); setDelId(null) } }} disabled={delLoading}>
                {t('common.cancel')}
              </Button>
              <Button status="danger" loading={delLoading} onClick={() => void confirmDelete()}>
                {t('common.confirmDelete')}
              </Button>
            </Space>
          }
        >
          <Typography.Text>{t('sipTrunkNumbers.deleteBody')}</Typography.Text>
        </Drawer>
      </Space>
    </BaseLayout>
  )
}

export default SIPTrunkNumbers
