import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Button,
  Drawer,
  Input,
  Select,
  Space,
  Tag,
  Typography,
} from '@arco-design/web-react'
import { IconDelete, IconDragDotVertical, IconPhone } from '@arco-design/web-react/icon'
import { ShiftScheduleModal } from '@/components/ACD/ShiftScheduleModal'
import {
  MetaDataKeyValueEditor,
  metaDataJSONFromPairs,
  metaDataPairsFromJSON,
  validateMetaDataPairs,
  type MetaDataPair,
} from '@/components/ACD/MetaDataKeyValueEditor'
import { showAlert } from '@/utils/notification'
import { shiftScheduleSummary } from '@/utils/shiftSchedule'
import {
  ACD_ROUTE_TYPES,
  ACD_WORK_STATES,
  createACDPoolTarget,
  deleteACDPoolTarget,
  listACDPoolTargets,
  reorderACDPoolTargets,
  updateACDPoolTarget,
  type ACDPoolTargetRow,
} from '@/api/acdPool'
import { seatIdKey, useSIPAgentIncomingPoll } from '@/hooks/useSIPAgentIncomingPoll'
import { callerDisplay, SIP_INCOMING_POLL_MS } from '@/utils/sipAgentIncoming'
import { listTrunkNumbers } from '@/api/trunks'
import './ACDPoolTab.css'

type FormState = {
  trunkNumberId: number
  name: string
  routeType: string
  targetValue: string
  workState: string
  shiftSchedule: string
  remark: string
  metaDataPairs: MetaDataPair[]
}
const defaultForm = (): FormState => ({
  trunkNumberId: 0,
  name: '',
  routeType: 'sip',
  targetValue: '',
  workState: 'offline',
  shiftSchedule: '',
  remark: '',
  metaDataPairs: [],
})

