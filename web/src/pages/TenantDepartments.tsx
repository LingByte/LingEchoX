import { useCallback, useEffect, useState } from 'react'
import { Button, Card, Form, Input, Message, Modal, Select, Space, Switch, Table, Typography } from '@arco-design/web-react'
import BaseLayout from '@/components/Layout/BaseLayout'
import {
  createOrgGroup,
  deleteOrgGroup,
  listOrgGroups,
  putOrgTenantUserGroups,
  updateOrgGroup,
  type OrgGroup,
} from '@/api/tenantOrg'
import { listTenantUsers, type TenantUserRow } from '@/api/tenantUsers'

const FormItem = Form.Item

export default function TenantDepartments() {
  const [groups, setGroups] = useState<OrgGroup[]>([])
  const [users, setUsers] = useState<TenantUserRow[]>([])
  const [loading, setLoading] = useState(false)

  const [groupModalOpen, setGroupModalOpen] = useState(false)
  const [groupEditing, setGroupEditing] = useState<OrgGroup | null>(null)
  const [groupForm] = Form.useForm()

  const [assignGroupOpen, setAssignGroupOpen] = useState(false)
  const [assignGroupUserId, setAssignGroupUserId] = useState<number | undefined>(undefined)
  const [assignGroupIds, setAssignGroupIds] = useState<number[]>([])

  const loadGroups = useCallback(async () => {
    const res = await listOrgGroups()
    if (res.code === 200 && res.data?.list) setGroups(res.data.list)
  }, [])

  const loadUsers = useCallback(async () => {
    const res = await listTenantUsers(1, 200)
    if (res.code === 200 && res.data?.list) setUsers(res.data.list)
  }, [])

  const refreshAll = useCallback(async () => {
    setLoading(true)
    try {
      await Promise.all([loadGroups(), loadUsers()])
    } finally {
      setLoading(false)
    }
  }, [loadGroups, loadUsers])

  useEffect(() => {
    void refreshAll()
  }, [refreshAll])

  return (
    <BaseLayout title="部门" description="维护租户部门结构，并为成员分配所属部门">
      <Card bordered={false} loading={loading}>
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <Typography.Title heading={6} style={{ margin: 0 }}>
            部门列表
          </Typography.Title>
          <Space style={{ marginBottom: 8 }} wrap>
            <Button
              type="primary"
              onClick={() => {
                setGroupEditing(null)
                groupForm.resetFields()
                groupForm.setFieldsValue({ name: '', isDefault: false })
                setGroupModalOpen(true)
              }}
            >
              新建部门
            </Button>
            <Button
              onClick={() => {
                setAssignGroupUserId(undefined)
                setAssignGroupIds([])
                setAssignGroupOpen(true)
              }}
            >
              分配部门
            </Button>
          </Space>
          <Table
            rowKey="id"
            data={groups}
            pagination={false}
            columns={[
              { title: 'ID', dataIndex: 'id', width: 72 },
              { title: '名称', dataIndex: 'name' },
              {
                title: '默认部门',
                dataIndex: 'isDefault',
                render: (v: boolean) => (v ? '是' : '否'),
              },
              {
                title: '操作',
                width: 200,
                render: (_: unknown, row: OrgGroup) => (
                  <Space>
                    <Button
                      type="text"
                      size="mini"
                      onClick={() => {
                        setGroupEditing(row)
                        groupForm.setFieldsValue({ name: row.name, isDefault: !!row.isDefault })
                        setGroupModalOpen(true)
                      }}
                    >
                      编辑
                    </Button>
                    <Button
                      type="text"
                      size="mini"
                      status="danger"
                      onClick={() => {
                        Modal.confirm({
                          title: '删除部门',
                          content: `确定删除「${row.name}」？`,
                          onOk: async () => {
                            const r = await deleteOrgGroup(row.id)
                            if (r.code !== 200) {
                              Message.error(r.msg || '删除失败')
                              return
                            }
                            Message.success('已删除')
                            await loadGroups()
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
        title={groupEditing ? '编辑部门' : '新建部门'}
        visible={groupModalOpen}
        onCancel={() => setGroupModalOpen(false)}
        onOk={async () => {
          try {
            const v = await groupForm.validate()
            const name = String(v.name || '').trim()
            if (!name) {
              Message.warning('请输入名称')
              return
            }
            if (groupEditing) {
              const r = await updateOrgGroup(groupEditing.id, {
                name,
                isDefault: !!v.isDefault,
              })
              if (r.code !== 200) {
                Message.error(r.msg || '保存失败')
                return
              }
            } else {
              const r = await createOrgGroup({ name, isDefault: !!v.isDefault })
              if (r.code !== 200) {
                Message.error(r.msg || '创建失败')
                return
              }
            }
            Message.success('已保存')
            setGroupModalOpen(false)
            await loadGroups()
          } catch {
            /* validate */
          }
        }}
      >
        <Form form={groupForm} layout="vertical">
          <FormItem label="名称" field="name" rules={[{ required: true }]}>
            <Input placeholder="部门名称" />
          </FormItem>
          <FormItem label="设为默认部门" field="isDefault" triggerPropName="checked">
            <Switch />
          </FormItem>
        </Form>
      </Modal>

      <Modal
        title="分配部门"
        style={{ width: 560 }}
        visible={assignGroupOpen}
        onCancel={() => setAssignGroupOpen(false)}
        onOk={async () => {
          if (!assignGroupUserId) {
            Message.warning('请选择用户')
            return
          }
          const r = await putOrgTenantUserGroups(assignGroupUserId, { groupIds: assignGroupIds })
          if (r.code !== 200) {
            Message.error(r.msg || '保存失败')
            return
          }
          Message.success('已更新用户部门')
          setAssignGroupOpen(false)
        }}
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <div>
            <div style={{ marginBottom: 8, fontSize: 13 }}>用户</div>
            <Select
              placeholder="选择租户成员"
              style={{ width: '100%' }}
              value={assignGroupUserId}
              onChange={(v) => setAssignGroupUserId(v as number)}
              options={users.map((u) => ({
                value: u.id,
                label: `${u.displayName || u.username || u.email || u.id}`,
              }))}
              showSearch
            />
          </div>
          <div>
            <div style={{ marginBottom: 8, fontSize: 13 }}>部门（多选）</div>
            <Select
              mode="multiple"
              placeholder="选择部门"
              style={{ width: '100%' }}
              value={assignGroupIds}
              onChange={(v) => setAssignGroupIds(v as number[])}
              options={groups.map((g) => ({
                value: g.id,
                label: g.name,
              }))}
            />
          </div>
        </Space>
      </Modal>
    </BaseLayout>
  )
}
