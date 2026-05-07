import { Form, Input, Button, Message } from '@arco-design/web-react'
import { Link, useNavigate } from 'react-router-dom'
import AuthShell from '@/components/Auth/AuthShell'
import { registerTenant } from '@/api/tenantAuth'
import { useAuthStore } from '@/stores/authStore'

const FormItem = Form.Item

export default function TenantRegister() {
  const [form] = Form.useForm()
  const navigate = useNavigate()
  const login = useAuthStore((s) => s.login)

  return (
    <AuthShell
      title="企业注册"
      subtitle="创建组织后将自动为您开通「管理员」角色，并登录控制台。"
      footer={
        <div style={{ textAlign: 'center', fontSize: 13, color: 'var(--color-text-3)' }}>
          已有账号？
          <Link to="/login" style={{ marginLeft: 6, color: 'rgb(var(--primary-6))' }}>
            前往登录
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
            const res = await registerTenant({
              companyName: String(v.companyName || '').trim(),
              adminEmail: String(v.adminEmail || '').trim(),
              adminPassword: String(v.adminPassword || ''),
              adminDisplayName: String(v.adminDisplayName || '').trim() || undefined,
            })
            if (res.code !== 200 || !res.data?.token) {
              Message.error(res.msg || '注册失败')
              return
            }
            const d = res.data
            if (!d.token || d.principal !== 'tenant' || !d.user || !d.tenant) {
              Message.error('注册响应无效')
              return
            }
            const { token, user, tenant } = d
            await login(token, {
              ...user,
              tenantSlug: tenant.slug,
              tenantName: tenant.name,
              principal: 'tenant' as const,
              permissionCodes: d.permissionCodes ?? [],
            })
            Message.success('注册成功')
            navigate('/overview', { replace: true })
          } catch (e: unknown) {
            const msg = typeof e === 'object' && e && 'msg' in e ? String((e as { msg?: string }).msg) : '注册失败'
            Message.error(msg)
          }
        }}
      >
        <FormItem
          label="企业 / 组织名称"
          field="companyName"
          rules={[{ required: true, message: '请输入组织名称' }]}
        >
          <Input placeholder="例如 杭州某某科技有限公司" autoComplete="organization" />
        </FormItem>
        <FormItem label="管理员邮箱" field="adminEmail" rules={[{ required: true, message: '请输入邮箱' }]}>
          <Input placeholder="管理员登录邮箱" autoComplete="email" />
        </FormItem>
        <FormItem
          label="登录密码"
          field="adminPassword"
          rules={[{ required: true, message: '至少 8 位密码' }, { minLength: 8, message: '至少 8 位' }]}
        >
          <Input.Password placeholder="不少于 8 位" autoComplete="new-password" />
        </FormItem>
        <FormItem label="管理员显示名（可选）" field="adminDisplayName">
          <Input placeholder="默认为邮箱前缀" autoComplete="name" />
        </FormItem>
        <FormItem style={{ marginBottom: 0 }}>
          <Button type="primary" htmlType="submit" long style={{ height: 40, marginTop: 4 }}>
            创建组织并登录
          </Button>
        </FormItem>
      </Form>
    </AuthShell>
  )
}
