import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import OutboundCampaignTab from '@/pages/ContactCenter/OutboundCampaignTab'

const OutboundTasks = () => {
  return (
    <BaseLayout title="外呼任务" description="云联络中心 / 外呼任务">
      <OutboundCampaignTab />
    </BaseLayout>
  )
}

export default OutboundTasks
