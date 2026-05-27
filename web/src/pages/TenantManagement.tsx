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
} from '@arco-design/web-react'
import { Link, useNavigate } from 'react-router-dom'
import BaseLayout from '@/components/Layout/BaseLayout'
import { useTranslation } from '@/i18n'
import { TableIdCell } from '@/components/TableIdCell'
import { EllipsisHoverCell } from '@/pages/ContactCenter/EllipsisHoverCell'
import {
  createTenantPlatform,
  deleteTenantPlatform,
  listTenants,
  updateTenantPlatform,
  type TenantRow,
} from '@/api/tenants'

const FormItem = Form.Item

export default function TenantManagement() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [rows, setRows] = useState<TenantRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [size] = useState(20)
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(false)

  const [createOpen, setCreateOpen] = useState(false)
  const [createForm] = Form.useForm()

  const [editOpen, setEditOpen] = useState(false)
  const [editForm] = Form.useForm()
  const [editing, setEditing] = useState<TenantRow | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listTenants(page, size, { search: search.trim() || undefined })
      if (res.code === 200 && res.data) {
        setRows(res.data.list || [])
        setTotal(res.data.total ?? 0)
      } else {
        Message.error(res.msg || t('common.loadFailed'))
      }
    } finally {
      setLoading(false)
    }
  }, [page, search, size])

  useEffect(() => {
    void load()
  }, [load])

  return (
    <BaseLayout
      title={t('pages.tenantManagement.title')}
      description={t('pages.tenantManagement.description')}
    >
      <Card bordered={false}>
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <Space wrap>
            <Input.Search
              placeholder={t('tenantManagement.searchPlaceholder')}
              style={{ width: 280 }}
              allowClear
              onSearch={(v) => {
                setPage(1)
                setSearch(v)
              }}
            />
            <Button
              type="primary"
              onClick={() => {
                createForm.resetFields()
                setCreateOpen(true)
              }}
            >
              {t('tenantManagement.createTenant')}
            </Button>
            <Link to="/sip-trunk-numbers">
              <Button type="outline">{t('common.assignTrunkNumbers')}</Button>
            </Link>
          </Space>
          <Table
            rowKey="id"
            loading={loading}
            data={rows}
            tableLayoutFixed
            pagination={{
              current: page,
              pageSize: size,
              total,
              onChange: (p) => setPage(p),
            }}
            columns={[
              { title: 'ID', dataIndex: 'id', width: 96, render: (id: string) => <TableIdCell id={id} /> },
              {
                title: t('tenantManagement.colCompany'),
                dataIndex: 'name',
                width: 200,
                ellipsis: true,
                render: (v: string) => <EllipsisHoverCell text={v} lines={1} />,
              },
              {
                title: t('tenantManagement.colSlug'),
                dataIndex: 'slug',
                width: 160,
                ellipsis: true,
                render: (v: string) => <EllipsisHoverCell text={v} lines={1} mono />,
              },
              {
                title: t('tenantManagement.colEmail'),
                dataIndex: 'contactEmail',
                width: 200,
                ellipsis: true,
                render: (v: string) => <EllipsisHoverCell text={v || '—'} lines={1} />,
              },
              { title: t('tenantManagement.colMaxUsers'), dataIndex: 'maxUserCount', width: 96, render: (v: number) => (v && v > 0 ? v : 5) },
              {
                title: t('common.status'),
                dataIndex: 'status',
                width: 88,
                render: (v: string) => (v === 'suspended' ? t('tenantManagement.statusSuspended') : t('tenantManagement.statusActive')),
              },
              {
                title: t('common.actions'),
                width: 240,
                fixed: 'right',
                render: (_: unknown, row: TenantRow) => (
                  <Space>
                    <Button
                      type="text"
                      size="mini"
                      onClick={() => {
                        setEditing(row)
                        editForm.setFieldsValue({
                          name: row.name,
                          description: row.description || '',
                          status: row.status || 'active',
                          contactEmail: row.contactEmail || '',
                          maxUserCount: row.maxUserCount || 5,
                        })
                        setEditOpen(true)
                      }}
                    >
                      {t('common.edit')}
                    </Button>
                    <Button
                      type="text"
                      size="mini"
                      onClick={() => navigate(`/tenant-management/${row.id}/ai`)}
                    >
                      {t('tenantManagement.aiConfig')}
                    </Button>
                    <Button
                      type="text"
                      size="mini"
                      status="danger"
                      onClick={() => {
                        Modal.confirm({
                          title: t('tenantManagement.deleteTitle'),
                          content: t('tenantManagement.deleteContent', { name: row.name }),
                          onOk: async () => {
                            const r = await deleteTenantPlatform(row.id)
                            if (r.code !== 200) {
                              Message.error(r.msg || t('common.deleteFailed'))
                              return
                            }
                            Message.success(t('tenantManagement.deleted'))
                            await load()
                          },
                        })
                      }}
                    >
                      {t('common.delete')}
                    </Button>
                  </Space>
                ),
              },
            ]}
          />
        </Space>
      </Card>

      <Modal
        title={t('tenantManagement.modalCreate')}
        style={{ width: 520 }}
        visible={createOpen}
        onCancel={() => setCreateOpen(false)}
        onOk={async () => {
          try {
            const v = await createForm.validate()
            const r = await createTenantPlatform({
              companyName: String(v.companyName || '').trim(),
              adminEmail: String(v.adminEmail || '').trim(),
              adminPassword: String(v.adminPassword || ''),
              adminDisplayName: String(v.adminDisplayName || '').trim(),
              tenantDescription: String(v.tenantDescription || '').trim(),
              maxUserCount: Number(v.maxUserCount) || 5,
            })
            if (r.code !== 200) {
              Message.error(r.msg || t('tenantManagement.createFailed'))
              return
            }
            Message.success(t('tenantManagement.createSuccess'))
            setCreateOpen(false)
            await load()
          } catch {
            /* validate */
          }
        }}
      >
        <Form form={createForm} layout="vertical">
          <FormItem label={t('tenantManagement.formCompany')} field="companyName" rules={[{ required: true }]}>
            <Input placeholder={t('tenantManagement.companyPlaceholder')} />
          </FormItem>
          <FormItem label={t('tenantManagement.formAdminEmail')} field="adminEmail" rules={[{ required: true }]}>
            <Input placeholder={t('tenantManagement.loginEmailPlaceholder')} />
          </FormItem>
          <FormItem label={t('tenantManagement.formAdminPassword')} field="adminPassword" rules={[{ required: true }]}>
            <Input.Password placeholder={t('auth.passwordMin8Short')} />
          </FormItem>
          <FormItem label={t('tenantManagement.formAdminDisplay')} field="adminDisplayName">
            <Input placeholder={t('tenantManagement.optional')} />
          </FormItem>
          <FormItem label={t('tenantManagement.formRemark')} field="tenantDescription">
            <Input.TextArea placeholder={t('tenantManagement.optional')} autoSize={{ minRows: 2 }} />
          </FormItem>
          <FormItem label={t('tenantManagement.formMaxUsers')} field="maxUserCount" initialValue={5}>
            <Input type="number" min={1} />
          </FormItem>
        </Form>
      </Modal>

      <Modal
        title={t('tenantManagement.modalEdit')}
        style={{ width: 480 }}
        visible={editOpen}
        onCancel={() => setEditOpen(false)}
        onOk={async () => {
          if (!editing) return
          try {
            const v = await editForm.validate()
            const r = await updateTenantPlatform(editing.id, {
              name: String(v.name || '').trim(),
              description: String(v.description || '').trim(),
              status: String(v.status || 'active'),
              contactEmail: String(v.contactEmail || '').trim(),
              maxUserCount: Number(v.maxUserCount) || 5,
            })
            if (r.code !== 200) {
              Message.error(r.msg || t('common.saveFailed'))
              return
            }
            Message.success(t('common.saveSuccess'))
            setEditOpen(false)
            await load()
          } catch {
            /* validate */
          }
        }}
      >
        <Form form={editForm} layout="vertical">
          <FormItem label={t('tenantManagement.formCompany')} field="name" rules={[{ required: true }]}>
            <Input />
          </FormItem>
          <FormItem label={t('tenantManagement.formDescription')} field="description">
            <Input.TextArea autoSize={{ minRows: 2 }} />
          </FormItem>
          <FormItem label={t('tenantManagement.formStatus')} field="status" rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'active', label: t('tenantManagement.statusNormal') },
                { value: 'suspended', label: t('tenantManagement.statusPaused') },
              ]}
            />
          </FormItem>
          <FormItem label={t('tenantManagement.formContactEmail')} field="contactEmail">
            <Input />
          </FormItem>
          <FormItem label={t('tenantManagement.formMaxUsers')} field="maxUserCount">
            <Input type="number" min={1} />
          </FormItem>
        </Form>
      </Modal>

    </BaseLayout>
  )
}
