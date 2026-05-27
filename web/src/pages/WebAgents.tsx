import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import WebSeatContactTab from '@/pages/ContactCenter/WebSeatContactTab'
import { useTranslation } from '@/i18n'

const WebAgents = () => {
  const { t } = useTranslation()
  return (
    <BaseLayout title={t('pages.webAgents.title')} description={t('pages.webAgents.description')}>
      <WebSeatContactTab />
    </BaseLayout>
  )
}

export default WebAgents