export default function ACDPoolTab({ active, refreshNonce = 0 }: { active: boolean; refreshNonce?: number }) {
  const [rows, setRows] = useState<ACDPoolTargetRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [routeTypeFilter, setRouteTypeFilter] = useState('')
  const [trunkNumFilter, setTrunkNumFilter] = useState<number | undefined>(undefined)
  const [trunkNumOpts, setTrunkNumOpts] = useState<{ label: string; value: number }[]>([])
  const [loading, setLoading] = useState(false)
  const [modalOpen, setModalOpen] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [form, setForm] = useState<FormState>(defaultForm)
  const [saving, setSaving] = useState(false)
  const [acdDeleteOpen, setAcdDeleteOpen] = useState(false)
  const [acdDeleteId, setAcdDeleteId] = useState<number | null>(null)
  const [acdDeleteLoading, setAcdDeleteLoading] = useState(false)
  const [shiftModalOpen, setShiftModalOpen] = useState(false)
  const [dragIdx, setDragIdx] = useState<number | null>(null)
  const [dropOverIdx, setDropOverIdx] = useState<number | null>(null)
  const [reordering, setReordering] = useState(false)
  const sipRowsOnPage = useMemo(() => rows.filter((r) => r.routeType === 'sip'), [rows])
  const { incomingBySeatId, incomingSeats } = useSIPAgentIncomingPoll(sipRowsOnPage, active)
  const listPageSize = trunkNumFilter != null && trunkNumFilter > 0 ? 500 : 20
  const dragEnabled = rows.length > 1 && !reordering

  const load = useCallback(async () => {
    if (!active) return
    setLoading(true)
    try {
      const res = await listACDPoolTargets(page, listPageSize, {
        routeType: routeTypeFilter.trim() || undefined,
        trunkNumberId: trunkNumFilter,
      })
      if (res.code === 200 && res.data) {
        setRows(res.data.list || [])
        setTotal(res.data.total || 0)
      }
    } catch (e: unknown) {
      showAlert((e as { msg?: string })?.msg || '加载失败', 'error')
    } finally {
      setLoading(false)
    }
  }, [active, page, listPageSize, routeTypeFilter, trunkNumFilter])

  useEffect(() => {
    setPage(1)
  }, [trunkNumFilter, routeTypeFilter])

  useEffect(() => {
    void load()
  }, [load, refreshNonce])

  const reloadTrunkNums = useCallback(async () => {
    try {
      const res = await listTrunkNumbers(1, 500)
      if (res.code === 200 && res.data?.list?.length) {
        setTrunkNumOpts(
          res.data.list.map((n) => ({
            label: `${n.number} (#${n.id})`,
            value: n.id,
          })),
        )
      } else {
        setTrunkNumOpts([])
      }
    } catch {
      setTrunkNumOpts([])
    }
  }, [])

  useEffect(() => {
    if (!active) return
    void reloadTrunkNums()
  }, [active, reloadTrunkNums])

  const incomingSipRows = incomingSeats

  const trunkNumLabel = (id?: number) => {
    if (!id) return '—'
    const hit = trunkNumOpts.find((o) => o.value === id)
    return hit?.label || `#${id}`
  }

  const openCreate = () => {
    setEditingId(null)
    const first = trunkNumOpts[0]?.value ?? 0
    setForm({ ...defaultForm(), trunkNumberId: first })
    setModalOpen(true)
  }
  const openEdit = (r: ACDPoolTargetRow) => {
    // @ts-ignore
    setEditingId(r.id)
    setForm({
      trunkNumberId: r.trunkNumberId ?? 0,
      name: r.name || '',
      routeType: r.routeType || 'sip',
      targetValue: r.targetValue || '',
      workState: r.workState || 'offline',
      shiftSchedule: r.shiftSchedule ?? '',
      remark: r.remark || '',
      metaDataPairs: metaDataPairsFromJSON(r.metaData),
    })
    setModalOpen(true)
  }
  const closeModal = () => {
    setModalOpen(false)
    setEditingId(null)
  }

  const save = async () => {
    setSaving(true)
    try {
      if (!form.trunkNumberId || form.trunkNumberId <= 0) {
        showAlert('请选择中继号码后再绑定坐席', 'error')
        return
      }
      const routeType = editingId == null ? 'sip' : form.routeType
      const tv = routeType === 'sip' ? form.targetValue.trim() : ''
      if (routeType === 'sip' && !tv) {
        showAlert('SIP 目标不能为空', 'error')
        return
      }
      const shiftTrim = form.shiftSchedule.trim()
      const metaErr = validateMetaDataPairs(form.metaDataPairs)
      if (metaErr) {
        showAlert(metaErr, 'error')
        return
      }
      const metaData = metaDataJSONFromPairs(form.metaDataPairs)
      if (form.remark.trim().length > 128) {
        showAlert('备注最多 128 个字符', 'error')
        return
      }
      const body = {
        name: form.name.trim(),
        trunkNumberId: form.trunkNumberId,
        routeType,
        sipSource: '',
        targetValue: tv,
        sipTrunkHost: '',
        sipTrunkPort: 0,
        sipTrunkSignalingAddr: '',
        sipCallerId: '',
        sipCallerDisplayName: '',
        weight: 10,
        workState: form.workState,
        shiftSchedule: shiftTrim,
        remark: form.remark.trim(),
        metaData,
      }
      const res = editingId == null ? await createACDPoolTarget(body) : await updateACDPoolTarget(editingId, body)
      if (res.code === 200) {
        showAlert('保存成功', 'success')
        closeModal()
        void load()
      } else showAlert(res.msg || '保存失败', 'error')
    } catch (e: unknown) {
      showAlert((e as { msg?: string })?.msg || '保存失败', 'error')
    } finally {
      setSaving(false)
    }
  }

  const confirmAcdDelete = async () => {
    if (acdDeleteId == null) return
    setAcdDeleteLoading(true)
    try {
      const res = await deleteACDPoolTarget(acdDeleteId)
      if (res.code !== 200) {
        showAlert(res.msg || '删除失败', 'error')
        return
      }
      showAlert('删除成功', 'success')
      setAcdDeleteOpen(false)
      setAcdDeleteId(null)
      void load()
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '删除失败', 'error')
    } finally {
      setAcdDeleteLoading(false)
    }
  }

  const workStateLabel = (s: string) =>
    ({ offline: '离线', available: '可用', ringing: '振铃中', busy: '忙碌', acw: '话后整理', break: '休息' } as Record<string, string>)[s] || s

  const persistOrder = async (ordered: ACDPoolTargetRow[], trunkId: number) => {
    if (!trunkId || trunkId <= 0) {
      showAlert('该坐席未绑定中继号码，无法排序', 'warning')
      return
    }
    setReordering(true)
    try {
      let ids = ordered.filter((r) => r.trunkNumberId === trunkId).map((r) => r.id)
      if (trunkNumFilter !== trunkId) {
        const full = await listACDPoolTargets(1, 500, { trunkNumberId: trunkId })
        const all = full.data?.list ?? []
        const visSet = new Set(ids.map((id) => String(id)))
        const rest = all.filter((r) => !visSet.has(String(r.id))).map((r) => r.id)
        ids = [...ids, ...rest]
      }
      const res = await reorderACDPoolTargets(trunkId, ids)
      if (res.code === 200) {
        showAlert('优先级已更新', 'success')
      } else {
        showAlert(res.msg || '排序保存失败', 'error')
        void load()
      }
    } catch (e: unknown) {
      showAlert((e as { msg?: string })?.msg || '排序保存失败', 'error')
      void load()
    } finally {
      setReordering(false)
    }
  }

  const handleDrop = (toIdx: number) => {
    if (dragIdx == null || dragIdx === toIdx || !dragEnabled) return
    const from = rows[dragIdx]
    const to = rows[toIdx]
    if (!from || !to) return
    const trunkId = from.trunkNumberId ?? 0
    if (trunkId <= 0) {
      showAlert('该坐席未绑定中继号码，无法排序', 'warning')
      clearDragState()
      return
    }
    if ((to.trunkNumberId ?? 0) !== trunkId) {
      showAlert('只能在同一中继号码下调整优先级', 'warning')
      clearDragState()
      return
    }
    const next = [...rows]
    const [moved] = next.splice(dragIdx, 1)
    if (!moved) return
    next.splice(toIdx, 0, moved)
    setRows(next)
    setDragIdx(null)
    setDropOverIdx(null)
    void persistOrder(next, trunkId)
  }

  const clearDragState = () => {
    setDragIdx(null)
    setDropOverIdx(null)
  }

  const rowDragClass = (idx: number) => {
    if (dragIdx === idx) return ' acd-pool-row--dragging'
    if (dropOverIdx === idx && dragIdx != null && dragIdx !== idx) return ' acd-pool-row--drop-over'
    return ''
  }

  return (
    <div className="mt-4 space-y-3">
      <Typography.Paragraph style={{ margin: 0, fontSize: 12 }} className="rounded-lg border px-3 py-2.5">
        每条坐席目标必须绑定本平台已分配给当前租户的中继号码（被叫号码）；来电命中该号码时优先路由到对应坐席。
        SIP 坐席每 {SIP_INCOMING_POLL_MS / 1000} 秒自动查询是否有转接振铃（不含 Web 坐席）。
        拖动最左侧手柄调整转接优先级（越靠上越优先）；同一中继号码下的坐席才能互相排序。
      </Typography.Paragraph>
      {incomingSipRows.length > 0 && (
        <div className="rounded-lg border border-orange-500/40 bg-orange-500/10 px-3 py-2.5 text-sm">
          <Space wrap>
            <Tag color="orangered" icon={<IconPhone />}>转接振铃</Tag>
            {incomingSipRows.map((r) => {
              const inc = incomingBySeatId[seatIdKey(r.id)]
              return (
                <span key={seatIdKey(r.id)}>
                  <Typography.Text bold>{r.name || r.targetValue || '坐席'}</Typography.Text>
                  <Typography.Text type="secondary"> · 主叫 {callerDisplay(inc)}</Typography.Text>
                </span>
              )
            })}
          </Space>
        </div>
      )}
      <Space wrap align="end">
        <Space direction="vertical" size={4}>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>中继号码</Typography.Text>
          <Select
            style={{ width: 220 }}
            placeholder="全部号码"
            allowClear
            value={trunkNumFilter}
            onChange={(v) => {
              setTrunkNumFilter((v as number | undefined) ?? undefined)
            }}
            options={trunkNumOpts}
          />
        </Space>
        <Space direction="vertical" size={4}>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>路由类型</Typography.Text>
          <Select
            style={{ width: 112 }}
            value={routeTypeFilter === '' ? undefined : routeTypeFilter}
            placeholder="全部"
            allowClear
            onChange={(v) => setRouteTypeFilter((v as string) ?? '')}
            options={ACD_ROUTE_TYPES.map((rt) => ({ label: rt, value: rt }))}
          />
        </Space>
        <Button type="primary" size="small" onClick={() => { setPage(1); void load() }}>搜索</Button>
        <Button type="outline" size="small" onClick={openCreate}>新增 SIP 目标</Button>
      </Space>

      {loading ? (
        <div className="p-4 text-sm text-muted-foreground">加载中...</div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border bg-card">
          <table className="acd-pool-table min-w-[860px] w-full text-sm">
            <thead className="bg-muted/50">
              <tr>
                <th className="text-left p-3 w-16 whitespace-nowrap">排序</th>
                <th className="text-left p-3 whitespace-nowrap">中继号码</th>
                <th className="text-left p-3 whitespace-nowrap">名称</th>
                <th className="text-left p-3 min-w-[180px]">呼叫号码</th>
                <th className="text-left p-3 whitespace-nowrap">班次</th>
                <th className="text-left p-3 min-w-[140px]">转接来电</th>
                <th className="text-left p-3 min-w-[200px]">状态</th>
                <th className="text-left p-3 whitespace-nowrap text-xs">状态时间</th>
                <th className="text-right p-3 whitespace-nowrap">操作</th>
              </tr>
            </thead>
            <tbody>
              {rows.length === 0 ? (
                <tr><td colSpan={9} className="p-6 text-center text-muted-foreground">暂无数据</td></tr>
              ) : rows.map((r, idx) => {
                const inc = r.routeType === 'sip' ? incomingBySeatId[seatIdKey(r.id)] : undefined
                return (
                <tr
                  key={seatIdKey(r.id)}
                  className={`border-t border-border align-top${rowDragClass(idx)}`}
                  onDragEnter={(e) => {
                    if (!dragEnabled || dragIdx == null || dragIdx === idx) return
                    e.preventDefault()
                    setDropOverIdx(idx)
                  }}
                  onDragOver={(e) => {
                    if (!dragEnabled || dragIdx == null) return
                    e.preventDefault()
                    e.dataTransfer.dropEffect = 'move'
                    if (dragIdx !== idx) setDropOverIdx(idx)
                  }}
                  onDragLeave={(e) => {
                    const related = e.relatedTarget as Node | null
                    if (related && (e.currentTarget as Node).contains(related)) return
                    if (dropOverIdx === idx) setDropOverIdx(null)
                  }}
                  onDrop={(e) => {
                    e.preventDefault()
                    handleDrop(idx)
                  }}
                >
                  <td className="p-3 whitespace-nowrap">
                    <span className="inline-flex items-center gap-1.5 text-muted-foreground">
                      <span
                        className={`acd-pool-drag-handle inline-flex items-center${dragEnabled ? ' cursor-grab active:cursor-grabbing' : ' opacity-40'}`}
                        draggable={dragEnabled}
                        title={dragEnabled ? '拖动排序' : '至少两条记录可排序'}
                        onDragStart={(e) => {
                          e.stopPropagation()
                          e.dataTransfer.effectAllowed = 'move'
                          e.dataTransfer.setData('text/plain', String(idx))
                          setDragIdx(idx)
                          setDropOverIdx(null)
                        }}
                        onDragEnd={clearDragState}
                      >
                        <IconDragDotVertical />
                      </span>
                      <span className="text-xs tabular-nums w-4 text-center">{idx + 1}</span>
                    </span>
                  </td>
                  <td className="p-3 text-xs text-muted-foreground max-w-[200px]">{trunkNumLabel(r.trunkNumberId)}</td>
                  <td className="p-3 max-w-[200px] truncate">{r.name || '—'}</td>
                  <td className="p-3 font-mono text-xs max-w-[260px] break-all text-muted-foreground">{r.routeType === 'sip' ? r.targetValue || '—' : '—'}</td>
                  <td className="p-3 whitespace-nowrap text-xs text-muted-foreground">{shiftScheduleSummary(r.shiftSchedule)}</td>
                  <td className="p-3 align-top">
                    {r.routeType === 'sip' ? (
                      inc?.incoming ? (
                        <Space direction="vertical" size={2}>
                          <Tag size="small" color="orangered" icon={<IconPhone />}>振铃中</Tag>
                          <span className="font-mono text-xs text-muted-foreground">{callerDisplay(inc)}</span>
                        </Space>
                      ) : (
                        <Typography.Text type="secondary" style={{ fontSize: 12 }}>—</Typography.Text>
                      )
                    ) : (
                      <Typography.Text type="secondary" style={{ fontSize: 12 }}>Web</Typography.Text>
                    )}
                  </td>
                  <td className="p-3 align-top"><Tag size="small">{workStateLabel(r.workState)}</Tag></td>
                  <td className="p-3 whitespace-nowrap text-xs text-muted-foreground">{r.workStateAt ? new Date(r.workStateAt).toLocaleString() : '—'}</td>
                  <td className="p-3 text-right">
                    <Space>
                      <Button type="outline" size="small" onClick={() => openEdit(r)}>编辑</Button>
                      <Button type="outline" status="danger" size="small" icon={<IconDelete />} onClick={() => { // @ts-ignore
                          setAcdDeleteId(r.id); setAcdDeleteOpen(true) }}>删除</Button>
                    </Space>
                  </td>
                </tr>
              )})}
            </tbody>
          </table>
          <div className="flex items-center justify-between p-3 border-t border-border text-sm">
            <span className="text-muted-foreground">
              总计: {total}
              {reordering ? ' · 保存排序中…' : ''}
            </span>
            {!trunkNumFilter && (
            <Space>
              <Button size="small" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>上一页</Button>
              <Button size="small" disabled={page * listPageSize >= total} onClick={() => setPage((p) => p + 1)}>下一页</Button>
            </Space>
            )}
          </div>
        </div>
      )}

      <Drawer
        title={editingId == null ? '新增 SIP 目标' : '编辑目标'}
        visible={modalOpen}
        placement="right"
        width={620}
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
            <Typography.Text style={{ fontSize: 12 }}>中继号码 *</Typography.Text>
            <Select
              placeholder={trunkNumOpts.length ? '请选择已分配给本租户的号码' : '暂无可用号码，请联系平台分配'}
              value={form.trunkNumberId || undefined}
              onChange={(v) => setForm((f) => ({ ...f, trunkNumberId: (v as number) ?? 0 }))}
              options={trunkNumOpts}
            />
            <Typography.Paragraph type="secondary" style={{ margin: '6px 0 0', fontSize: 11 }}>
              必须先选择号码再配置 SIP/Web 坐席；列表来自「中继号码」中 tenantId 指向当前租户的记录。
            </Typography.Paragraph>
          </div>
          <div>
            <Typography.Text style={{ fontSize: 12 }}>名称</Typography.Text>
            <Input value={form.name} onChange={(v) => setForm((f) => ({ ...f, name: v }))} />
          </div>
          {(editingId == null || form.routeType === 'sip') && (
            <div>
              <Typography.Text style={{ fontSize: 12 }}>呼叫电话号</Typography.Text>
              <Input placeholder="例如 10086 或 13800138000" value={form.targetValue} onChange={(v) => setForm((f) => ({ ...f, targetValue: v }))} />
            </div>
          )}
          <div>
            <Typography.Text style={{ fontSize: 12 }}>工作状态</Typography.Text>
            <Select
              value={form.workState}
              onChange={(v) => setForm((f) => ({ ...f, workState: v as string }))}
              options={ACD_WORK_STATES.filter((ws) => ws === 'offline' || ws === 'available' || ws === 'break').map((ws) => ({ label: workStateLabel(ws), value: ws }))}
            />
          </div>
          <div>
            <Typography.Text style={{ fontSize: 12 }}>备注（可选）</Typography.Text>
            <Input
              maxLength={128}
              showWordLimit
              placeholder="管理员备注，模板占位符 {{Note}}"
              value={form.remark}
              onChange={(v) => setForm((f) => ({ ...f, remark: v }))}
            />
          </div>
          <MetaDataKeyValueEditor
            pairs={form.metaDataPairs}
            onChange={(metaDataPairs) => setForm((f) => ({ ...f, metaDataPairs }))}
          />
          <div>
            <Typography.Text style={{ fontSize: 12 }}>接线班次（可选）</Typography.Text>
            <div className="mt-2 flex flex-wrap items-center gap-3 rounded-md border border-border bg-muted/20 px-3 py-2.5">
              <Typography.Text style={{ fontSize: 12, flex: '1 1 200px', minWidth: 0 }}>
                当前：{shiftScheduleSummary(form.shiftSchedule)}
              </Typography.Text>
              <Button type="primary" size="small" onClick={() => setShiftModalOpen(true)}>
                配置座席时间策略
              </Button>
            </div>
            <Typography.Paragraph style={{ margin: '6px 0 0', fontSize: 11 }} type="secondary">
              按星期配置；同一天可添加多段时段（如午休前后），各时段纵向对齐。
            </Typography.Paragraph>
          </div>
        </Space>
      </Drawer>

      <ShiftScheduleModal
        visible={shiftModalOpen}
        value={form.shiftSchedule}
        onCancel={() => setShiftModalOpen(false)}
        onConfirm={(serialized) => {
          setForm((f) => ({ ...f, shiftSchedule: serialized }))
          setShiftModalOpen(false)
        }}
      />

      <Drawer
        title="确认删除号码池目标"
        visible={acdDeleteOpen}
        placement="right"
        width={420}
        onCancel={() => { if (!acdDeleteLoading) { setAcdDeleteOpen(false); setAcdDeleteId(null) } }}
        footer={
          <Space>
            <Button onClick={() => { if (!acdDeleteLoading) { setAcdDeleteOpen(false); setAcdDeleteId(null) } }} disabled={acdDeleteLoading}>
              取消
            </Button>
            <Button status="danger" loading={acdDeleteLoading} onClick={() => void confirmAcdDelete()}>
              确认删除
            </Button>
          </Space>
        }
      >
        <Typography.Text>删除后不可恢复，确认继续吗？</Typography.Text>
      </Drawer>
    </div>
  )
}
