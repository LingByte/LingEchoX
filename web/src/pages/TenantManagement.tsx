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
  Tabs,
  Typography,
} from '@arco-design/web-react'
import { Link } from 'react-router-dom'
import BaseLayout from '@/components/Layout/BaseLayout'
import {
  createTenantPlatform,
  deleteTenantPlatform,
  getTenant,
  listTenants,
  updateTenantPlatform,
  type TenantRow,
} from '@/api/tenants'
import {
  type AiTab,
  defaultDraft,
  draftToPayload,
  normalizeDraft,
  providerRulesFor,
  ruleFor,
  validateDraft,
} from '@/constants/tenantAiConfigRules'

const FormItem = Form.Item
const TabPane = Tabs.TabPane

export default function TenantManagement() {
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

  const [aiOpen, setAiOpen] = useState(false)
  const [aiInitLoading, setAiInitLoading] = useState(false)
  const [aiSaveLoading, setAiSaveLoading] = useState(false)
  const [aiRow, setAiRow] = useState<TenantRow | null>(null)
  const [aiTab, setAiTab] = useState<AiTab>('asr')
  const [draftAsr, setDraftAsr] = useState<Record<string, unknown>>(() => defaultDraft('asr'))
  const [draftTts, setDraftTts] = useState<Record<string, unknown>>(() => defaultDraft('tts'))
  const [draftLlm, setDraftLlm] = useState<Record<string, unknown>>(() => defaultDraft('llm'))

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await listTenants(page, size, { search: search.trim() || undefined })
      if (res.code === 200 && res.data) {
        setRows(res.data.list || [])
        setTotal(res.data.total ?? 0)
      } else {
        Message.error(res.msg || '加载失败')
      }
    } finally {
      setLoading(false)
    }
  }, [page, search, size])

  useEffect(() => {
    void load()
  }, [load])

  const openAiModal = async (row: TenantRow) => {
    setAiRow(row)
    setAiTab('asr')
    setAiOpen(true)
    setAiInitLoading(true)
    try {
      const r = await getTenant(row.id)
      if (r.code !== 200 || !r.data?.tenant) {
        Message.error(r.msg || '加载租户失败')
        setAiOpen(false)
        return
      }
      const t = r.data.tenant
      setDraftAsr(normalizeDraft('asr', t.asrConfig))
      setDraftTts(normalizeDraft('tts', t.ttsConfig))
      setDraftLlm(normalizeDraft('llm', t.llmConfig))
    } finally {
      setAiInitLoading(false)
    }
  }

  const renderAiFields = (
    tab: AiTab,
    draft: Record<string, unknown>,
    setDraft: (next: Record<string, unknown>) => void,
  ) => {
    const provider = String(draft.provider ?? '')
    const def = ruleFor(tab, provider)
    const opts = providerRulesFor(tab).map((x) => ({ value: x.provider, label: x.label }))
    return (
      <Space direction="vertical" size={14} style={{ width: '100%' }}>
        <div>
          <Typography.Text style={{ display: 'block', marginBottom: 6 }}>厂商（provider）</Typography.Text>
          <Select
            style={{ width: '100%' }}
            value={provider}
            options={opts}
            onChange={(v) => setDraft({ ...defaultDraft(tab), provider: String(v) })}
          />
        </div>
        {def?.fields.map((f) => (
          <div key={f.key}>
            <Typography.Text style={{ display: 'block', marginBottom: 6 }}>
              {f.label}
              {f.required ? ' *' : ''}
            </Typography.Text>
            {f.type === 'password' ? (
              <Input.Password
                autoComplete="new-password"
                placeholder={f.placeholder}
                value={String(draft[f.key] ?? '')}
                onChange={(val) => setDraft({ ...draft, [f.key]: val })}
              />
            ) : f.type === 'number' ? (
              <Input
                type="number"
                placeholder={f.placeholder}
                value={draft[f.key] === undefined || draft[f.key] === '' ? '' : String(draft[f.key])}
                onChange={(val) => setDraft({ ...draft, [f.key]: val })}
              />
            ) : (
              <Input
                placeholder={f.placeholder}
                value={String(draft[f.key] ?? '')}
                onChange={(val) => setDraft({ ...draft, [f.key]: val })}
              />
            )}
          </div>
        ))}
      </Space>
    )
  }

  return (
    <BaseLayout
      title="租户管理"
      description="平台运维：创建企业租户、维护状态与基本信息（需平台管理员登录）。嵌入式 SIP 语音当前仅消费「腾讯云 qcloud」ASR+TTS JSON；其它厂商字段可先存档，后续接管线。"
    >
      <Card bordered={false}>
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
            新建租户将自动创建系统角色「管理员」并绑定当前权限目录中的<strong>全部</strong>能力；该角色的权限不可在租户侧修改。
          </Typography.Paragraph>
          <Space wrap>
            <Input.Search
              placeholder="按名称 / 标识搜索"
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
              新建租户
            </Button>
            <Link to="/sip-trunk-numbers">
              <Button type="outline">去分配中继号码</Button>
            </Link>
          </Space>
          <Table
            rowKey="id"
            loading={loading}
            data={rows}
            pagination={{
              current: page,
              pageSize: size,
              total,
              onChange: (p) => setPage(p),
            }}
            columns={[
              { title: 'ID', dataIndex: 'id', width: 72 },
              { title: '企业名称', dataIndex: 'name' },
              { title: '标识 slug', dataIndex: 'slug', render: (v: string) => <Typography.Text copyable>{v}</Typography.Text> },
              { title: '联系邮箱', dataIndex: 'contactEmail', width: 180, render: (v: string) => v || '—' },
              { title: '人数上限', dataIndex: 'maxUserCount', width: 100, render: (v: number) => (v && v > 0 ? v : 5) },
              {
                title: '状态',
                dataIndex: 'status',
                width: 100,
                render: (v: string) => (v === 'suspended' ? '已暂停' : '正常'),
              },
              {
                title: '操作',
                width: 240,
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
                      编辑
                    </Button>
                    <Button type="text" size="mini" onClick={() => void openAiModal(row)}>
                      AI 配置
                    </Button>
                    <Button
                      type="text"
                      size="mini"
                      status="danger"
                      onClick={() => {
                        Modal.confirm({
                          title: '删除租户',
                          content: `将软删除租户「${row.name}」，其成员将无法登录。确定继续？`,
                          onOk: async () => {
                            const r = await deleteTenantPlatform(row.id)
                            if (r.code !== 200) {
                              Message.error(r.msg || '删除失败')
                              return
                            }
                            Message.success('已删除')
                            await load()
                          },
                        })
                      }}
                    >
                      删除
                    </Button>
                  </Space>
                ),
              },
            ]}
          />
        </Space>
      </Card>

      <Modal
        title="新建租户"
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
              Message.error(r.msg || '创建失败')
              return
            }
            Message.success('租户已创建')
            setCreateOpen(false)
            await load()
          } catch {
            /* validate */
          }
        }}
      >
        <Form form={createForm} layout="vertical">
          <FormItem label="企业名称" field="companyName" rules={[{ required: true }]}>
            <Input placeholder="公司或团队名称" />
          </FormItem>
          <FormItem label="管理员邮箱" field="adminEmail" rules={[{ required: true }]}>
            <Input placeholder="登录邮箱" />
          </FormItem>
          <FormItem label="管理员密码" field="adminPassword" rules={[{ required: true }]}>
            <Input.Password placeholder="至少 8 位" />
          </FormItem>
          <FormItem label="管理员显示名" field="adminDisplayName">
            <Input placeholder="可选" />
          </FormItem>
          <FormItem label="租户备注" field="tenantDescription">
            <Input.TextArea placeholder="可选" autoSize={{ minRows: 2 }} />
          </FormItem>
          <FormItem label="用户上限" field="maxUserCount" initialValue={5}>
            <Input type="number" min={1} />
          </FormItem>
        </Form>
      </Modal>

      <Modal
        title="编辑租户"
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
              Message.error(r.msg || '保存失败')
              return
            }
            Message.success('已保存')
            setEditOpen(false)
            await load()
          } catch {
            /* validate */
          }
        }}
      >
        <Form form={editForm} layout="vertical">
          <FormItem label="企业名称" field="name" rules={[{ required: true }]}>
            <Input />
          </FormItem>
          <FormItem label="备注" field="description">
            <Input.TextArea autoSize={{ minRows: 2 }} />
          </FormItem>
          <FormItem label="状态" field="status" rules={[{ required: true }]}>
            <Select
              options={[
                { value: 'active', label: '正常' },
                { value: 'suspended', label: '暂停' },
              ]}
            />
          </FormItem>
          <FormItem label="官方联系邮箱" field="contactEmail">
            <Input />
          </FormItem>
          <FormItem label="用户上限" field="maxUserCount">
            <Input type="number" min={1} />
          </FormItem>
        </Form>
      </Modal>

      <Modal
        title={aiRow ? `AI 密钥与模型 — ${aiRow.name}` : 'AI 配置'}
        style={{ width: 560 }}
        visible={aiOpen}
        confirmLoading={aiSaveLoading}
        okButtonProps={{ disabled: aiInitLoading }}
        onCancel={() => setAiOpen(false)}
        onOk={async () => {
          if (!aiRow) return
          const e1 = validateDraft('asr', draftAsr)
          const e2 = validateDraft('tts', draftTts)
          const e3 = validateDraft('llm', draftLlm)
          const err = e1 || e2 || e3
          if (err) {
            Message.error(err)
            return
          }
          setAiSaveLoading(true)
          try {
            const r = await updateTenantPlatform(aiRow.id, {
              asrConfig: draftToPayload('asr', draftAsr),
              ttsConfig: draftToPayload('tts', draftTts),
              llmConfig: draftToPayload('llm', draftLlm),
            })
            if (r.code !== 200) {
              Message.error(r.msg || '保存失败')
              return
            }
            Message.success('已保存 AI 配置')
            setAiOpen(false)
            await load()
          } finally {
            setAiSaveLoading(false)
          }
        }}
      >
        {aiInitLoading ? (
          <Typography.Paragraph style={{ margin: 24, textAlign: 'center' }}>加载租户配置…</Typography.Paragraph>
        ) : (
          <Tabs activeTab={aiTab} onChange={(k) => setAiTab(k as AiTab)}>
            <TabPane key="asr" title="ASR">
              {renderAiFields('asr', draftAsr, setDraftAsr)}
            </TabPane>
            <TabPane key="tts" title="TTS">
              {renderAiFields('tts', draftTts, setDraftTts)}
            </TabPane>
            <TabPane key="llm" title="LLM">
              {renderAiFields('llm', draftLlm, setDraftLlm)}
            </TabPane>
          </Tabs>
        )}
      </Modal>
    </BaseLayout>
  )
}
