import { useState } from 'react'
import { Form, Input, Button, Message } from '@arco-design/web-react'
import { Link, useNavigate } from 'react-router-dom'
import AuthShell from '@/components/Auth/AuthShell'
import { tenantLogin } from '@/api/tenantAuth'
import { useAuthStore } from '@/stores/authStore'
import { useTranslation } from '@/i18n'

const FormItem = Form.Item

export default function TenantLogin() {
  const { t } = useTranslation()
  const [form] = Form.useForm()
  const navigate = useNavigate()
  const login = useAuthStore((s) => s.login)
  const [needsTotp, setNeedsTotp] = useState(false)

  return (
    <AuthShell
      title={t('auth.tenantLogin')}
      footer={
        <div style={{ textAlign: 'center', fontSize: 13, color: 'var(--color-text-3)' }}>
          {t('auth.noOrgYet')}
          <Link to="/register" style={{ marginLeft: 6, color: 'rgb(var(--primary-6))' }}>
            {t('auth.registerOrg')}
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
                Message.warning(t('auth.needsTotp'))
                return
              }
              Message.error(res.msg || t('auth.loginFailed'))
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
              Message.error(t('auth.invalidResponse'))
              return
            }
            Message.success(t('auth.loginSuccess'))
            navigate(d.principal === 'platform' ? '/sip-users' : '/overview', { replace: true })
          } catch (e: unknown) {
            const msg =
              typeof e === 'object' && e && 'msg' in e ? String((e as { msg?: string }).msg) : t('auth.loginFailed')
            Message.error(msg)
          }
        }}
      >
        <FormItem label={t('auth.email')} field="email" rules={[{ required: true, message: t('auth.email') }]}>
          <Input placeholder="name@company.com" />
        </FormItem>
        <FormItem label={t('auth.password')} field="password" rules={[{ required: true, message: t('auth.password') }]}>
          <Input.Password />
        </FormItem>
        {needsTotp && (
          <FormItem label={t('auth.totpCode')} field="totpCode" rules={[{ required: true, message: t('auth.totpCode') }]}>
            <Input maxLength={12} placeholder={t('auth.totpPlaceholder')} />
          </FormItem>
        )}
        <Button type="primary" htmlType="submit" long style={{ marginTop: 8 }}>
          {t('auth.login')}
        </Button>
      </Form>
    </AuthShell>
  )
}
