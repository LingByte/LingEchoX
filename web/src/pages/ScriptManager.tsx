import AdminLayout from '@/components/Layout/AdminLayout'
import ScriptManagerTab from '@/pages/ContactCenter/ScriptManagerTab'

const ScriptManager = () => {
  return (
    <AdminLayout title="脚本管理" description="云联络中心 / 脚本管理">
      <ScriptManagerTab />
    </AdminLayout>
  )
}

export default ScriptManager
