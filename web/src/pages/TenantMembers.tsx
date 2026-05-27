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
  Typography,
} from '@arco-design/web-react'
import BaseLayout from '@/components/Layout/BaseLayout'
import { useTranslation } from '@/i18n'
import {
  createTenantUser,
  deleteTenantUser,
  listTenantUsers,
  updateTenantUser,
  updateTenantUserStatus,
  type TenantUserRow,
} from '@/api/tenantUsers'
import { useAuthStore } from '@/stores/authStore'
import { showAlert } from '@/utils/notification'

const statusOpts = [
  { label: '正常', value: 'active' },
  { label: '禁用', value: 'disabled' },
  { label: '待激活', value: 'pending' },
]

export default function TenantMembers() {
  const { t } = useTranslation()
  const meId = Number(useAuthStore((s) => s.user?.id) ?? 0)
  const [rows, setRows] = useState<TenantUserRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm] = Form.useForm()
  const [creating, setCreating] = useState(false)

  const [editOpen, setEditOpen] = useState(false)
  const [editForm] = Form.useForm()
  const [editing, setEditing] = useState(false)
  const [editId, setEditId] = useState<number | null>(null)

  const [delTarget, setDelTarget] = useState<TenantUserRow | null>(null)
  const [delLoading, setDelLoading] = useState(false)

  const pageSize = 20

  const loadUsers = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listTenantUsers(page, pageSize)
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
  }, [page])

  useEffect(() => {
    void loadUsers()
  }, [loadUsers])

  const submitCreate = async () => {
    setCreating(true)
    try {
      const v = await createForm.validate()
      const res = await createTenantUser({
        email: String(v.email || '').trim(),
        password: String(v.password || ''),
        displayName: String(v.displayName || '').trim() || undefined,
        phone: String(v.phone || '').trim() || undefined,
        username: String(v.username || '').trim() || undefined,
        status: (v.status as string) || 'active',
      })
      if (res.code === 200) {
        Message.success('成员已创建')
        setCreateOpen(false)
        createForm.resetFields()
        void loadUsers()
      } else {
        showAlert(res.msg || '创建失败', 'error')
      }
    } catch {
      /* validation or API */
    } finally {
      setCreating(false)
    }
  }

  const openEdit = (r: TenantUserRow) => {
    setEditId(r.id)
    editForm.setFieldsValue({
      email: r.email || '',
      phone: r.phone || '',
      username: r.username || '',
      displayName: r.displayName || '',
      status: r.status || 'active',
    })
    setEditOpen(true)
  }

  const submitEdit = async () => {
    if (editId == null) return
    setEditing(true)
    try {
      const v = await editForm.validate()
      const res = await updateTenantUser(editId, {
        email: String(v.email || '').trim() || undefined,
        phone: String(v.phone || '').trim() || undefined,
        username: String(v.username || '').trim() || undefined,
        displayName: String(v.displayName || '').trim() || undefined,
        status: String(v.status || '').trim() || undefined,
      })
      if (res.code === 200) {
        Message.success('已保存')
        setEditOpen(false)
        void loadUsers()
      } else {
        showAlert(res.msg || '保存失败', 'error')
      }
    } catch {
      /* validation */
    } finally {
      setEditing(false)
    }
  }

  const quickStatus = async (r: TenantUserRow, status: string) => {
    try {
      const res = await updateTenantUserStatus(r.id, status)
      if (res.code === 200) {
        Message.success('状态已更新')
        void loadUsers()
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
      const res = await deleteTenantUser(delTarget.id)
      if (res.code === 200) {
        Message.success('已删除')
        setDelTarget(null)
        void loadUsers()
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
    <BaseLayout
      title={t('pages.tenantMembers.title')}
      description={t('pages.tenantMembers.description')}
    >
      <Space direction="vertical" size={16} style={{ width: '100%' }}>
        <Card bordered={false}>
          <Space style={{ marginBottom: 12 }} wrap>
            <Typography.Title heading={6} style={{ margin: 0 }}>
              成员列表
            </Typography.Title>
            <Button
              type="primary"
              onClick={() => {
                createForm.resetFields()
                createForm.setFieldsValue({ status: 'active' })
                setCreateOpen(true)
              }}
            >
              新建成员
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
              { title: '邮箱', dataIndex: 'email' },
              { title: '显示名', dataIndex: 'displayName' },
              { title: '手机', dataIndex: 'phone' },
              { title: '状态', dataIndex: 'status', width: 100 },
              {
                title: '操作',
                width: 280,
                render: (_: unknown, r: TenantUserRow) => (
                  <Space>
                    <Button type="text" size="small" onClick={() => openEdit(r)}>
                      编辑
                    </Button>
                    {r.status === 'active' ? (
                      <Button type="text" size="small" onClick={() => void quickStatus(r, 'disabled')}>
                        禁用
                      </Button>
                    ) : (
                      <Button type="text" size="small" onClick={() => void quickStatus(r, 'active')}>
                        启用
                      </Button>
                    )}
                    <Button
                      type="text"
                      size="small"
                      status="danger"
                      disabled={r.id === meId}
                      onClick={() => setDelTarget(r)}
                    >
                      删除
                    </Button>
                  </Space>
                ),
              },
            ]}
          />
        </Card>
      </Space>

      <Modal
        title="新建成员"
        visible={createOpen}
        onCancel={() => !creating && setCreateOpen(false)}
        onOk={() => void submitCreate()}
        confirmLoading={creating}
        unmountOnExit
      >
        <Form form={createForm} layout="vertical">
          <Form.Item label="邮箱" field="email" rules={[{ required: true, message: '必填' }]}>
            <Input placeholder="login@example.com" />
          </Form.Item>
          <Form.Item
            label="初始密码"
            field="password"
            rules={[
              { required: true, message: '必填' },
              {
                validator: (v, cb) => {
                  if (v && String(v).length >= 8) return cb()
                  cb('不少于 8 位')
                },
              },
            ]}
          >
            <Input.Password placeholder="不少于 8 位" />
          </Form.Item>
          <Form.Item label="显示名" field="displayName">
            <Input />
          </Form.Item>
          <Form.Item label="手机" field="phone">
            <Input />
          </Form.Item>
          <Form.Item label="用户名" field="username">
            <Input />
          </Form.Item>
          <Form.Item label="状态" field="status" initialValue="active">
            <Select options={statusOpts} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="编辑成员"
        visible={editOpen}
        onCancel={() => !editing && setEditOpen(false)}
        onOk={() => void submitEdit()}
        confirmLoading={editing}
        unmountOnExit
      >
        <Form form={editForm} layout="vertical">
          <Form.Item label="邮箱" field="email" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item label="显示名" field="displayName">
            <Input />
          </Form.Item>
          <Form.Item label="手机" field="phone">
            <Input />
          </Form.Item>
          <Form.Item label="用户名" field="username">
            <Input />
          </Form.Item>
          <Form.Item label="状态" field="status">
            <Select options={statusOpts} />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title="确认删除成员"
        visible={!!delTarget}
        onCancel={() => !delLoading && setDelTarget(null)}
        onOk={() => void confirmDelete()}
        confirmLoading={delLoading}
      >
        <Typography.Text>
          删除后该成员将无法登录（软删除）。确定删除 {delTarget?.email}？
        </Typography.Text>
      </Modal>
    </BaseLayout>
  )
}
