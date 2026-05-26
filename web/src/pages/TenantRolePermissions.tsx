import { useCallback, useEffect, useMemo, useState } from 'react'
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
  Tree,
  Typography,
} from '@arco-design/web-react'
import type { TreeDataType } from '@arco-design/web-react/es/Tree/interface'
import BaseLayout from '@/components/Layout/BaseLayout'
import {
  createOrgRole,
  deleteOrgRole,
  getOrgRole,
  listOrgPermissions,
  listOrgRoles,
  putOrgRolePermissions,
  putOrgTenantUserRoles,
  type OrgPermission,
  type OrgRole,
} from '@/api/tenantOrg'
import { listTenantUsers, type TenantUserRow } from '@/api/tenantUsers'

const FormItem = Form.Item

const KIND_RANK: Record<string, number> = {
  module: 0,
  menu: 1,
  button: 2,
  api: 3,
  data: 4,
}

function kindRank(k?: string): number {
  return KIND_RANK[k || ''] ?? 99
}

function permNodeTitle(p: OrgPermission): string {
  const k = p.kind || ''
  if (k === 'module') return `模块·${p.name}`
  if (k === 'menu') return `页面·${p.name}（菜单）`
  if (k === 'button') return `按钮·${p.name}`
  if (k === 'api') return `接口·${p.name}`
  if (k === 'data') return `数据·${p.name}`
  return p.name
}

function buildPermissionTree(perms: OrgPermission[]): TreeDataType[] {
  const codeSet = new Set(perms.map((p) => p.code))
  const resolvedParent = (p: OrgPermission): string => {
    const pc = (p.parentCode || '').trim()
    if (!pc) return ''
    return codeSet.has(pc) ? pc : ''
  }
  const byParent = new Map<string, OrgPermission[]>()
  for (const p of perms) {
    const par = resolvedParent(p)
    if (!byParent.has(par)) byParent.set(par, [])
    byParent.get(par)!.push(p)
  }
  for (const arr of byParent.values()) {
    arr.sort((a, b) => {
      const d = kindRank(a.kind) - kindRank(b.kind)
      if (d !== 0) return d
      return a.code.localeCompare(b.code)
    })
  }
  const walk = (parentCode: string): TreeDataType[] => {
    const kids = byParent.get(parentCode) || []
    return kids.map((p) => {
      const children = walk(p.code)
      return {
        key: String(p.id),
        title: permNodeTitle(p),
        children: children.length ? children : undefined,
      }
    })
  }
  return walk('')
}

function collectKeysWithChildren(nodes: TreeDataType[]): string[] {
  const out: string[] = []
  const walk = (ns: TreeDataType[]) => {
    for (const n of ns) {
      if (n.children && n.children.length > 0) {
        out.push(String(n.key))
        walk(n.children)
      }
    }
  }
  walk(nodes)
  return out
}

