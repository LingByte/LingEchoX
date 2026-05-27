import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import ScriptManagerTab from '@/pages/ContactCenter/ScriptManagerTab'
import { useTranslation } from '@/i18n'

const ScriptManager = () => {
  const { t } = useTranslation()
  return (
    <BaseLayout title={t('pages.scriptManager.title')} description={t('pages.scriptManager.description')}>
      <ScriptManagerTab />
    </BaseLayout>
  )
}

export default ScriptManager
