import AdminLayout from '@/components/Layout/AdminLayout'
import ACDPoolTab from '@/pages/ContactCenter/ACDPoolTab'

const NumberPool = () => {
  return (
    <AdminLayout title="号码池" description="云联络中心 / 号码池">
      <ACDPoolTab active={true} />
    </AdminLayout>
  )
}

export default NumberPool
