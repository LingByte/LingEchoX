import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import ScriptManagerTab from '@/pages/ContactCenter/ScriptManagerTab'

const ScriptManager = () => {
  return (
    <BaseLayout title="脚本管理" description="云联络中心 / 脚本管理">
      <ScriptManagerTab />
    </BaseLayout>
  )
}

export default ScriptManager
