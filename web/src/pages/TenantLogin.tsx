import { Form, Input, Button, Message } from '@arco-design/web-react'
import { IconLock, IconUser } from '@arco-design/web-react/icon'
import { Link, useNavigate } from 'react-router-dom'
import AuthShell from '@/components/Auth/AuthShell'
import { tenantLogin } from '@/api/tenantAuth'
import { useAuthStore } from '@/stores/authStore'

const FormItem = Form.Item

export default function TenantLogin() {
  const [form] = Form.useForm()
  const navigate = useNavigate()
  const login = useAuthStore((s) => s.login)

  return (
    <AuthShell
      title="租户登录"
      subtitle="邮箱在系统内全局唯一，使用邮箱与密码登录即可。"
      footer={
        <div style={{ textAlign: 'center', fontSize: 13, color: 'var(--color-text-3)' }}>
          还没有组织？
          <Link to="/register" style={{ marginLeft: 6, color: 'rgb(var(--primary-6))' }}>
            注册企业
          </Link>
        </div>
      }
    >
      <Form
        form={form}
        layout="vertical"
        requiredSymbol={false}
        onSubmit={async (v) => {
          try {
            const res = await tenantLogin({
              email: String(v.email || '').trim(),
              password: String(v.password || ''),
            })
            if (res.code !== 200 || !res.data?.token) {
              Message.error(res.msg || '登录失败')
              return
            }
            const d = res.data
            if (d.principal === 'platform' && d.platformAdmin) {
              const a = d.platformAdmin
              await login(d.token, {
                id: a.id,
                email: a.email,
                displayName: a.displayName,
                isPlatformAdmin: true,
                principal: 'platform' as const,
              })
            } else if (d.principal === 'tenant' && d.user && d.tenant) {
              const { token, user, tenant } = d
              await login(token, {
                ...user,
                tenantSlug: tenant.slug,
                tenantName: tenant.name,
                principal: 'tenant' as const,
              })
            } else {
              Message.error('登录响应无效')
              return
            }
            Message.success('登录成功')
            navigate('/overview', { replace: true })
          } catch (e: unknown) {
            const msg = typeof e === 'object' && e && 'msg' in e ? String((e as { msg?: string }).msg) : '登录失败'
            Message.error(msg)
          }
        }}
      >
        <FormItem label="邮箱" field="email" rules={[{ required: true, message: '请输入邮箱' }]}>
          <Input placeholder="name@company.com" autoComplete="email" />
        </FormItem>
        <FormItem label="密码" field="password" rules={[{ required: true, message: '请输入密码' }]}>
          <Input.Password placeholder="登录密码" autoComplete="current-password" />
        </FormItem>
        <FormItem style={{ marginBottom: 0 }}>
          <Button type="primary" htmlType="submit" long style={{ height: 40, marginTop: 4 }}>
            登录
          </Button>
        </FormItem>
      </Form>
      <div style={{ marginTop: 20, fontSize: 12, color: 'var(--color-text-4)', lineHeight: 1.6 }}>
        <IconUser style={{ marginRight: 6, verticalAlign: -2 }} />
        一家企业注册成功后，系统将自动为新组织生成唯一 slug（由公司名拼音/英文与随机后缀组成）。
        <br />
        <IconLock style={{ marginRight: 6, verticalAlign: -2 }} />
        令牌仅保存在本机浏览器，请勿在公共设备勾选「记住密码」类插件写入明文。
      </div>
    </AuthShell>
  )
}
