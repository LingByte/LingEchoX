import { Card, Grid, Typography } from '@arco-design/web-react'
import BaseLayout from '@/components/Layout/BaseLayout'
import { useAuthStore } from '@/stores/authStore'

const { Row, Col } = Grid

export default function Overview() {
  const user = useAuthStore((s) => s.user)
  const tenantName = String(user?.tenantName || '贵司')
  const displayName = String(user?.displayName || user?.email || '管理员')

  return (
    <BaseLayout title="概览" description="欢迎来到 LingEchoX 控制台">
      <Card bordered={false}>
        <Typography.Title heading={4} style={{ marginTop: 0 }}>
          欢迎，{tenantName}
        </Typography.Title>
        <Typography.Paragraph style={{ color: 'var(--color-text-2)' }}>
          {displayName}，您已登录企业空间。可从左侧导航进入 SIP 用户、通话记录、外呼任务等模块。
        </Typography.Paragraph>
      </Card>
      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={8}>
          <Card title="组织名称" bordered>
            <Typography.Text>{tenantName}</Typography.Text>
          </Card>
        </Col>
        <Col span={8}>
          <Card title="当前账号" bordered>
            <Typography.Text>{displayName}</Typography.Text>
          </Card>
        </Col>
        <Col span={8}>
          <Card title="组织标识" bordered>
            <Typography.Text>{String(user?.tenantSlug || '-')}</Typography.Text>
          </Card>
        </Col>
      </Row>
    </BaseLayout>
  )
}
