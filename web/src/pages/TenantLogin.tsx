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
      subtitle="使用组织标识与账号邮箱登录控制台。"
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
              tenantSlug: String(v.tenantSlug || '').trim(),
              email: String(v.email || '').trim(),
              password: String(v.password || ''),
            })
            if (res.code !== 200 || !res.data?.token) {
              Message.error(res.msg || '登录失败')
              return
            }
            const { token, user, tenant } = res.data
            await login(token, {
              ...user,
              tenantSlug: tenant?.slug,
              tenantName: tenant?.name,
            })
            Message.success('登录成功')
            navigate('/overview', { replace: true })
          } catch (e: unknown) {
            const msg = typeof e === 'object' && e && 'msg' in e ? String((e as { msg?: string }).msg) : '登录失败'
            Message.error(msg)
          }
        }}
      >
        <FormItem
          label="组织标识"
          field="tenantSlug"
          rules={[{ required: true, message: '请输入组织标识（slug）' }]}
        >
          <Input placeholder="例如 acme-corp" autoComplete="organization" />
        </FormItem>
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
        组织标识即注册时填写的 slug，用于区分不同企业空间。
        <br />
        <IconLock style={{ marginRight: 6, verticalAlign: -2 }} />
        令牌仅保存在本机浏览器，请勿在公共设备勾选「记住密码」类插件写入明文。
      </div>
    </AuthShell>
  )
}
