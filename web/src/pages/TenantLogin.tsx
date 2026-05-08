import { useState } from 'react'
import { Form, Input, Button, Message } from '@arco-design/web-react'
import { Link, useNavigate } from 'react-router-dom'
import AuthShell from '@/components/Auth/AuthShell'
import { tenantLogin } from '@/api/tenantAuth'
import { useAuthStore } from '@/stores/authStore'

const FormItem = Form.Item

export default function TenantLogin() {
  const [form] = Form.useForm()
  const navigate = useNavigate()
  const login = useAuthStore((s) => s.login)
  const [needsTotp, setNeedsTotp] = useState(false)

  return (
    <AuthShell
      title="租户登录"
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
        onValuesChange={(patch) => {
          if (patch.email !== undefined || patch.password !== undefined) {
            setNeedsTotp(false)
          }
        }}
        onSubmit={async (v) => {
          try {
            const res = await tenantLogin({
              email: String(v.email || '').trim(),
              password: String(v.password || ''),
              totpCode: needsTotp ? String(v.totpCode || '').trim() : undefined,
            })
            if (res.code !== 200 || !res.data?.token) {
              const extra = res.data as { needsTotp?: boolean } | undefined
              if (extra?.needsTotp) {
                setNeedsTotp(true)
                Message.warning('请输入验证器中的 6 位动态码')
                return
              }
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
                permissionCodes: d.permissionCodes ?? [],
              })
            } else {
              Message.error('登录响应无效')
              return
            }
            Message.success('登录成功')
            navigate(d.principal === 'platform' ? '/sip-users' : '/overview', { replace: true })
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
        {needsTotp && (
          <FormItem
            label="两步验证码"
            field="totpCode"
            rules={[{ required: true, message: '请输入 6 位动态码' }]}
          >
            <Input placeholder="打开验证器 App，输入 6 位数字" autoComplete="one-time-code" maxLength={12} />
          </FormItem>
        )}
        <FormItem style={{ marginBottom: 0 }}>
          <Button type="primary" htmlType="submit" long style={{ height: 40, marginTop: 4 }}>
            {needsTotp ? '验证并登录' : '登录'}
          </Button>
        </FormItem>
      </Form>
    </AuthShell>
  )
}
