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

function toRFC3339OrUndefined(isoLocal: string): string | undefined {
  const s = isoLocal.trim()
  if (!s) return undefined
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return undefined
  return d.toISOString()
}

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
  outboundTrunkNumberId: '0',
})

const SIPTrunkNumbers = () => {
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
      direction: r.direction || '',
      status: r.status || '',
      concurrent: String(r.concurrent ?? 0),
      callInConcurrent: String(r.callInConcurrent ?? 0),
      isTransferRelay: !!r.isTransferRelay,
      effectiveTime: eff,
      expirationTime: exp,
      voiceDialogWsUrl: r.voiceDialogWsUrl || '',
      welcomeAudioUrl: r.welcomeAudioUrl || '',
      transferRingingUrl: r.transferRingingUrl || '',
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
      direction: form.direction.trim(),
      status: form.status.trim(),
      concurrent: conc,
      callInConcurrent: cin,
      isTransferRelay: form.isTransferRelay,
      effectiveTime: eff ?? null,
      expirationTime: exp ?? null,
      voiceDialogWsUrl: form.voiceDialogWsUrl.trim(),
      welcomeAudioUrl: form.welcomeAudioUrl.trim(),
      transferRingingUrl: form.transferRingingUrl.trim(),
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
    <BaseLayout title="中继号码" description="云联络中心 / 中继号码">
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Typography.Paragraph style={{ margin: 0, fontSize: 12, padding: '10px 12px', background: 'var(--color-fill-2)', borderRadius: 8 }}>
          维护每条中继线路下的外显 / 入局号码；「号码」对应外呼主叫 user（原 SIP_CALLER_ID），「主叫显示名」对应 From 头引号显示名（原 SIP_CALLER_DISPLAY_NAME，可留空）。
          通过「分配租户」将号码划拨给某个租户后，该租户才能在号码池绑定坐席（ACD）。生效时间请使用本地时间选择（将按 RFC3339 提交）。
          「呼入语音对话 WebSocket」按每条号码单独配置：留空则走平台默认网关；填写 ws:// 或 wss:// 完整路径后，该局呼入会在媒体建立后向该地址拨号（自动附带 token、call_id 查询参数）。
          <strong style={{ fontWeight: 600 }}>用途</strong>
          （呼叫用途字段）：号码的业务标签 / 呼叫方向约定，常见填{' '}
          <Typography.Text code style={{ fontSize: 11 }}>inbound</Typography.Text>（侧重入局）、
          <Typography.Text code style={{ fontSize: 11 }}>outbound</Typography.Text>（侧重外呼）、
          <Typography.Text code style={{ fontSize: 11 }}>both</Typography.Text>
          （双向），也可填自定义说明便于运维区分。
          <strong style={{ fontWeight: 600 }}>呼出并发</strong>
          ：允许该号码同时占用的外呼通道数上限（容量规划用，具体是否在网关侧硬限流以实现为准）。
          <strong style={{ fontWeight: 600 }}>呼入并发</strong>
          ：允许同时并发的入局呼叫路数上限（入局容量）。下方表格较宽时可在区域内<strong style={{ fontWeight: 600 }}>左右滑动</strong>
          查看全部列。
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
          <Button type="primary" onClick={() => { setPage(1); void load() }}>搜索</Button>
          <Button type="outline" onClick={openCreate} disabled={!trunks.length}>新增号码</Button>
        </Space>

        <Card bordered={false} bodyStyle={{ padding: 0 }}>
          {loading ? (
            <div style={{ padding: 24 }}>
              <Typography.Text type="secondary">加载中...</Typography.Text>
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
                      <tr><td colSpan={13} style={{ padding: 24, textAlign: 'center' }}>暂无数据</td></tr>
                    ) : rows.map((r) => (
                      <tr key={r.id} style={{ borderTop: '1px solid var(--color-border)' }}>
                        <td style={{ padding: 12 }}>{r.id}</td>
                        <td style={{ padding: 12, fontSize: 12, maxWidth: 200 }}>{tenantLabel(r.tenantId)}</td>
                        <td style={{ padding: 12, maxWidth: 140 }}>{trunkLabel(r.trunkId)}</td>
                        <td style={{ padding: 12, fontFamily: 'monospace', fontSize: 12 }}>{r.number}</td>
                        <td style={{ padding: 12, maxWidth: 160, wordBreak: 'break-all', fontSize: 12 }}>{r.callerDisplayName?.trim() || '—'}</td>
                        <td style={{ padding: 12, fontSize: 12 }}>{directionLabel(r.direction)}</td>
                        <td style={{ padding: 12, fontSize: 12 }}>{r.status || '—'}</td>
                        <td style={{ padding: 12 }}>{r.concurrent ?? '—'}</td>
                        <td style={{ padding: 12 }}>{r.callInConcurrent ?? '—'}</td>
                        <td style={{ padding: 12, fontSize: 12 }}>{r.isTransferRelay ? '是' : '否'}</td>
                        <td style={{ padding: 12, fontSize: 11, wordBreak: 'break-all', maxWidth: 160, color: 'var(--color-text-2)' }}>
                          {(() => {
                            const u = String(r.voiceDialogWsUrl || '').trim()
                            if (!u) return '默认'
                            return u.length > 36 ? `${u.slice(0, 34)}…` : u
                          })()}
                        </td>
                        <td style={{ padding: 12, fontFamily: 'monospace', fontSize: 12, wordBreak: 'break-all', maxWidth: 240 }}>{r.providerCode || '—'}</td>
                        <td style={{ padding: 12, textAlign: 'right' }}>
                          <Space>
                            <Button type="outline" size="small" onClick={() => openEdit(r)}>编辑</Button>
                            <Button type="outline" status="danger" size="small" icon={<IconDelete />} onClick={() => { setDelId(r.id); setDelOpen(true) }}>删除</Button>
                          </Space>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 12, padding: '12px 16px 16px', borderTop: '1px solid var(--color-border)' }}>
                <Typography.Text type="secondary">总计: {total}</Typography.Text>
                <Space>
                  <Button size="small" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>上一页</Button>
                  <Button size="small" disabled={page * pageSize >= total} onClick={() => setPage((p) => p + 1)}>下一页</Button>
                </Space>
              </div>
            </>
          )}
        </Card>

        <Drawer
          title={editingId == null ? '新增中继号码' : '编辑中继号码'}
          visible={modalOpen}
          placement="right"
          width={640}
          onCancel={closeModal}
          footer={
            <Space>
              <Button onClick={closeModal} disabled={saving}>取消</Button>
              <Button type="primary" loading={saving} onClick={() => void save()}>
                {saving ? '保存中...' : '保存'}
              </Button>
            </Space>
          }
        >
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>分配租户</Typography.Text>
              <Select
                placeholder="待分配（平台池）"
                value={form.tenantId}
                onChange={(v) => setForm((f) => ({ ...f, tenantId: v ?? '0' }))}
                options={tenantOptions}
              />
              <Typography.Paragraph type="secondary" style={{ margin: '4px 0 0', fontSize: 12 }}>
                选择租户后，该号码仅对该租户可见，用于号码池坐席绑定与外呼选线。
              </Typography.Paragraph>
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>所属线路 *</Typography.Text>
              <Select
                placeholder="请选择"
                value={form.trunkId || undefined}
                onChange={(v) => setForm((f) => ({ ...f, trunkId: v ?? '' }))}
                options={trunkOptions}
              />
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>号码 *</Typography.Text>
              <Input value={form.number} onChange={(v) => setForm((f) => ({ ...f, number: v }))} />
              <Typography.Paragraph type="secondary" style={{ margin: '4px 0 0', fontSize: 12 }}>
                外呼 / 转呼时 INVITE From 的 user 部分（原 SIP_CALLER_ID）。
              </Typography.Paragraph>
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>主叫显示名</Typography.Text>
              <Input
                placeholder="例如 七牛云客服专线（可留空）"
                value={form.callerDisplayName}
                onChange={(v) => setForm((f) => ({ ...f, callerDisplayName: v }))}
              />
              <Typography.Paragraph type="secondary" style={{ margin: '4px 0 0', fontSize: 12 }}>
                INVITE From 头的引号显示名（原 SIP_CALLER_DISPLAY_NAME）；留空则 From 仅含号码，无 display-name。
              </Typography.Paragraph>
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>呼入语音对话 WebSocket</Typography.Text>
              <Input.TextArea
                placeholder="留空=平台默认 loopback；例如 wss://your-host/dialog/ws"
                autoSize={{ minRows: 2, maxRows: 5 }}
                value={form.voiceDialogWsUrl}
                onChange={(v) => setForm((f) => ({ ...f, voiceDialogWsUrl: v }))}
                style={{ fontFamily: 'monospace', fontSize: 12 }}
              />
              <Typography.Paragraph type="secondary" style={{ margin: '4px 0 0', fontSize: 12 }}>
                与本条「号码」入局匹配时生效；网关会自动追加 token、call_id 查询参数。
              </Typography.Paragraph>
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>入局欢迎语 WAV</Typography.Text>
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
              <Typography.Paragraph type="secondary" style={{ margin: '4px 0 0', fontSize: 12 }}>
                呼入匹配本号码时播放的欢迎语音频（PCM WAV，建议 16-bit / 8-48 kHz mono）。可直接粘贴外链 URL，或点「上传 WAV」交由平台托管；保存时后端会做可达性 + RIFF/WAVE magic 双重校验。留空则回退到 SIP_WELCOME_WAV_PATH env / scripts/welcome.wav；都不存在则跳过欢迎语阶段。
              </Typography.Paragraph>
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>转接回铃 WAV</Typography.Text>
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
              <Typography.Paragraph type="secondary" style={{ margin: '4px 0 0', fontSize: 12 }}>
                转接/转人工阶段（SIP 透传 ringback 与 voicedialog transfer-loading）播放给主叫的回铃 WAV。校验规则与上传流程同欢迎语完全一致；留空则回退到 SIP_TRANSFER_RINGING_WAV_PATH env / scripts/ringing.wav。
              </Typography.Paragraph>
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>前缀</Typography.Text>
              <Input value={form.prefix} onChange={(v) => setForm((f) => ({ ...f, prefix: v }))} />
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>备注</Typography.Text>
              <Input value={form.description} onChange={(v) => setForm((f) => ({ ...f, description: v }))} />
            </div>
            <Space style={{ width: '100%' }} size={12}>
              <div style={{ flex: 1 }}>
                <Typography.Text style={{ fontSize: 12 }}>呼叫用途</Typography.Text>
                <Input
                  placeholder="如 inbound / outbound / both"
                  value={form.direction}
                  onChange={(v) => setForm((f) => ({ ...f, direction: v }))}
                />
                <Typography.Paragraph type="secondary" style={{ margin: '4px 0 0', fontSize: 12 }}>
                  号码用途或呼叫方向标签；列表中会翻译成常见中文（未知值则原样显示）。
                </Typography.Paragraph>
              </div>
              <div style={{ flex: 1 }}>
                <Typography.Text style={{ fontSize: 12 }}>状态</Typography.Text>
                <Input value={form.status} onChange={(v) => setForm((f) => ({ ...f, status: v }))} />
              </div>
            </Space>
            <Space style={{ width: '100%' }} size={12}>
              <div style={{ flex: 1 }}>
                <Typography.Text style={{ fontSize: 12 }}>呼出并发</Typography.Text>
                <Input type="number" value={form.concurrent} onChange={(v) => setForm((f) => ({ ...f, concurrent: v }))} />
                <Typography.Paragraph type="secondary" style={{ margin: '4px 0 0', fontSize: 12 }}>
                  同时外呼路数上限（0 常表示不单独限制或未启用）。
                </Typography.Paragraph>
              </div>
              <div style={{ flex: 1 }}>
                <Typography.Text style={{ fontSize: 12 }}>呼入并发</Typography.Text>
                <Input type="number" value={form.callInConcurrent} onChange={(v) => setForm((f) => ({ ...f, callInConcurrent: v }))} />
                <Typography.Paragraph type="secondary" style={{ margin: '4px 0 0', fontSize: 12 }}>
                  同时入局呼叫路数上限。
                </Typography.Paragraph>
              </div>
            </Space>
            <Checkbox checked={form.isTransferRelay} onChange={(c) => setForm((f) => ({ ...f, isTransferRelay: !!c }))}>转人工中继号码</Checkbox>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>外呼号码（可选）</Typography.Text>
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
              <Typography.Paragraph type="secondary" style={{ margin: '4px 0 0', fontSize: 12 }}>
                当本号码作为「呼入 DID」需要转接/外呼时,改用这条号码作为出局网关+主叫。候选范围:同租户、direction ∈ outbound/both/all。留空=用本号码自己。
              </Typography.Paragraph>
            </div>
            <Space style={{ width: '100%' }} size={12}>
              <div style={{ flex: 1 }}>
                <Typography.Text style={{ fontSize: 12 }}>生效时间（可选）</Typography.Text>
                <Input type="datetime-local" value={form.effectiveTime} onChange={(v) => setForm((f) => ({ ...f, effectiveTime: v }))} />
              </div>
              <div style={{ flex: 1 }}>
                <Typography.Text style={{ fontSize: 12 }}>失效时间（可选）</Typography.Text>
                <Input type="datetime-local" value={form.expirationTime} onChange={(v) => setForm((f) => ({ ...f, expirationTime: v }))} />
              </div>
            </Space>
            <Typography.Paragraph type="secondary" style={{ margin: 0, fontSize: 12 }}>
              供应商编码由系统自动分配，全局唯一，创建后请在列表中查看。
            </Typography.Paragraph>
          </Space>
        </Drawer>

        <Drawer
          title="确认删除中继号码"
          visible={delOpen}
          placement="right"
          width={420}
          onCancel={() => { if (!delLoading) { setDelOpen(false); setDelId(null) } }}
          footer={
            <Space>
              <Button onClick={() => { if (!delLoading) { setDelOpen(false); setDelId(null) } }} disabled={delLoading}>
                取消
              </Button>
              <Button status="danger" loading={delLoading} onClick={() => void confirmDelete()}>
                确认删除
              </Button>
            </Space>
          }
        >
          <Typography.Text>删除后不可恢复（软删除），确认继续吗？</Typography.Text>
        </Drawer>
      </Space>
    </BaseLayout>
  )
}

export default SIPTrunkNumbers
