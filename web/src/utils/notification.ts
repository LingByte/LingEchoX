import { Message } from '@arco-design/web-react'

export type NotificationType = 'success' | 'error' | 'warning' | 'info'

export interface NotificationOptions {
  title?: string
  message?: string
  duration?: number
}

export const showAlert = (
  message: string,
  type: NotificationType = 'info',
  title?: string,
  options?: Partial<NotificationOptions>
) => {
  const head =
    title ||
    (type === 'error' ? '错误' : type === 'warning' ? '警告' : type === 'success' ? '成功' : '提示')
  const content = `${head}: ${message}`
  const duration = options?.duration
  switch (type) {
    case 'success':
      Message.success({ content, duration })
      break
    case 'error':
      Message.error({ content, duration })
      break
    case 'warning':
      Message.warning({ content, duration })
      break
    default:
      Message.info({ content, duration })
  }
}

export const showConfirm = (
  message: string,
  title: string = '确认',
  onConfirm: () => void,
  onCancel?: () => void
) => {
  const confirmed = window.confirm(`${title}\n\n${message}`)
  if (confirmed) onConfirm()
  else onCancel?.()
}

export const showPrompt = (
  message: string,
  title: string = '输入',
  defaultValue: string = ''
): string | null => window.prompt(`${title}\n\n${message}`, defaultValue)
