import { useEffect, useState } from 'react'
import { Button, Card, Descriptions, Form, Input, Message, Space, Tabs } from '@arco-design/web-react'
import { useNavigate } from 'react-router-dom'
import BaseLayout from '@/components/Layout/BaseLayout'
import { fetchMe, updateMe as updateMeApi, updateMyPassword, logoutApi } from '@/api/me'
import { useAuthStore } from '@/stores/authStore'

const TabPane = Tabs.TabPane
const FormItem = Form.Item

export default function Profile() {
  const [loading, setLoading] = useState(false)
  const [me, setMe] = useState<any>(null)
  const [profileForm] = Form.useForm()
  const [pwdForm] = Form.useForm()
  const updateLocalProfile = useAuthStore((s) => s.updateProfile)
  const clearUser = useAuthStore((s) => s.clearUser)
  const navigate = useNavigate()

  const loadMe = async () => {
    setLoading(true)
    try {
      const res = await fetchMe()
      if (res.code !== 200 || !res.data) {
        Message.error(res.msg || '加载失败')
        return
      }
      setMe(res.data)
      const d = res.data
      if (d.principal === 'platform' && d.platformAdmin) {
        profileForm.setFieldsValue({
          displayName: d.platformAdmin.displayName || '',
          username: '',
          phone: '',
        })
        updateLocalProfile({
          id: d.platformAdmin.id,
          email: d.platformAdmin.email,
          displayName: d.platformAdmin.displayName,
          isPlatformAdmin: true,
          principal: 'platform',
        })
      } else if (d.principal === 'tenant' && d.user) {
        profileForm.setFieldsValue({
          displayName: d.user.displayName || '',
          username: d.user.username || '',
          phone: d.user.phone || '',
        })
        updateLocalProfile({
          ...d.user,
          tenantSlug: d.tenant?.slug,
          tenantName: d.tenant?.name,
          principal: 'tenant',
        })
      }
    } catch (e: any) {
      Message.error(e?.msg || '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadMe()
  }, [])

  const isPlatform = me?.principal === 'platform'

  return (
    <BaseLayout title="个人中心" description={isPlatform ? '平台管理员账号' : '查看并维护您的账号信息'}>
      <Tabs defaultActiveTab="profile">
        <TabPane key="profile" title="资料信息">
          <Card loading={loading}>
            {isPlatform ? (
              <>
                <Descriptions
                  column={1}
                  style={{ marginBottom: 18 }}
                  data={[
                    { label: '角色', value: '平台管理员' },
                    { label: '邮箱', value: me?.platformAdmin?.email || '-' },
                  ]}
                />
                <Form
                  form={profileForm}
                  layout="vertical"
                  requiredSymbol={false}
                  onSubmit={async (v) => {
                    try {
                      const res = await updateMeApi({
                        displayName: String(v.displayName || '').trim(),
                      })
                      if (res.code !== 200 || !res.data) {
                        Message.error(res.msg || '更新失败')
                        return
                      }
                      Message.success('资料已更新')
                      const pdata = res.data as { id?: number; email?: string; displayName?: string }
                      updateLocalProfile({
                        ...pdata,
                        isPlatformAdmin: true,
                        principal: 'platform',
                      })
                      await loadMe()
                    } catch (e: any) {
                      Message.error(e?.msg || '更新失败')
                    }
                  }}
                >
                  <FormItem label="显示名" field="displayName">
                    <Input placeholder="显示名" />
                  </FormItem>
                  <Button type="primary" htmlType="submit">
                    保存资料
                  </Button>
                </Form>
              </>
            ) : (
              <>
                <Descriptions
                  column={1}
                  style={{ marginBottom: 18 }}
                  data={[
                    { label: '组织', value: me?.tenant?.name || '-' },
                    { label: '组织标识', value: me?.tenant?.slug || '-' },
                    { label: '账号邮箱', value: me?.user?.email || '-' },
                  ]}
                />
                <Form
                  form={profileForm}
                  layout="vertical"
                  requiredSymbol={false}
                  onSubmit={async (v) => {
                    try {
                      const res = await updateMeApi({
                        displayName: String(v.displayName || '').trim(),
                        username: String(v.username || '').trim(),
                        phone: String(v.phone || '').trim(),
                      })
                      if (res.code !== 200 || !res.data) {
                        Message.error(res.msg || '更新失败')
                        return
                      }
                      Message.success('资料已更新')
                      updateLocalProfile(res.data as never)
                      await loadMe()
                    } catch (e: any) {
                      Message.error(e?.msg || '更新失败')
                    }
                  }}
                >
                  <FormItem label="显示名" field="displayName">
                    <Input placeholder="请输入显示名" />
                  </FormItem>
                  <FormItem label="用户名" field="username">
                    <Input placeholder="请输入用户名" />
                  </FormItem>
                  <FormItem label="手机号" field="phone">
                    <Input placeholder="请输入手机号" />
                  </FormItem>
                  <Button type="primary" htmlType="submit">
                    保存资料
                  </Button>
                </Form>
              </>
            )}
          </Card>
        </TabPane>

        <TabPane key="password" title="修改密码">
          <Card>
            <Form
              form={pwdForm}
              layout="vertical"
              requiredSymbol={false}
              onSubmit={async (v) => {
                if (String(v.newPassword || '') !== String(v.confirmPassword || '')) {
                  Message.error('两次输入的新密码不一致')
                  return
                }
                try {
                  const res = await updateMyPassword({
                    oldPassword: String(v.oldPassword || ''),
                    newPassword: String(v.newPassword || ''),
                  })
                  if (res.code !== 200) {
                    Message.error(res.msg || '修改失败')
                    return
                  }
                  Message.success('密码修改成功，请重新登录')
                  clearUser()
                  navigate('/login', { replace: true })
                } catch (e: any) {
                  Message.error(e?.msg || '修改失败')
                }
              }}
            >
              <FormItem label="旧密码" field="oldPassword" rules={[{ required: true, message: '请输入旧密码' }]}>
                <Input.Password />
              </FormItem>
              <FormItem
                label="新密码"
                field="newPassword"
                rules={[{ required: true, message: '请输入新密码' }, { minLength: 8, message: '至少8位' }]}
              >
                <Input.Password />
              </FormItem>
              <FormItem
                label="确认新密码"
                field="confirmPassword"
                rules={[{ required: true, message: '请再次输入新密码' }]}
              >
                <Input.Password />
              </FormItem>
              <Button type="primary" htmlType="submit">
                更新密码
              </Button>
            </Form>
          </Card>
        </TabPane>
      </Tabs>

      <Card style={{ marginTop: 16 }}>
        <Space>
          <Button
            status="warning"
            onClick={async () => {
              try {
                await logoutApi()
              } finally {
                clearUser()
                navigate('/login', { replace: true })
              }
            }}
          >
            退出登录
          </Button>
        </Space>
      </Card>
    </BaseLayout>
  )
}
