import { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Form,
  Input,
  Message,
  Modal,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from '@arco-design/web-react'
import BaseLayout from '@/components/Layout/BaseLayout'
import {
  createPlatformAdmin,
  deletePlatformAdmin,
  listPlatformAdmins,
  resetPlatformAdminPassword,
  updatePlatformAdmin,
  updatePlatformAdminStatus,
  type PlatformAdminRow,
} from '@/api/platformAdmins'
import { useAuthStore } from '@/stores/authStore'
import { showAlert } from '@/utils/notification'

const statusOpts = [
  { label: '正常', value: 'active' },
  { label: '已停用', value: 'disabled' },
]

function fmtStatus(s?: string) {
  const v = String(s || '').toLowerCase()
  if (v === 'active') return '正常'
  if (v === 'disabled') return '已停用'
  return s || '—'
}

export default function PlatformAdmins() {
  const meId = Number(useAuthStore((s) => s.user?.id) ?? 0)
  const [rows, setRows] = useState<PlatformAdminRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm] = Form.useForm()
  const [creating, setCreating] = useState(false)

  const [editOpen, setEditOpen] = useState(false)
  const [editForm] = Form.useForm()
  const [editing, setEditing] = useState(false)
  const [editRow, setEditRow] = useState<PlatformAdminRow | null>(null)

  const [pwdOpen, setPwdOpen] = useState(false)
  const [pwdForm] = Form.useForm()
  const [pwdLoading, setPwdLoading] = useState(false)
  const [pwdTarget, setPwdTarget] = useState<PlatformAdminRow | null>(null)

  const [delTarget, setDelTarget] = useState<PlatformAdminRow | null>(null)
  const [delLoading, setDelLoading] = useState(false)

  const pageSize = 20

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listPlatformAdmins(page, pageSize, search)
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
  }, [page, search])

  useEffect(() => {
    void load()
  }, [load])

  const submitCreate = async () => {
    setCreating(true)
    try {
      const v = await createForm.validate()
      const res = await createPlatformAdmin({
        email: String(v.email || '').trim(),
        password: String(v.password || ''),
        displayName: String(v.displayName || '').trim() || undefined,
        status: (v.status as string) || 'active',
      })
      if (res.code === 200) {
        Message.success('管理员已创建')
        setCreateOpen(false)
        createForm.resetFields()
        void load()
      } else {
        showAlert(res.msg || '创建失败', 'error')
      }
    } catch {
      /* validation */
    } finally {
      setCreating(false)
    }
  }

  const openEdit = (r: PlatformAdminRow) => {
    setEditRow(r)
    editForm.setFieldsValue({
      email: r.email || '',
      displayName: r.displayName || '',
    })
    setEditOpen(true)
  }

  const submitEdit = async () => {
    if (!editRow) return
    setEditing(true)
    try {
      const v = await editForm.validate()
      const res = await updatePlatformAdmin(editRow.id, {
        email: String(v.email || '').trim() || undefined,
        displayName: String(v.displayName || '').trim() || undefined,
      })
      if (res.code === 200) {
        Message.success('已保存')
        setEditOpen(false)
        void load()
      } else {
        showAlert(res.msg || '保存失败', 'error')
      }
    } catch {
      /* validation */
    } finally {
      setEditing(false)
    }
  }

  const quickStatus = async (r: PlatformAdminRow, status: 'active' | 'disabled') => {
    try {
      const res = await updatePlatformAdminStatus(r.id, status)
      if (res.code === 200) {
        Message.success('状态已更新')
        void load()
      } else {
        showAlert(res.msg || '操作失败', 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : '操作失败', 'error')
    }
  }

  const submitPassword = async () => {
    if (!pwdTarget) return
    setPwdLoading(true)
    try {
      const v = await pwdForm.validate()
      const res = await resetPlatformAdminPassword(pwdTarget.id, String(v.password || ''))
      if (res.code === 200) {
        Message.success('密码已重置')
        setPwdOpen(false)
        pwdForm.resetFields()
      } else {
        showAlert(res.msg || '重置失败', 'error')
      }
    } catch {
      /* validation */
    } finally {
      setPwdLoading(false)
    }
  }

  const confirmDelete = async () => {
    if (!delTarget) return
    setDelLoading(true)
    try {
      const res = await deletePlatformAdmin(delTarget.id)
      if (res.code === 200) {
        Message.success('已删除')
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

  return (
    <BaseLayout title="平台管理员" description="运维账号：启用/停用、重置密码（不可停用或删除当前登录账号）">
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Card bordered={false}>
          <Space style={{ marginBottom: 12 }} wrap>
            <Typography.Title heading={6} style={{ margin: 0 }}>
              管理员列表
            </Typography.Title>
            <Input.Search
              allowClear
              placeholder="搜索邮箱 / 显示名"
              style={{ width: 240 }}
              onSearch={(v) => {
                setSearch(v)
                setPage(1)
              }}
            />
            <Button type="primary" onClick={() => setCreateOpen(true)}>
              新增管理员
            </Button>
          </Space>
          <Table
            rowKey="id"
            loading={loading}
            data={rows}
            pagination={{
              current: page,
              pageSize,
              total,
              onChange: (p) => setPage(p),
            }}
            columns={[
              { title: '邮箱', dataIndex: 'email', ellipsis: true },
              { title: '显示名', dataIndex: 'displayName', render: (v) => v || '—' },
              {
                title: '状态',
                dataIndex: 'status',
                width: 100,
                render: (s: string) => (
                  <Tag color={s === 'active' ? 'green' : 'gray'}>{fmtStatus(s)}</Tag>
                ),
              },
              {
                title: '操作',
                width: 320,
                render: (_, r) => {
                  const isSelf = meId > 0 && Number(r.id) === meId
                  return (
                    <Space wrap size="mini">
                      <Button size="mini" onClick={() => openEdit(r)}>
                        编辑
                      </Button>
                      <Button size="mini" onClick={() => { setPwdTarget(r); setPwdOpen(true) }}>
                        重置密码
                      </Button>
                      {r.status === 'active' ? (
                        <Button
                          size="mini"
                          status="warning"
                          disabled={isSelf}
                          onClick={() => void quickStatus(r, 'disabled')}
                        >
                          停用
                        </Button>
                      ) : (
                        <Button size="mini" type="outline" onClick={() => void quickStatus(r, 'active')}>
                          启用
                        </Button>
                      )}
                      <Button size="mini" status="danger" disabled={isSelf} onClick={() => setDelTarget(r)}>
                        删除
                      </Button>
                    </Space>
                  )
                },
              },
            ]}
          />
        </Card>
      </Space>

      <Modal title="新增平台管理员" visible={createOpen} onOk={() => void submitCreate()} onCancel={() => setCreateOpen(false)} confirmLoading={creating}>
        <Form form={createForm} layout="vertical" initialValues={{ status: 'active' }}>
          <Form.Item label="邮箱" field="email" rules={[{ required: true, type: 'email' }]}>
            <Input />
          </Form.Item>
          <Form.Item label="密码" field="password" rules={[{ required: true, minLength: 8 }]}>
            <Input.Password />
          </Form.Item>
          <Form.Item label="显示名" field="displayName">
            <Input />
          </Form.Item>
          <Form.Item label="状态" field="status">
            <Select options={statusOpts} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal title="编辑" visible={editOpen} onOk={() => void submitEdit()} onCancel={() => setEditOpen(false)} confirmLoading={editing}>
        <Form form={editForm} layout="vertical">
          <Form.Item label="邮箱" field="email" rules={[{ type: 'email' }]}>
            <Input />
          </Form.Item>
          <Form.Item label="显示名" field="displayName">
            <Input />
          </Form.Item>
        </Form>
      </Modal>

      <Modal title={`重置密码 · ${pwdTarget?.email || ''}`} visible={pwdOpen} onOk={() => void submitPassword()} onCancel={() => setPwdOpen(false)} confirmLoading={pwdLoading}>
        <Form form={pwdForm} layout="vertical">
          <Form.Item label="新密码" field="password" rules={[{ required: true, minLength: 8 }]}>
            <Input.Password />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="删除管理员"
        visible={!!delTarget}
        onOk={() => void confirmDelete()}
        onCancel={() => setDelTarget(null)}
        confirmLoading={delLoading}
      >
        确认删除 {delTarget?.email}？此操作不可恢复。
      </Modal>
    </BaseLayout>
  )
}
