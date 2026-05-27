import type { ReactNode } from 'react'
import { Tooltip } from '@arco-design/web-react'
import { IconQuestionCircle } from '@arco-design/web-react/icon'

export function FieldHint({ content }: { content: ReactNode }) {
  if (content == null || content === '') return null
  return (
    <Tooltip
      position="top"
      trigger="hover"
      content={
        <div
          style={{
            maxWidth: 360,
            fontSize: 12,
            lineHeight: 1.55,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
          }}
        >
          {content}
        </div>
      }
    >
      <span
        className="inline-flex shrink-0 items-center justify-center cursor-help align-middle"
        style={{ color: 'var(--color-text-3)', marginLeft: 4, verticalAlign: 'middle' }}
        role="img"
        aria-label="字段说明"
        onClick={(e) => e.preventDefault()}
      >
        <IconQuestionCircle style={{ fontSize: 14 }} />
      </span>
    </Tooltip>
  )
}