export default function TenantRolePermissions() {
  const [roles, setRoles] = useState<OrgRole[]>([])
  const [perms, setPerms] = useState<OrgPermission[]>([])
  const [users, setUsers] = useState<TenantUserRow[]>([])
  const [loading, setLoading] = useState(false)

  const [roleModalOpen, setRoleModalOpen] = useState(false)
  const [roleForm] = Form.useForm()

  const [permModalOpen, setPermModalOpen] = useState(false)
  const [permRoleId, setPermRoleId] = useState<number | null>(null)
  const [permRoleName, setPermRoleName] = useState('')
  const [permSelected, setPermSelected] = useState<string[]>([])

  const [assignOpen, setAssignOpen] = useState(false)
  const [assignUserId, setAssignUserId] = useState<number | undefined>(undefined)
  const [assignRoleIds, setAssignRoleIds] = useState<number[]>([])

  const loadRoles = useCallback(async () => {
    const res = await listOrgRoles()
    if (res.code === 200 && res.data?.list) setRoles(res.data.list)
  }, [])

  const loadPerms = useCallback(async () => {
    const res = await listOrgPermissions()
    if (res.code === 200 && res.data?.list) setPerms(res.data.list)
  }, [])

  const loadUsers = useCallback(async () => {
    const res = await listTenantUsers(1, 200)
    if (res.code === 200 && res.data?.list) setUsers(res.data.list)
  }, [])

  const refreshAll = useCallback(async () => {
    setLoading(true)
    try {
      await Promise.all([loadRoles(), loadPerms(), loadUsers()])
    } finally {
      setLoading(false)
    }
  }, [loadPerms, loadRoles, loadUsers])

  useEffect(() => {
    void refreshAll()
  }, [refreshAll])

  const treeData = useMemo(() => buildPermissionTree(perms), [perms])
  const defaultExpandedKeys = useMemo(() => collectKeysWithChildren(treeData), [treeData])

  const openEditPerms = async (row: OrgRole) => {
    const res = await getOrgRole(row.id)
    if (res.code !== 200 || !res.data) {
      Message.error(res.msg || '加载失败')
      return
    }
    setPermRoleId(row.id)
    setPermRoleName(row.name)
    const ids = res.data.permissionIds || []
    setPermSelected(ids.map(String))
    setPermModalOpen(true)
  }

  return (
    <BaseLayout title="角色与权限" description="定义租户角色，并按模块树勾选菜单与操作权限">
      <Card bordered={false} loading={loading}>
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <Typography.Title heading={6} style={{ margin: 0 }}>
            角色
          </Typography.Title>
          <Space style={{ marginBottom: 8 }} wrap>
            <Button
              type="primary"
              onClick={() => {
                roleForm.resetFields()
                setRoleModalOpen(true)
              }}
            >
              新建角色
            </Button>
            <Button onClick={() => setAssignOpen(true)}>分配角色给用户</Button>
          </Space>
          <Table
            rowKey="id"
            data={roles}
            pagination={false}
            columns={[
              { title: '名称', dataIndex: 'name' },
              { title: '说明', dataIndex: 'description', ellipsis: true },
              {
                title: '类型',
                render: (_: unknown, row: OrgRole) => (row.isSystem ? '系统' : '自定义'),
              },
              {
                title: '操作',
                width: 220,
                render: (_: unknown, row: OrgRole) => (
                  <Space>
                    <Button
                      type="text"
                      size="mini"
                      disabled={!!row.isSystem}
                      onClick={() => {
                        if (row.isSystem) return
                        void openEditPerms(row)
                      }}
                    >
                      权限
                    </Button>
                    <Button
                      type="text"
                      size="mini"
                      status="danger"
                      disabled={!!row.isSystem}
                      onClick={() => {
                        Modal.confirm({
                          title: '删除角色',
                          content: `确定删除「${row.name}」？`,
                          onOk: async () => {
                            const r = await deleteOrgRole(row.id)
                            if (r.code !== 200) {
                              Message.error(r.msg || '删除失败')
                              return
                            }
                            Message.success('已删除')
                            await loadRoles()
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
        title="新建角色"
        visible={roleModalOpen}
        onCancel={() => setRoleModalOpen(false)}
        onOk={async () => {
          try {
            const v = await roleForm.validate()
            const name = String(v.name || '').trim()
            if (!name) {
              Message.warning('请输入名称')
              return
            }
            const r = await createOrgRole({
              name,
              description: String(v.description || '').trim(),
            })
            if (r.code !== 200) {
              Message.error(r.msg || '创建失败')
              return
            }
            Message.success('已创建')
            setRoleModalOpen(false)
            await loadRoles()
          } catch {
            /* validate */
          }
        }}
      >
        <Form form={roleForm} layout="vertical">
          <FormItem label="名称" field="name" rules={[{ required: true }]}>
            <Input placeholder="角色名称" />
          </FormItem>
          <FormItem label="说明" field="description">
            <Input placeholder="可选" />
          </FormItem>
        </Form>
      </Modal>

      <Modal
        title={permRoleName ? `角色权限 — ${permRoleName}` : '角色权限'}
        style={{ width: 720 }}
        visible={permModalOpen}
        onCancel={() => setPermModalOpen(false)}
        onOk={async () => {
          if (permRoleId == null) return
          const permissionIds = permSelected.map((k) => Number(k)).filter((n) => Number.isFinite(n) && n > 0)
          const r = await putOrgRolePermissions(permRoleId, { permissionIds })
          if (r.code !== 200) {
            Message.error(r.msg || '保存失败')
            return
          }
          Message.success('权限已更新')
          setPermModalOpen(false)
        }}
      >
        <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
          按业务模块展开后勾选页面（菜单）与具体操作。父节点勾选将同步选中其下级能力。
        </Typography.Paragraph>
        <div
          style={{
            maxHeight: 420,
            overflow: 'auto',
            padding: '12px 8px',
            border: '1px solid var(--color-border-2)',
            borderRadius: 4,
            background: 'var(--color-fill-1)',
          }}
        >
          <Tree
            checkable
            blockNode
            treeData={treeData}
            checkedKeys={permSelected}
            defaultExpandedKeys={defaultExpandedKeys}
            onCheck={(keys) => setPermSelected(keys as string[])}
          />
        </div>
      </Modal>

      <Modal
        title="分配角色"
        style={{ width: 560 }}
        visible={assignOpen}
        onCancel={() => setAssignOpen(false)}
        onOk={async () => {
          if (!assignUserId) {
            Message.warning('请选择用户')
            return
          }
          const r = await putOrgTenantUserRoles(assignUserId, { roleIds: assignRoleIds })
          if (r.code !== 200) {
            Message.error(r.msg || '保存失败')
            return
          }
          Message.success('已更新用户角色')
          setAssignOpen(false)
        }}
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <div>
            <div style={{ marginBottom: 8, fontSize: 13 }}>用户</div>
            <Select
              placeholder="选择租户成员"
              style={{ width: '100%' }}
              value={assignUserId}
              onChange={(v) => setAssignUserId(v as number)}
              options={users.map((u) => ({
                value: u.id,
                label: `${u.displayName || u.username || u.email || u.id}`,
              }))}
              showSearch
            />
          </div>
          <div>
            <div style={{ marginBottom: 8, fontSize: 13 }}>角色（多选）</div>
            <Select
              mode="multiple"
              placeholder="选择角色"
              style={{ width: '100%' }}
              value={assignRoleIds}
              onChange={(v) => setAssignRoleIds(v as number[])}
              options={roles.map((r) => ({
                value: r.id,
                label: r.name + (r.isSystem ? '（系统）' : ''),
              }))}
            />
          </div>
        </Space>
      </Modal>
    </BaseLayout>
  )
}
