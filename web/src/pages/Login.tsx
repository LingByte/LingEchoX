import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { motion } from 'framer-motion'
import { Mail, Eye, EyeOff, LogIn, Lock as LockIcon, Shield, ExternalLink } from 'lucide-react'
import Button from '@/components/UI/Button'
import Input from '@/components/UI/Input'
import Modal from '@/components/UI/Modal'
import DatabaseUpgradeAlert from '@/components/UI/DatabaseUpgradeAlert'
import Captcha from '@/components/Auth/Captcha'
import { useAuthStore } from '@/stores/authStore'
import { useSiteConfig } from '@/contexts/SiteConfigContext'
import { buildLogoUrl } from '@/utils/logoUrl'
import { showAlert } from '@/utils/notification'
import { post } from '@/utils/request'
import { getApiBaseURL } from '@/config/apiConfig'

const Login = () => {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading] = useState(false)
  const [showCaptchaModal, setShowCaptchaModal] = useState(false)
  const [captchaId, setCaptchaId] = useState<string>('')
  const [captchaType, setCaptchaType] = useState<string>('')
  const [captchaData, setCaptchaData] = useState<any>(null)
  const [captchaVerified, setCaptchaVerified] = useState(false)
  const [requiresTwoFactor, setRequiresTwoFactor] = useState(false)
  const [twoFactorCode, setTwoFactorCode] = useState('')
  const { login } = useAuthStore()
  const { config } = useSiteConfig()
  const navigate = useNavigate()
  
  const siteName = config?.SITE_NAME || '七牛云联络中心'
  const siteTermsUrl = config?.SITE_TERMS_URL || ''
  const siteUrl = config?.SITE_URL || ''
  const logoUrl = buildLogoUrl(config?.SITE_LOGO_URL || '/static/img/favicon.png')
  const shouldUpgradeDB = config?.SHOULD_UPGRADE_DB || false

  const handleCaptchaVerify = (id: string, type: string, data: any) => {
    setCaptchaId(id)
    setCaptchaType(type)
    setCaptchaData(data)
    setCaptchaVerified(true)
    setShowCaptchaModal(false)
    // 验证成功后自动提交登录
    performLogin(id, type, data)
  }

  const handleCaptchaError = (error: string) => {
    setCaptchaVerified(false)
    showAlert(error, 'error', '验证码错误')
  }

  const performLogin = async (captchaId: string, captchaType: string, captchaData: any) => {
    setLoading(true)
    try {
      // 构建登录请求数据
      const loginData: any = {
        email,
        password,
        captchaId: captchaId,
        captchaType: captchaType,
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
        remember: true,
        authToken: true
      }

      // 根据验证码类型添加相应的数据
      if (captchaType === 'image') {
        loginData.captchaCode = captchaData || ''
      } else {
        loginData.captchaData = captchaData
      }

      // 如果已有2FA验证码，添加到请求中
      if (requiresTwoFactor && twoFactorCode) {
        loginData.twoFactorCode = twoFactorCode
      }

      // 调用后台登录API（专门用于管理后台，会验证用户是否是staff或admin）
      const response = await post(`${getApiBaseURL()}/auth/login/password`, loginData)

      // 后端返回格式: {code: 200, msg: "...", data: {user: {...}, token: "...", requiresTwoFactor: true}}
      if (response.code !== 200) {
        // 如果登录失败且需要验证码，重新显示验证码弹窗
        if (response.msg?.includes('captcha') || response.data?.error === 'captcha_required') {
          setShowCaptchaModal(true)
          setCaptchaVerified(false)
        }
        throw new Error(response.msg || '登录失败')
      }

      // 检查是否需要2FA验证
      if (response.data?.requiresTwoFactor) {
        setRequiresTwoFactor(true)
        setLoading(false)
        return
      }

      const responseData = response.data || {}
      const userData = responseData.user || responseData
      const token = responseData.token || userData.token || userData.AuthToken
      
      if (!token) {
        throw new Error('登录失败：未获取到token')
      }
      
      // 使用authStore的login方法
      const success = await login(token, {
        id: userData.id || userData.ID || 0,
        email: userData.email || email,
        displayName: userData.displayName || userData.display_name || email.split('@')[0],
        ...userData
      })
      
      if (success) {
        showAlert('登录成功', 'success', '欢迎回来')
        navigate('/dashboard')
      } else {
        throw new Error('登录处理失败')
      }
    } catch (error: any) {
      console.error('Login error:', error)
      showAlert(
        error?.msg || error?.message || '登录失败，请检查邮箱和密码',
        'error',
        '登录失败'
      )
    } finally {
      setLoading(false)
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    
    if (!email || !password) {
      showAlert('请填写完整信息', 'error', '登录失败')
      return
    }

    // 如果正在2FA验证阶段，使用已有的验证码信息继续登录
    if (requiresTwoFactor && twoFactorCode) {
      if (captchaVerified && captchaId) {
        // 使用之前的验证码信息继续登录
        await performLogin(captchaId, captchaType, captchaData)
        return
      } else {
        // 如果没有验证码信息，需要重新验证
        showAlert('请先完成图形验证码验证', 'error', '验证失败')
        setShowCaptchaModal(true)
        return
      }
    }

    // 如果已经验证过验证码，直接登录
    if (captchaVerified && captchaId) {
      await performLogin(captchaId, captchaType, captchaData)
      return
    }

    // 否则显示验证码弹窗
    setShowCaptchaModal(true)

  }

  return (
    <div className="min-h-screen flex items-center justify-center relative overflow-hidden dark:from-slate-900 dark:via-slate-800 dark:to-slate-900">
      {/* 数据库升级提示 */}
      <DatabaseUpgradeAlert shouldShow={shouldUpgradeDB} />
      
      {/* 背景装饰 */}
      <div className="absolute inset-0 overflow-hidden">
        <div className="absolute -top-40 -right-40 w-96 h-96 bg-blue-300 dark:bg-blue-900 rounded-full mix-blend-multiply dark:mix-blend-soft-light filter blur-3xl opacity-30 animate-pulse" />
        <div className="absolute -bottom-40 -left-40 w-96 h-96 bg-indigo-300 dark:bg-indigo-900 rounded-full mix-blend-multiply dark:mix-blend-soft-light filter blur-3xl opacity-30 animate-pulse animation-delay-2000" />
        <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-96 h-96 bg-purple-300 dark:bg-purple-900 rounded-full mix-blend-multiply dark:mix-blend-soft-light filter blur-3xl opacity-20 animate-pulse animation-delay-4000" />
      </div>

      <motion.div
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.5 }}
        className="w-full max-w-md relative z-10 px-4"
      >
        <div className="bg-white/95 dark:bg-slate-800/95 backdrop-blur-xl rounded-3xl shadow-2xl p-8 md:p-10 border border-slate-200/50 dark:border-slate-700/50">
          {/* Logo和标题 */}
          <div className="text-center mb-8">
            <motion.div
              initial={{ scale: 0.8, opacity: 0 }}
              animate={{ scale: 1, opacity: 1 }}
              transition={{ delay: 0.2, type: "spring", stiffness: 200 }}
              className="inline-flex items-center justify-center mb-6"
            >
              <div className="relative">
                <div className="absolute inset-0 to-purple-600 rounded-2xl blur-lg opacity-50 animate-pulse" />
                <div className="relative w-20 h-20 rounded-2xl flex items-center justify-center">
                  <img 
                    src={logoUrl} 
                    alt={siteName} 
                    className="w-15 h-15 object-contain"
                    onError={(e) => {
                      // 如果logo加载失败，显示默认图标
                      const target = e.target as HTMLImageElement
                      target.style.display = 'none'
                      const parent = target.parentElement
                      if (parent) {
                        parent.innerHTML = '<svg class="w-10 h-10 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" /></svg>'
                      }
                    }}
                  />
                </div>
              </div>
            </motion.div>
            <motion.h1
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: 0.3 }}
              className="text-3xl font-bold bg-gradient-to-r from-blue-600 via-indigo-600 to-purple-600 bg-clip-text mb-2"
            >
              七牛云联络中心
            </motion.h1>
            <motion.p
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: 0.4 }}
              className="text-slate-600 dark:text-slate-400 text-sm"
            >
              管理后台登录
            </motion.p>
          </div>

          {/* 登录表单 */}
          <form onSubmit={handleSubmit} className="space-y-5">
            <div>
              <Input
                type="email"
                label="邮箱"
                placeholder="请输入邮箱"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                leftIcon={<Mail className="w-4 h-4" />}
                size="lg"
                required
                disabled={loading}
              />
            </div>

            <div>
              <Input
                type={showPassword ? 'text' : 'password'}
                label="密码"
                placeholder="请输入密码"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                leftIcon={<LockIcon className="w-4 h-4" />}
                rightIcon={
                  <button
                    type="button"
                    onClick={() => setShowPassword(!showPassword)}
                    className="text-slate-400 hover:text-slate-600 dark:hover:text-slate-300"
                  >
                    {showPassword ? (
                      <EyeOff className="w-4 h-4" />
                    ) : (
                      <Eye className="w-4 h-4" />
                    )}
                  </button>
                }
                size="lg"
                required
                disabled={loading}
              />
            </div>

            {requiresTwoFactor && (
              <div className="space-y-3 p-4 bg-blue-50 dark:bg-blue-900/20 rounded-lg border border-blue-200 dark:border-blue-800">
                <div className="flex items-center gap-2 text-blue-600 dark:text-blue-400">
                  <Shield className="w-5 h-5" />
                  <p className="text-sm font-medium">两步验证</p>
                </div>
                <Input
                  type="text"
                  label="验证码"
                  placeholder="请输入6位验证码"
                  value={twoFactorCode}
                  onChange={(e) => {
                    const value = e.target.value.replace(/\D/g, '').slice(0, 6)
                    setTwoFactorCode(value)
                  }}
                  leftIcon={<Shield className="w-4 h-4" />}
                  size="lg"
                  required
                  disabled={loading}
                  maxLength={6}
                />
                <p className="text-xs text-slate-500 dark:text-slate-400">
                  请输入您的身份验证器应用生成的6位验证码
                </p>
              </div>
            )}

            <Button
              type="submit"
              variant="primary"
              size="lg"
              fullWidth
              loading={loading}
              leftIcon={<LogIn className="w-4 h-4" />}
              className="mt-6"
            >
              {loading ? '登录中...' : requiresTwoFactor ? '验证并登录' : '登录'}
            </Button>
          </form>

          {/* 联系我们 */}
          {(siteTermsUrl || siteUrl) && (
            <motion.div
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ delay: 0.5 }}
              className="mt-6 pt-6 border-t border-slate-200 dark:border-slate-700"
            >
              <p className="text-xs text-slate-500 dark:text-slate-400 text-center mb-3">
                联系我们
              </p>
              <div className="flex items-center justify-center gap-4">
                {siteUrl && (
                  <a
                    href={siteUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center gap-1 text-xs text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 transition-colors"
                  >
                    <span>访问官网</span>
                    <ExternalLink className="w-3 h-3" />
                  </a>
                )}
                {siteTermsUrl && (
                  <a
                    href={siteTermsUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center gap-1 text-xs text-blue-600 dark:text-blue-400 hover:text-blue-700 dark:hover:text-blue-300 transition-colors"
                  >
                    <span>服务条款</span>
                    <ExternalLink className="w-3 h-3" />
                  </a>
                )}
              </div>
            </motion.div>
          )}
        </div>

        {/* 验证码弹窗 */}
        <Modal
          isOpen={showCaptchaModal}
          onClose={() => {
            setShowCaptchaModal(false)
            setCaptchaVerified(false)
          }}
          title="安全验证"
          size="md"
          closeOnOverlayClick={false}
        >
          <Captcha
            onVerify={handleCaptchaVerify}
            onError={handleCaptchaError}
          />
        </Modal>
      </motion.div>
    </div>
  )
}

export default Login

