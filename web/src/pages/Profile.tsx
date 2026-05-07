import { useEffect, useState, type CSSProperties } from 'react'
import {
  Avatar,
  Button,
  Card,
  Form,
  Input,
  Message,
  Space,
  Tabs,
  Tag,
  Upload,
} from '@arco-design/web-react'
import dayjs from 'dayjs'
import { UserCircle } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
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

const TabPane = Tabs.TabPane
const FormItem = Form.Item

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

  const detailLabelStyle: CSSProperties = {
    fontSize: 13,
    color: 'var(--color-text-3)',
    whiteSpace: 'nowrap',
  }
  const detailGridStyle: CSSProperties = {
    display: 'grid',
    gridTemplateColumns: 'minmax(88px, auto) 1fr minmax(88px, auto) 1fr',
    columnGap: 16,
    rowGap: 14,
    alignItems: 'center',
  }
  const detailValueStyle: CSSProperties = {
    fontSize: 13,
    color: 'var(--color-text-1)',
    wordBreak: 'break-word',
  }

  return (
    <BaseLayout title="个人中心" description="">
      <Tabs defaultActiveTab="profile">
        <TabPane key="profile" title="资料信息">
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Card
              loading={loading}
              bordered={false}
              style={{
                borderRadius: 12,
                overflow: 'hidden',
                background:
                  'linear-gradient(135deg, var(--color-primary-light-1) 0%, var(--color-bg-2) 42%, var(--color-bg-2) 100%)',
                border: '1px solid var(--color-border)',
              }}
              bodyStyle={{ padding: 24 }}
            >
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 20, alignItems: 'flex-start' }}>
                <Avatar size={72} style={{ flexShrink: 0, backgroundColor: 'var(--color-fill-3)' }}>
                  {!isPlatform && avatarSrc() ? (
                    <img alt="" src={avatarSrc()} style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
                  ) : (
                    <UserCircle size={40} strokeWidth={1.5} color="var(--color-text-2)" />
                  )}
                </Avatar>
                <div style={{ flex: '1 1 220px', minWidth: 0 }}>
                  <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 10 }}>
                    <span style={{ fontSize: 22, fontWeight: 700, color: 'var(--color-text-1)' }}>{heroTitle()}</span>
                    {isPlatform ? (
                      <Tag color="orangered">平台管理员</Tag>
                    ) : (
                      <Tag color="arcoblue">租户成员</Tag>
                    )}
                    <Tag color={isPlatform || me?.user?.status === 'active' ? 'green' : 'gray'}>
                      {isPlatform ? '活跃' : fmtStatus(me?.user?.status)}
                    </Tag>
                  </div>
                  <div style={{ marginTop: 8, fontSize: 14, color: 'var(--color-text-2)', wordBreak: 'break-all' }}>
                    {heroSubtitle()}
                  </div>
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
                        <Tag size="small" style={{ color: 'var(--color-text-3)' }}>
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

            <Card
              title="账号详情"
              bordered={false}
              style={{ borderRadius: 12, border: '1px solid var(--color-border)' }}
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
                    <div style={detailLabelStyle}>账号 ID</div>
                    <div style={detailValueStyle}>{me?.platformAdmin?.id ?? '—'}</div>
                    <div style={detailLabelStyle}>邮箱</div>
                    <div style={detailValueStyle}>{me?.platformAdmin?.email || '—'}</div>
                    <div style={detailLabelStyle}>显示名</div>
                    {editingProfile ? (
                      <FormItem field="displayName" noStyle>
                        <Input placeholder="显示名" />
                      </FormItem>
                    ) : (
                      <div style={detailValueStyle}>{me?.platformAdmin?.displayName || '—'}</div>
                    )}
                    <div style={detailLabelStyle}>状态</div>
                    <div style={detailValueStyle}>{fmtStatus(me?.platformAdmin?.status)}</div>
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
                    <div style={detailLabelStyle}>账号 ID</div>
                    <div style={detailValueStyle}>{me?.user?.id ?? '—'}</div>
                    <div style={detailLabelStyle}>邮箱</div>
                    <div style={detailValueStyle}>{me?.user?.email || '—'}</div>
                    <div style={detailLabelStyle}>显示名</div>
                    {editingProfile ? (
                      <FormItem field="displayName" noStyle>
                        <Input placeholder="请输入显示名" />
                      </FormItem>
                    ) : (
                      <div style={detailValueStyle}>{me?.user?.displayName || '—'}</div>
                    )}
                    <div style={detailLabelStyle}>用户名</div>
                    {editingProfile ? (
                      <FormItem field="username" noStyle>
                        <Input placeholder="请输入用户名" />
                      </FormItem>
                    ) : (
                      <div style={detailValueStyle}>{me?.user?.username || '—'}</div>
                    )}
                    <div style={detailLabelStyle}>手机号</div>
                    {editingProfile ? (
                      <FormItem field="phone" noStyle>
                        <Input placeholder="请输入手机号" />
                      </FormItem>
                    ) : (
                      <div style={detailValueStyle}>{me?.user?.phone || '—'}</div>
                    )}
                    <div style={detailLabelStyle}>登录次数</div>
                    <div style={detailValueStyle}>{me?.user?.loginCount ?? 0}</div>
                    <div style={detailLabelStyle}>上次登录</div>
                    <div style={detailValueStyle}>{fmtLastLogin(me?.user?.lastLogin)}</div>
                    <div style={detailLabelStyle}>组织</div>
                    <div style={detailValueStyle}>{me?.tenant?.name || '—'}</div>
                    <div style={detailLabelStyle}>组织标识</div>
                    <div style={detailValueStyle}>{me?.tenant?.slug || '—'}</div>
                    <div style={detailLabelStyle}>部门</div>
                    <div style={detailValueStyle}>
                      {Array.isArray(me?.user?.tenantGroups) && me.user.tenantGroups.length
                        ? me.user.tenantGroups.map((g: { name?: string }) => g.name).join('、')
                        : me?.user?.tenantGroup?.name || '未分配'}
                    </div>
                    <div style={detailLabelStyle}>账号状态</div>
                    <div style={detailValueStyle}>{fmtStatus(me?.user?.status)}</div>
                    <div style={detailLabelStyle}>上次登录 IP</div>
                    <div style={detailValueStyle}>{me?.user?.lastLoginIp || '—'}</div>
                    <div style={detailLabelStyle}>账号来源</div>
                    <div style={detailValueStyle}>{me?.user?.source || '—'}</div>
                    <div style={detailLabelStyle}>注册时间</div>
                    <div style={detailValueStyle}>
                      {me?.user?.createdAt ? dayjs(me.user.createdAt).format('YYYY/MM/DD HH:mm:ss') : '—'}
                    </div>
                    <div style={detailLabelStyle}>两步验证</div>
                    <div style={detailValueStyle}>{me?.user?.totpEnabled ? '已开启' : '未开启'}</div>
                    <div style={{ ...detailLabelStyle, gridColumn: '1 / -1', marginTop: 4 }}>角色</div>
                    <div style={{ ...detailValueStyle, gridColumn: '1 / -1' }}>
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
        </TabPane>

        {!isPlatform && (
          <TabPane key="security" title="安全 · 两步验证">
            <Card bordered={false} style={{ borderRadius: 12, border: '1px solid var(--color-border)' }}>
              {me?.user?.totpEnabled ? (
                <>
                  <div style={{ marginBottom: 12, color: 'var(--color-text-2)', fontSize: 13 }}>
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
                  <div style={{ marginBottom: 12, color: 'var(--color-text-2)', fontSize: 13 }}>
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
                          style={{ width: 220, height: 220, borderRadius: 8, border: '1px solid var(--color-border)' }}
                        />
                        <div style={{ marginTop: 8, fontSize: 12, color: 'var(--color-text-3)', wordBreak: 'break-all' }}>
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
          </TabPane>
        )}

        <TabPane key="password" title="修改密码">
          <Card bordered={false} style={{ borderRadius: 12, border: '1px solid var(--color-border)' }}>
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

      <Card style={{ marginTop: 16 }} bordered={false}>
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
