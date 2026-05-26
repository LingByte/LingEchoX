import { useEffect, useState } from 'react'
import { Button, Popover, Space, Tag, Typography } from '@arco-design/web-react'
import { IconPhone } from '@arco-design/web-react/icon'
import { Link } from 'react-router-dom'
import type { ACDPoolTargetRow } from '@/api/acdPool'
import { useSIPAgentIncomingPoll, seatIdKey } from '@/hooks/useSIPAgentIncomingPoll'
import { useAuthStore } from '@/stores/authStore'
import { acdSeatMatchesUser, callerDisplay, fetchAllSipACDSeats } from '@/utils/sipAgentIncoming'

/**
 * Global header widget: polls SIP ACD seats bound to the logged-in user.
 */
export default function SIPAgentIncomingBell() {
  const user = useAuthStore((s) => s.user)
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const [matchedSeats, setMatchedSeats] = useState<ACDPoolTargetRow[]>([])
  const [ready, setReady] = useState(false)
  const [popoverOpen, setPopoverOpen] = useState(false)

  const { incomingBySeatId, incomingSeats, hasIncoming } = useSIPAgentIncomingPoll(matchedSeats, ready, {
    notify: true,
  })

  useEffect(() => {
    if (!isAuthenticated || !user) {
      setMatchedSeats([])
      setReady(false)
      return
    }
    let cancelled = false
    ;(async () => {
      try {
        const all = await fetchAllSipACDSeats()
        if (cancelled) return
        const matched = all.filter((row) => acdSeatMatchesUser(row, user))
        setMatchedSeats(matched)
        setReady(matched.length > 0)
      } catch {
        if (!cancelled) {
          setMatchedSeats([])
          setReady(false)
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [isAuthenticated, user?.id, user?.username, user?.email, user?.displayName])

  useEffect(() => {
    if (hasIncoming) setPopoverOpen(true)
    else setPopoverOpen(false)
  }, [hasIncoming])

  if (!ready || !matchedSeats.length) return null

  const seatLabels = matchedSeats.map((s) => s.name || s.targetValue || seatIdKey(s.id)).join('、')

  const content = (
    <div style={{ maxWidth: 320 }}>
      <Typography.Title heading={6} style={{ marginTop: 0, marginBottom: 8 }}>
        {hasIncoming ? '转接振铃中' : '400 值班'}
      </Typography.Title>
      {hasIncoming ? (
        <Space direction="vertical" size={8} style={{ width: '100%' }}>
          {incomingSeats.map((seat) => {
            const inc = incomingBySeatId[seatIdKey(seat.id)]
            return (
              <div key={seatIdKey(seat.id)}>
                <Space wrap>
                  <Tag color="orangered" icon={<IconPhone />}>
                    来电
                  </Tag>
                  <Typography.Text bold style={{ fontFamily: 'ui-monospace, monospace', fontSize: 16 }}>
                    {callerDisplay(inc)}
                  </Typography.Text>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    → {seat.name || seat.targetValue}
                  </Typography.Text>
                </Space>
              </div>
            )
          })}
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            请尽快在 SIP 软电话或话机上接听转接。
          </Typography.Text>
        </Space>
      ) : (
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          坐席：{seatLabels}。等待 400 转接振铃，有来电将自动提示。
        </Typography.Text>
      )}
      <div style={{ marginTop: 12 }}>
        <Link to="/number-pool" onClick={() => setPopoverOpen(false)}>
          打开号码池 / 坐席
        </Link>
      </div>
    </div>
  )

  return (
    <Popover
      trigger="click"
      position="br"
      popupVisible={popoverOpen}
      onVisibleChange={setPopoverOpen}
      content={content}
    >
      <Button
        type={hasIncoming ? 'primary' : 'secondary'}
        size="small"
        icon={<IconPhone />}
        status={hasIncoming ? 'warning' : undefined}
        style={hasIncoming ? { animation: 'sip-incoming-pulse 1.2s ease-in-out infinite' } : undefined}
      >
        {hasIncoming ? `来电 ${callerDisplay(incomingBySeatId[seatIdKey(incomingSeats[0]!.id)])}` : '值班'}
      </Button>
    </Popover>
  )
}
