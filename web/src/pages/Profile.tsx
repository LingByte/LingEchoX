import { useEffect, useMemo, useState } from 'react'
import {
  Avatar,
  Button,
  Card,
  Form,
  Input,
  Menu,
  Message,
  Space,
  Tag,
  Upload,
} from '@arco-design/web-react'
import dayjs from 'dayjs'
import { UserCircle } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import BaseLayout from '@/components/Layout/BaseLayout'
import {
  disableTotp,
  enableTotp,
  fetchMe,
  setupTotp,
  updateMe as updateMeApi,
  updateMyPassword,
  uploadMyAvatar,
  logoutApi,
} from '@/api/me'
import { useAuthStore } from '@/stores/authStore'

const FormItem = Form.Item

type ProfileSection = 'profile' | 'security' | 'password'

function fmtLastLogin(iso?: string) {
  if (!iso) return '-'
  const d = dayjs(iso)
  return d.isValid() ? d.format('YYYY-MM-DD HH:mm:ss') : '-'
}

function fmtStatus(s?: string) {
  const v = String(s || '').toLowerCase()
  if (v === 'active') return '正常'
  if (v === 'disabled') return '已停用'
  if (v === 'pending') return '待激活'
  return s || '-'
}

export default function Profile() {
  const [loading, setLoading] = useState(false)
  const [me, setMe] = useState<any>(null)
  const [activeSection, setActiveSection] = useState<ProfileSection>('profile')
  const [profileForm] = Form.useForm()
  const [pwdForm] = Form.useForm()
  const [totpEnableForm] = Form.useForm()
  const [totpDisableForm] = Form.useForm()
  const [totpDraft, setTotpDraft] = useState<{ secret: string; qrDataUrl: string } | null>(null)
  const [editingProfile, setEditingProfile] = useState(false)
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

  const navItems = useMemo(() => {
    const items: { key: ProfileSection; label: string }[] = [
      { key: 'profile', label: '资料信息' },
    ]
    items.push({ key: 'security', label: '安全 · 两步验证' })
    items.push({ key: 'password', label: '修改密码' })
    return items
  }, [isPlatform])

  const resetProfileFormFromMe = () => {
    if (isPlatform && me?.platformAdmin) {
      profileForm.setFieldsValue({
        displayName: me.platformAdmin.displayName || '',
        username: '',
        phone: '',
      })
      return
    }
    if (me?.principal === 'tenant' && me?.user) {
      profileForm.setFieldsValue({
        displayName: me.user.displayName || '',
        username: me.user.username || '',
        phone: me.user.phone || '',
      })
    }
  }

  const heroTitle = () => {
    if (isPlatform) return String(me?.platformAdmin?.displayName || me?.platformAdmin?.email || '管理员')
    const u = me?.user
    return String(u?.displayName?.trim() || u?.username?.trim() || u?.email || '用户')
  }

  const heroSubtitle = () => {
    if (isPlatform) return String(me?.platformAdmin?.email || '')
    return String(me?.user?.email || '')
  }

  const avatarSrc = () => (!isPlatform ? String(me?.user?.avatarUrl || '').trim() : '')

  const detailGridStyle = {
    display: 'grid',
    gridTemplateColumns: 'minmax(88px, auto) 1fr minmax(88px, auto) 1fr',
    columnGap: 16,
    rowGap: 14,
    alignItems: 'center' as const,
  }

  const profilePanel = (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <Card
        loading={loading}
        bordered={false}
        className="overflow-hidden rounded-xl border border-border bg-gradient-to-br from-primary/10 via-card to-card"
        bodyStyle={{ padding: 20 }}
      >
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 20, alignItems: 'flex-start' }}>
          <Avatar size={72} className="shrink-0 bg-muted">
            {!isPlatform && avatarSrc() ? (
              <img alt="" src={avatarSrc()} className="h-full w-full object-cover" />
            ) : (
              <UserCircle size={40} strokeWidth={1.5} className="text-muted-foreground" />
            )}
          </Avatar>
          <div style={{ flex: '1 1 220px', minWidth: 0 }}>
            <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 10 }}>
              <span className="text-[22px] font-bold text-foreground">{heroTitle()}</span>
              {isPlatform ? (
                <Tag color="orangered">平台管理员</Tag>
              ) : (
                <Tag color="arcoblue">租户成员</Tag>
              )}
              <Tag color={isPlatform || me?.user?.status === 'active' ? 'green' : 'gray'}>
                {isPlatform ? '活跃' : fmtStatus(me?.user?.status)}
              </Tag>
            </div>
            <div className="mt-2 break-all text-sm text-muted-foreground">{heroSubtitle()}</div>
            <Space style={{ marginTop: 12 }} wrap>
              <Tag color="green" size="small">
                邮箱已验证
              </Tag>
              {!isPlatform && me?.user?.phone ? (
                <Tag color="green" size="small">
                  手机已登记
                </Tag>
              ) : (
                !isPlatform && (
                  <Tag size="small" className="text-muted-foreground">
                    手机未登记
                  </Tag>
                )
              )}
              {!isPlatform && (
                <Upload
                  accept="image/png,image/jpeg,image/gif,image/webp"
                  showUploadList={false}
                  beforeUpload={async (file: File) => {
                    try {
                      const res = await uploadMyAvatar(file)
                      if (res.code !== 200 || !res.data?.user) {
                        Message.error(res.msg || '上传失败')
                        return false
                      }
                      Message.success('头像已更新')
                      updateLocalProfile(res.data.user as never)
                      await loadMe()
                    } catch (e: any) {
                      Message.error(e?.msg || '上传失败')
                    }
                    return false
                  }}
                >
                  <Button size="mini" type="outline">
                    更换头像
                  </Button>
                </Upload>
              )}
            </Space>
          </div>
        </div>
      </Card>

      <Card title="账号详情" bordered={false} className="rounded-xl border border-border"
        extra={
          <Button
            type="text"
            size="small"
            onClick={() => {
              if (editingProfile) {
                resetProfileFormFromMe()
                setEditingProfile(false)
              } else {
                setEditingProfile(true)
              }
            }}
          >
            {editingProfile ? '取消' : '编辑'}
          </Button>
        }
      >
        {isPlatform ? (
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
                setEditingProfile(false)
                await loadMe()
              } catch (e: any) {
                Message.error(e?.msg || '更新失败')
              }
            }}
          >
            <div style={detailGridStyle}>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">账号 ID</div>
              <div className="text-[13px] text-foreground break-words">{me?.platformAdmin?.id ?? '—'}</div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">邮箱</div>
              <div className="text-[13px] text-foreground break-words">{me?.platformAdmin?.email || '—'}</div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">显示名</div>
              {editingProfile ? (
                <FormItem field="displayName" noStyle>
                  <Input placeholder="显示名" />
                </FormItem>
              ) : (
                <div className="text-[13px] text-foreground break-words">{me?.platformAdmin?.displayName || '—'}</div>
              )}
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">状态</div>
              <div className="text-[13px] text-foreground break-words">{fmtStatus(me?.platformAdmin?.status)}</div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">两步验证</div>
              <div className="text-[13px] text-foreground break-words">{me?.platformAdmin?.totpEnabled ? '已开启' : '未开启'}</div>
              <div style={{ gridColumn: '1 / -1', fontSize: 12 }}>
                <Link to="/platform-admins">管理平台管理员账号</Link>
              </div>
              {editingProfile && (
                <div style={{ gridColumn: '1 / -1' }}>
                  <Button type="primary" htmlType="submit" size="small">
                    保存
                  </Button>
                </div>
              )}
            </div>
          </Form>
        ) : (
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
                setEditingProfile(false)
                await loadMe()
              } catch (e: any) {
                Message.error(e?.msg || '更新失败')
              }
            }}
          >
            <div style={detailGridStyle}>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">账号 ID</div>
              <div className="text-[13px] text-foreground break-words">{me?.user?.id ?? '—'}</div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">邮箱</div>
              <div className="text-[13px] text-foreground break-words">{me?.user?.email || '—'}</div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">显示名</div>
              {editingProfile ? (
                <FormItem field="displayName" noStyle>
                  <Input placeholder="请输入显示名" />
                </FormItem>
              ) : (
                <div className="text-[13px] text-foreground break-words">{me?.user?.displayName || '—'}</div>
              )}
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">用户名</div>
              {editingProfile ? (
                <FormItem field="username" noStyle>
                  <Input placeholder="请输入用户名" />
                </FormItem>
              ) : (
                <div className="text-[13px] text-foreground break-words">{me?.user?.username || '—'}</div>
              )}
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">手机号</div>
              {editingProfile ? (
                <FormItem field="phone" noStyle>
                  <Input placeholder="请输入手机号" />
                </FormItem>
              ) : (
                <div className="text-[13px] text-foreground break-words">{me?.user?.phone || '—'}</div>
              )}
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">登录次数</div>
              <div className="text-[13px] text-foreground break-words">{me?.user?.loginCount ?? 0}</div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">上次登录</div>
              <div className="text-[13px] text-foreground break-words">{fmtLastLogin(me?.user?.lastLogin)}</div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">组织</div>
              <div className="text-[13px] text-foreground break-words">{me?.tenant?.name || '—'}</div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">组织标识</div>
              <div className="text-[13px] text-foreground break-words">{me?.tenant?.slug || '—'}</div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">部门</div>
              <div className="text-[13px] text-foreground break-words">
                {Array.isArray(me?.user?.tenantGroups) && me.user.tenantGroups.length
                  ? me.user.tenantGroups.map((g: { name?: string }) => g.name).join('、')
                  : me?.user?.tenantGroup?.name || '未分配'}
              </div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">账号状态</div>
              <div className="text-[13px] text-foreground break-words">{fmtStatus(me?.user?.status)}</div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">上次登录 IP</div>
              <div className="text-[13px] text-foreground break-words">{me?.user?.lastLoginIp || '—'}</div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">账号来源</div>
              <div className="text-[13px] text-foreground break-words">{me?.user?.source || '—'}</div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">注册时间</div>
              <div className="text-[13px] text-foreground break-words">
                {me?.user?.createdAt ? dayjs(me.user.createdAt).format('YYYY/MM/DD HH:mm:ss') : '—'}
              </div>
              <div className="text-[13px] text-muted-foreground whitespace-nowrap">两步验证</div>
              <div className="text-[13px] text-foreground break-words">{me?.user?.totpEnabled ? '已开启' : '未开启'}</div>
              <div className="col-span-full mt-1 text-[13px] text-muted-foreground">角色</div>
              <div className="col-span-full text-[13px] text-foreground break-words">
                {Array.isArray(me?.user?.roles) && me.user.roles.length
                  ? me.user.roles.map((r: { name?: string }) => r.name).join('、')
                  : '—'}
              </div>
              {editingProfile && (
                <div style={{ gridColumn: '1 / -1' }}>
                  <Button type="primary" htmlType="submit" size="small">
                    保存
                  </Button>
                </div>
              )}
            </div>
          </Form>
        )}
      </Card>
    </Space>
  )

  const totpEnabled = isPlatform ? !!me?.platformAdmin?.totpEnabled : !!me?.user?.totpEnabled

  const securityPanel = (
    <Card bordered={false} className="rounded-xl border border-border">
      {totpEnabled ? (
        <>
          <div className="mb-3 text-[13px] text-muted-foreground">
            关闭两步验证前，请输入登录密码与当前验证器中的动态码。
          </div>
          <Form
            form={totpDisableForm}
            layout="vertical"
            requiredSymbol={false}
            onSubmit={async (v) => {
              try {
                const res = await disableTotp({
                  password: String(v.password || ''),
                  code: String(v.code || '').trim(),
                })
                if (res.code !== 200 || !res.data) {
                  Message.error(res.msg || '关闭失败')
                  return
                }
                Message.success('已关闭两步验证')
                updateLocalProfile(res.data as never)
                setTotpDraft(null)
                totpDisableForm.resetFields()
                await loadMe()
              } catch (e: any) {
                Message.error(e?.msg || '关闭失败')
              }
            }}
          >
            <FormItem label="登录密码" field="password" rules={[{ required: true, message: '请输入密码' }]}>
              <Input.Password />
            </FormItem>
            <FormItem label="动态码" field="code" rules={[{ required: true, message: '请输入 6 位动态码' }]}>
              <Input maxLength={12} placeholder="验证器中的 6 位数字" />
            </FormItem>
            <Button type="primary" htmlType="submit" status="danger">
              关闭两步验证
            </Button>
          </Form>
        </>
      ) : (
        <>
          <div className="mb-3 text-[13px] text-muted-foreground">
            使用 Google Authenticator、1Password 等应用扫码绑定；绑定完成后需输入一次动态码确认。
          </div>
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Button
              type="outline"
              onClick={async () => {
                try {
                  const res = await setupTotp()
                  if (res.code !== 200 || !res.data) {
                    Message.error(res.msg || '生成失败')
                    return
                  }
                  setTotpDraft({ secret: res.data.secret, qrDataUrl: res.data.qrDataUrl })
                  totpEnableForm.setFieldValue('code', '')
                  Message.success('请使用验证器扫码')
                } catch (e: any) {
                  Message.error(e?.msg || '生成失败')
                }
              }}
            >
              生成绑定二维码
            </Button>
            {totpDraft?.qrDataUrl && (
              <div>
                <img
                  src={totpDraft.qrDataUrl}
                  alt="TOTP QR"
                  className="h-[220px] w-[220px] rounded-lg border border-border"
                />
                <div className="mt-2 break-all text-xs text-muted-foreground">
                  手动输入密钥：{totpDraft.secret}
                </div>
              </div>
            )}
            <Form
              form={totpEnableForm}
              layout="vertical"
              requiredSymbol={false}
              onSubmit={async (v) => {
                if (!totpDraft?.secret) {
                  Message.warning('请先生成二维码')
                  return
                }
                try {
                  const res = await enableTotp({
                    secret: totpDraft.secret,
                    code: String(v.code || '').trim(),
                  })
                  if (res.code !== 200 || !res.data) {
                    Message.error(res.msg || '启用失败')
                    return
                  }
                  Message.success('两步验证已开启')
                  updateLocalProfile(res.data as never)
                  setTotpDraft(null)
                  totpEnableForm.resetFields()
                  await loadMe()
                } catch (e: any) {
                  Message.error(e?.msg || '启用失败')
                }
              }}
            >
              <FormItem label="动态码确认" field="code" rules={[{ required: true, message: '请输入动态码' }]}>
                <Input maxLength={12} placeholder="扫码后输入 6 位数字" />
              </FormItem>
              <Button type="primary" htmlType="submit">
                确认启用
              </Button>
            </Form>
          </Space>
        </>
      )}
    </Card>
  )

  const passwordPanel = (
    <Card bordered={false} className="rounded-xl border border-border">
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
  )

  const handleLogout = async () => {
    try {
      await logoutApi()
    } finally {
      clearUser()
      navigate('/login', { replace: true })
    }
  }

  return (
    <BaseLayout title="个人中心" description="">
      <div className="flex flex-wrap items-start gap-3">
        <aside
          className="flex w-[168px] shrink-0 flex-col overflow-hidden rounded-lg border border-border bg-card"
          style={{ minHeight: 'min(72vh, 640px)' }}
        >
          <Menu
            className="!flex-1 !border-none !bg-transparent"
            style={{ width: '100%', padding: '8px 6px' }}
            selectedKeys={[activeSection]}
            onClickMenuItem={(key) => setActiveSection(key as ProfileSection)}
          >
            {navItems.map((item) => (
              <Menu.Item key={item.key} style={{ borderRadius: 8, lineHeight: '44px', height: 44, marginBottom: 4 }}>
                {item.label}
              </Menu.Item>
            ))}
          </Menu>
          <div className="mt-auto shrink-0 border-t border-border p-3">
            <Button long status="warning" onClick={() => void handleLogout()}>
              退出登录
            </Button>
          </div>
        </aside>

        <div className="min-w-0 flex-1 basis-[360px]">
          {activeSection === 'profile' && profilePanel}
          {activeSection === 'security' && securityPanel}
          {activeSection === 'password' && passwordPanel}
        </div>
      </div>
    </BaseLayout>
  )
}
