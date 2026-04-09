import AdminLayout from '@/components/Layout/AdminLayout'
import OutboundCampaignTab from '@/pages/ContactCenter/OutboundCampaignTab'

const OutboundTasks = () => {
  return (
    <AdminLayout title="外呼任务" description="云联络中心 / 外呼任务">
      <OutboundCampaignTab />
    </AdminLayout>
  )
}

export default OutboundTasks
