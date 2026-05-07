import { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Drawer,
  Input,
  Modal,
  Select,
  Space,
  Tag,
  Typography,
} from '@arco-design/web-react'
import { IconCopy, IconDelete } from '@arco-design/web-react/icon'
import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import {
  createCredential,
  deleteCredential,
  disableCredential,
  enableCredential,
  listCredentials,
  updateCredential,
  type CredentialCreateResult,
  type CredentialRow,
  type CredentialStatus,
} from '@/api/credentials'
import { showAlert } from '@/utils/notification'

type EditState = {
  id: number | null
  name: string
  allowIp: string
  permissionCodesJson: string
}

const defaultEdit = (): EditState => ({
  id: null,
  name: '',
  allowIp: '',
  permissionCodesJson: '["*"]',
})

const statusOptions: { label: string; value: '' | CredentialStatus }[] = [
  { label: '全部', value: '' },
  { label: '启用', value: 'active' },
  { label: '禁用', value: 'disabled' },
]

const StatusTag = ({ status }: { status: CredentialStatus }) => {
  if (status === 'active') return <Tag color="green">启用</Tag>
  return <Tag color="red">禁用</Tag>
}

const AccessKeys = () => {
  const [rows, setRows] = useState<CredentialRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [statusQ, setStatusQ] = useState<'' | CredentialStatus>('')
  const [nameQ, setNameQ] = useState('')
  const [loading, setLoading] = useState(false)

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm, setCreateForm] = useState({ name: '', allowIp: '', permissionCodesJson: '["*"]' })
  const [creating, setCreating] = useState(false)

  const [revealOpen, setRevealOpen] = useState(false)
  const [issued, setIssued] = useState<CredentialCreateResult | null>(null)

  const [editOpen, setEditOpen] = useState(false)
  const [editForm, setEditForm] = useState<EditState>(defaultEdit)
  const [editing, setEditing] = useState(false)

  const [delTarget, setDelTarget] = useState<CredentialRow | null>(null)
  const [delLoading, setDelLoading] = useState(false)

  const pageSize = 20

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listCredentials({
        page,
        size: pageSize,
        status: statusQ || undefined,
        name: nameQ.trim() || undefined,
      })
      if (res.code === 200 && res.data) {
        setRows(res.data.list || [])
        setTotal(res.data.total || 0)
      } else {
        showAlert(res.msg || '加载失败', 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '加载失败', 'error')
    } finally {
      setLoading(false)
    }
  }, [page, statusQ, nameQ])

  useEffect(() => {
    void load()
  }, [load])

  const openCreate = () => {
    setCreateForm({ name: '', allowIp: '', permissionCodesJson: '["*"]' })
    setCreateOpen(true)
  }

  const parsePermissionCodes = (raw: string): string[] | undefined => {
    const s = raw.trim()
    if (!s) return undefined
    const parsed = JSON.parse(s) as unknown
    if (!Array.isArray(parsed) || !parsed.every((x) => typeof x === 'string')) {
      throw new Error('权限码须为非空 JSON 字符串数组')
    }
    return parsed
  }

  const submitCreate = async () => {
    setCreating(true)
    try {
      let permissionCodes: string[] | undefined
      try {
        permissionCodes = parsePermissionCodes(createForm.permissionCodesJson)
      } catch {
        showAlert('权限码格式无效（需 JSON 数组，如 ["*"] 或 []）', 'error')
        setCreating(false)
        return
      }
      const res = await createCredential({
        name: createForm.name.trim() || undefined,
        allowIp: createForm.allowIp.trim() || undefined,
        permissionCodes,
      })
      if (res.code === 200 && res.data) {
        setIssued(res.data)
        setRevealOpen(true)
        setCreateOpen(false)
        void load()
      } else {
        showAlert(res.msg || '创建失败', 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '创建失败', 'error')
    } finally {
      setCreating(false)
    }
  }

  const openEdit = (r: CredentialRow) => {
    const pc = Array.isArray(r.permissionCodes) ? r.permissionCodes : ['*']
    setEditForm({
      id: r.id,
      name: r.name || '',
      allowIp: r.allowIp || '',
      permissionCodesJson: JSON.stringify(pc),
    })
    setEditOpen(true)
  }

  const submitEdit = async () => {
    if (editForm.id == null) return
    const name = editForm.name.trim()
    if (!name) {
      showAlert('名称不能为空', 'error')
      return
    }
    let permissionCodes: string[]
    try {
      const parsed = parsePermissionCodes(editForm.permissionCodesJson)
      permissionCodes = parsed ?? []
    } catch {
      showAlert('权限码格式无效（需 JSON 数组）', 'error')
      return
    }
    setEditing(true)
    try {
      const res = await updateCredential(editForm.id, {
        name,
        allowIp: editForm.allowIp.trim(),
        permissionCodes,
      })
      if (res.code === 200) {
        showAlert('保存成功', 'success')
        setEditOpen(false)
        void load()
      } else {
        showAlert(res.msg || '保存失败', 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '保存失败', 'error')
    } finally {
      setEditing(false)
    }
  }

  const toggleStatus = async (r: CredentialRow) => {
    try {
      const res = r.status === 'active' ? await disableCredential(r.id) : await enableCredential(r.id)
      if (res.code === 200) {
        showAlert(r.status === 'active' ? '已禁用' : '已启用', 'success')
        void load()
      } else {
        showAlert(res.msg || '操作失败', 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '操作失败', 'error')
    }
  }

  const confirmDelete = async () => {
    if (!delTarget) return
    setDelLoading(true)
    try {
      const res = await deleteCredential(delTarget.id)
      if (res.code === 200) {
        showAlert('已删除', 'success')
        setDelTarget(null)
        void load()
      } else {
        showAlert(res.msg || '删除失败', 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '删除失败', 'error')
    } finally {
      setDelLoading(false)
    }
  }

  const copy = async (label: string, value: string) => {
    try {
      await navigator.clipboard.writeText(value)
      showAlert(`${label} 已复制到剪贴板`, 'success')
    } catch {
      showAlert(`${label} 复制失败，请手动选择`, 'error')
    }
  }

  return (
    <BaseLayout title="访问管理" description="租户级 API AK / SK 凭据，用于第三方/集成系统调用">
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Typography.Paragraph
          style={{
            margin: 0,
            fontSize: 12,
            padding: '10px 12px',
            background: 'var(--color-fill-2)',
            borderRadius: 8,
          }}
        >
          AK 与 SK 由后端生成，SK 仅在创建时显示一次，请立即妥善保存；丢失后只能重新创建一对。
          已禁用或已删除的 AK 在 AKSK 鉴权中间件中会立即失效。权限码为 JSON 数组：默认 <code>["*"]</code> 表示继承当前路由所需权限；空数组{' '}
          <code>[]</code> 表示禁止所有需权限的接口。
        </Typography.Paragraph>

        <Space wrap align="end">
          <Space direction="vertical" size={4}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>名称</Typography.Text>
            <Input
              allowClear
              placeholder="模糊搜索"
              style={{ width: 200 }}
              value={nameQ}
              onChange={setNameQ}
            />
          </Space>
          <Space direction="vertical" size={4}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>状态</Typography.Text>
            <Select
              style={{ width: 140 }}
              value={statusQ}
              onChange={(v) => setStatusQ((v as '' | CredentialStatus) ?? '')}
              options={statusOptions}
            />
          </Space>
          <Button
            type="primary"
            onClick={() => {
              setPage(1)
              void load()
            }}
          >
            搜索
          </Button>
          <Button type="outline" onClick={openCreate}>
            新建访问凭据
          </Button>
        </Space>

        <Card bordered={false}>
          {loading ? (
            <Typography.Text type="secondary">加载中...</Typography.Text>
          ) : (
            <>
              <div style={{ overflowX: 'auto' }}>
                <table style={{ minWidth: 960, width: '100%', fontSize: 13 }}>
                  <thead style={{ background: 'var(--color-fill-2)' }}>
                    <tr>
                      <th style={{ textAlign: 'left', padding: 12 }}>ID</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>名称</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>Access Key</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>状态</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>权限码</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>白名单 IP</th>
                      <th style={{ textAlign: 'left', padding: 12 }}>创建时间</th>
                      <th style={{ textAlign: 'right', padding: 12 }}>操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.length === 0 ? (
                      <tr>
                        <td colSpan={8} style={{ padding: 24, textAlign: 'center', color: 'var(--color-text-3)' }}>
                          暂无数据
                        </td>
                      </tr>
                    ) : (
                      rows.map((r) => (
                        <tr key={r.id} style={{ borderTop: '1px solid var(--color-border)' }}>
                          <td style={{ padding: 12 }}>{r.id}</td>
                          <td style={{ padding: 12, maxWidth: 200 }}>
                            <div style={{ fontWeight: 500 }}>{r.name || '—'}</div>
                            {r.createBy && (
                              <div style={{ fontSize: 12, color: 'var(--color-text-3)' }}>by {r.createBy}</div>
                            )}
                          </td>
                          <td
                            style={{
                              padding: 12,
                              fontFamily: 'monospace',
                              fontSize: 12,
                              wordBreak: 'break-all',
                              maxWidth: 320,
                            }}
                          >
                            <Space size={6}>
                              <span>{r.accessKey}</span>
                              <Button
                                type="text"
                                size="mini"
                                icon={<IconCopy />}
                                onClick={() => void copy('Access Key', r.accessKey)}
                              />
                            </Space>
                          </td>
                          <td style={{ padding: 12 }}>
                            <StatusTag status={r.status} />
                          </td>
                          <td
                            style={{
                              padding: 12,
                              fontFamily: 'monospace',
                              fontSize: 11,
                              wordBreak: 'break-all',
                              maxWidth: 160,
                              color: 'var(--color-text-2)',
                            }}
                          >
                            {r.permissionCodes && r.permissionCodes.length
                              ? r.permissionCodes.join(', ')
                              : '—'}
                          </td>
                          <td
                            style={{
                              padding: 12,
                              fontFamily: 'monospace',
                              fontSize: 12,
                              wordBreak: 'break-all',
                              maxWidth: 220,
                            }}
                          >
                            {r.allowIp || '不限制'}
                          </td>
                          <td style={{ padding: 12, fontSize: 12, color: 'var(--color-text-3)' }}>
                            {r.createdAt ? new Date(r.createdAt).toLocaleString() : '—'}
                          </td>
                          <td style={{ padding: 12, textAlign: 'right' }}>
                            <Space>
                              <Button type="outline" size="small" onClick={() => openEdit(r)}>
                                编辑
                              </Button>
                              <Button
                                type="outline"
                                size="small"
                                status={r.status === 'active' ? 'warning' : 'success'}
                                onClick={() => void toggleStatus(r)}
                              >
                                {r.status === 'active' ? '禁用' : '启用'}
                              </Button>
                              <Button
                                type="outline"
                                status="danger"
                                size="small"
                                icon={<IconDelete />}
                                onClick={() => setDelTarget(r)}
                              >
                                删除
                              </Button>
                            </Space>
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
              <div
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  marginTop: 12,
                  paddingTop: 12,
                  borderTop: '1px solid var(--color-border)',
                }}
              >
                <Typography.Text type="secondary">总计: {total}</Typography.Text>
                <Space>
                  <Button size="small" disabled={page <= 1} onClick={() => setPage((p) => Math.max(1, p - 1))}>
                    上一页
                  </Button>
                  <Button size="small" disabled={page * pageSize >= total} onClick={() => setPage((p) => p + 1)}>
                    下一页
                  </Button>
                </Space>
              </div>
            </>
          )}
        </Card>

        <Drawer
          title="新建访问凭据"
          visible={createOpen}
          placement="right"
          width={520}
          onCancel={() => {
            if (!creating) setCreateOpen(false)
          }}
          footer={
            <Space>
              <Button onClick={() => setCreateOpen(false)} disabled={creating}>
                取消
              </Button>
              <Button type="primary" loading={creating} onClick={() => void submitCreate()}>
                {creating ? '创建中...' : '创建'}
              </Button>
            </Space>
          }
        >
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>名称</Typography.Text>
              <Input
                placeholder="例如：CRM-Prod"
                value={createForm.name}
                onChange={(v) => setCreateForm((f) => ({ ...f, name: v }))}
              />
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>白名单 IP（可选，逗号分隔）</Typography.Text>
              <Input
                placeholder="留空表示不限制；例如：1.1.1.1, 2.2.2.2"
                value={createForm.allowIp}
                onChange={(v) => setCreateForm((f) => ({ ...f, allowIp: v }))}
              />
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>权限码（JSON 数组）</Typography.Text>
              <Input.TextArea
                placeholder='例如 ["*"] 或 ["api.sip.calls.read"]；留空等同默认通配'
                autoSize={{ minRows: 2, maxRows: 6 }}
                value={createForm.permissionCodesJson}
                onChange={(v) => setCreateForm((f) => ({ ...f, permissionCodesJson: v }))}
                style={{ fontFamily: 'monospace', fontSize: 12 }}
              />
            </div>
            <Typography.Paragraph type="warning" style={{ margin: 0, fontSize: 12 }}>
              创建后会展示一次 Secret Key，请立即复制保存。
            </Typography.Paragraph>
          </Space>
        </Drawer>

        <Modal
          title="新凭据已创建"
          visible={revealOpen}
          maskClosable={false}
          onCancel={() => {
            setRevealOpen(false)
            setIssued(null)
          }}
          footer={
            <Button
              type="primary"
              onClick={() => {
                setRevealOpen(false)
                setIssued(null)
              }}
            >
              我已保存
            </Button>
          }
        >
          {issued && (
            <Space direction="vertical" style={{ width: '100%' }} size={12}>
              <Typography.Paragraph type="warning" style={{ margin: 0 }}>
                Secret Key 仅本次显示，请立即复制并妥善保存；离开后无法再次查看。
              </Typography.Paragraph>
              <div>
                <Typography.Text style={{ fontSize: 12 }}>名称</Typography.Text>
                <div style={{ padding: '8px 0' }}>{issued.name}</div>
              </div>
              <div>
                <Typography.Text style={{ fontSize: 12 }}>Access Key</Typography.Text>
                <Space>
                  <Input readOnly value={issued.accessKey} style={{ fontFamily: 'monospace' }} />
                  <Button icon={<IconCopy />} onClick={() => void copy('Access Key', issued.accessKey)}>
                    复制
                  </Button>
                </Space>
              </div>
              <div>
                <Typography.Text style={{ fontSize: 12 }}>Secret Key</Typography.Text>
                <Space>
                  <Input.TextArea
                    readOnly
                    value={issued.secretKey}
                    autoSize={{ minRows: 2, maxRows: 4 }}
                    style={{ fontFamily: 'monospace' }}
                  />
                  <Button icon={<IconCopy />} onClick={() => void copy('Secret Key', issued.secretKey)}>
                    复制
                  </Button>
                </Space>
              </div>
            </Space>
          )}
        </Modal>

        <Drawer
          title="编辑访问凭据"
          visible={editOpen}
          placement="right"
          width={520}
          onCancel={() => {
            if (!editing) setEditOpen(false)
          }}
          footer={
            <Space>
              <Button onClick={() => setEditOpen(false)} disabled={editing}>
                取消
              </Button>
              <Button type="primary" loading={editing} onClick={() => void submitEdit()}>
                {editing ? '保存中...' : '保存'}
              </Button>
            </Space>
          }
        >
          <Space direction="vertical" style={{ width: '100%' }} size={12}>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>名称 *</Typography.Text>
              <Input
                value={editForm.name}
                onChange={(v) => setEditForm((f) => ({ ...f, name: v }))}
              />
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>白名单 IP（可选，逗号分隔）</Typography.Text>
              <Input
                value={editForm.allowIp}
                onChange={(v) => setEditForm((f) => ({ ...f, allowIp: v }))}
              />
            </div>
            <div>
              <Typography.Text style={{ fontSize: 12 }}>权限码（JSON 数组）</Typography.Text>
              <Input.TextArea
                autoSize={{ minRows: 2, maxRows: 6 }}
                value={editForm.permissionCodesJson}
                onChange={(v) => setEditForm((f) => ({ ...f, permissionCodesJson: v }))}
                style={{ fontFamily: 'monospace', fontSize: 12 }}
              />
            </div>
            <Typography.Paragraph type="secondary" style={{ margin: 0, fontSize: 12 }}>
              出于安全考虑不支持修改 Access Key，需要轮换请新建一对并替换调用方配置。
            </Typography.Paragraph>
          </Space>
        </Drawer>

        <Modal
          title="确认删除访问凭据"
          visible={!!delTarget}
          maskClosable={false}
          onCancel={() => {
            if (!delLoading) setDelTarget(null)
          }}
          footer={
            <Space>
              <Button onClick={() => setDelTarget(null)} disabled={delLoading}>
                取消
              </Button>
              <Button status="danger" loading={delLoading} onClick={() => void confirmDelete()}>
                确认删除
              </Button>
            </Space>
          }
        >
          <Typography.Text>
            删除后该 Access Key 将立即失效，且无法恢复。请确认所有调用方都已停止使用：
          </Typography.Text>
          {delTarget && (
            <Typography.Paragraph
              style={{ marginTop: 8, fontFamily: 'monospace', fontSize: 12, wordBreak: 'break-all' }}
            >
              {delTarget.accessKey}
            </Typography.Paragraph>
          )}
        </Modal>
      </Space>
    </BaseLayout>
  )
}

export default AccessKeys
