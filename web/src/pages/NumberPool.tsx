import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import ACDPoolTab from '@/pages/ContactCenter/ACDPoolTab'
import { useTranslation } from '@/i18n'

const NumberPool = () => {
  const { t } = useTranslation()
  return (
    <BaseLayout title={t('pages.numberPool.title')} description={t('pages.numberPool.description')}>
      <ACDPoolTab active={true} />
    </BaseLayout>
  )
}

export default NumberPool
