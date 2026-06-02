import { Button, Input, Space, Typography } from '@arco-design/web-react'
import { IconDelete, IconPlus } from '@arco-design/web-react/icon'

export type MetaDataPair = { key: string; value: string }

export function metaDataPairsFromJSON(raw?: string): MetaDataPair[] {
  const t = raw?.trim()
  if (!t) return []
  try {
    const parsed = JSON.parse(t) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return []
    }
    return Object.entries(parsed as Record<string, unknown>).map(([key, value]) => ({
      key,
      value: value == null ? '' : String(value),
    }))
  } catch {
    return []
  }
}

export function metaDataJSONFromPairs(pairs: MetaDataPair[]): Record<string, unknown> | undefined {
  const out: Record<string, unknown> = {}
  for (const { key, value } of pairs) {
    const k = key.trim()
    if (!k) continue
    out[k] = value
  }
  if (Object.keys(out).length === 0) return undefined
  return out
}

export function validateMetaDataPairs(pairs: MetaDataPair[]): string | null {
  const seen = new Set<string>()
  for (const { key } of pairs) {
    const k = key.trim()
    if (!k) continue
    if (seen.has(k)) {
      return `扩展字段 key 重复：${k}`
    }
    seen.add(k)
  }
  return null
}

type Props = {
  pairs: MetaDataPair[]
  onChange: (pairs: MetaDataPair[]) => void
}

export function MetaDataKeyValueEditor({ pairs, onChange }: Props) {
  const updatePair = (index: number, patch: Partial<MetaDataPair>) => {
    onChange(pairs.map((p, i) => (i === index ? { ...p, ...patch } : p)))
  }

  return (
    <div>
      <Typography.Text style={{ fontSize: 12 }}>扩展字段 MetaData（可选）</Typography.Text>
      <Space direction="vertical" size={8} style={{ width: '100%', marginTop: 8 }}>
        {pairs.length === 0 ? (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            暂无扩展字段，点击下方「新增字段」添加。
          </Typography.Text>
        ) : (
          pairs.map((pair, index) => (
            <div key={index} className="flex items-center gap-2">
              <Input
                placeholder="Key，如 FactoryNumber"
                value={pair.key}
                onChange={(v) => updatePair(index, { key: v })}
                style={{ flex: 1 }}
              />
              <Input
                placeholder="Value，如 F-1001"
                value={pair.value}
                onChange={(v) => updatePair(index, { value: v })}
                style={{ flex: 1 }}
              />
              <Button
                type="text"
                status="danger"
                icon={<IconDelete />}
                onClick={() => onChange(pairs.filter((_, i) => i !== index))}
              />
            </div>
          ))
        )}
        <Button
          type="outline"
          size="small"
          icon={<IconPlus />}
          onClick={() => onChange([...pairs, { key: '', value: '' }])}
        >
          新增字段
        </Button>
      </Space>
      <Typography.Paragraph type="secondary" style={{ margin: '4px 0 0', fontSize: 11 }}>
        用于「坐席接听前播报」模板占位符，如 {'{{MetaData.FactoryNumber}}'}（在中继号码设置中配置播报文案）。
      </Typography.Paragraph>
    </div>
  )
}
