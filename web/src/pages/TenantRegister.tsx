import { Form, Input, Button, Message } from '@arco-design/web-react'
import { Link, useNavigate } from 'react-router-dom'
import AuthShell from '@/components/Auth/AuthShell'
import { registerTenant } from '@/api/tenantAuth'
import { useAuthStore } from '@/stores/authStore'
import { useTranslation } from '@/i18n'

const FormItem = Form.Item

export default function TenantRegister() {
  const { t } = useTranslation()
  const [form] = Form.useForm()
  const navigate = useNavigate()
  const login = useAuthStore((s) => s.login)

  return (
    <AuthShell
      title={t('auth.tenantRegister')}
      subtitle={t('auth.registerSubtitle')}
      footer={
        <div style={{ textAlign: 'center', fontSize: 13, color: 'var(--color-text-3)' }}>
          {t('auth.hasAccount')}
          <Link to="/login" style={{ marginLeft: 6, color: 'rgb(var(--primary-6))' }}>
            {t('auth.goLogin')}
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
              Message.error(res.msg || t('auth.registerFailed'))
              return
            }
            const d = res.data
            if (!d.token || d.principal !== 'tenant' || !d.user || !d.tenant) {
              Message.error(t('auth.registerInvalidResponse'))
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
            Message.success(t('auth.registerSuccess'))
            navigate('/overview', { replace: true })
          } catch (e: unknown) {
            const msg =
              typeof e === 'object' && e && 'msg' in e
                ? String((e as { msg?: string }).msg)
                : t('auth.registerFailed')
            Message.error(msg)
          }
        }}
      >
        <FormItem
          label={t('auth.companyNameLabel')}
          field="companyName"
          rules={[{ required: true, message: t('auth.companyNameRequired') }]}
        >
          <Input placeholder={t('auth.companyNamePlaceholder')} autoComplete="organization" />
        </FormItem>
        <FormItem label={t('auth.adminEmail')} field="adminEmail" rules={[{ required: true, message: t('auth.email') }]}>
          <Input placeholder={t('auth.adminEmailPlaceholder')} autoComplete="email" />
        </FormItem>
        <FormItem
          label={t('auth.adminPasswordLabel')}
          field="adminPassword"
          rules={[
            { required: true, message: t('auth.passwordMin8') },
            { minLength: 8, message: t('auth.passwordMin8Short') },
          ]}
        >
          <Input.Password placeholder={t('auth.passwordPlaceholder')} autoComplete="new-password" />
        </FormItem>
        <FormItem label={t('auth.displayNameOptional')} field="adminDisplayName">
          <Input placeholder={t('auth.displayNamePlaceholder')} autoComplete="name" />
        </FormItem>
        <FormItem style={{ marginBottom: 0 }}>
          <Button type="primary" htmlType="submit" long style={{ height: 40, marginTop: 4 }}>
            {t('auth.registerSubmit')}
          </Button>
        </FormItem>
      </Form>
    </AuthShell>
  )
}
