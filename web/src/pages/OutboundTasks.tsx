import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import OutboundCampaignTab from '@/pages/ContactCenter/OutboundCampaignTab'
import { useTranslation } from '@/i18n'

const OutboundTasks = () => {
  const { t } = useTranslation()
  return (
    <BaseLayout title={t('pages.outboundTasks.title')} description={t('pages.outboundTasks.description')}>
      <OutboundCampaignTab />
    </BaseLayout>
  )
}

export default OutboundTasks
