import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import ACDPoolTab from '@/pages/ContactCenter/ACDPoolTab'

const NumberPool = () => {
  return (
    <BaseLayout title="号码池" description="云联络中心 / 号码池">
      <ACDPoolTab active={true} />
    </BaseLayout>
  )
}

export default NumberPool
