import { useCallback, useEffect, useState } from 'react'
import {
  Button,
  Card,
  Input,
  Message,
  Select,
  Space,
  Spin,
  Tabs,
  Typography,
} from '@arco-design/web-react'
import { IconLeft } from '@arco-design/web-react/icon'
import { Link, useNavigate, useParams } from 'react-router-dom'
import BaseLayout from '@/components/Layout/BaseLayout'
import { getTenant, updateTenantPlatform } from '@/api/tenants'
import {
  type AiTab,
  defaultDraft,
  draftToPayload,
  normalizeDraft,
  providerRulesFor,
  ruleFor,
  validateDraft,
} from '@/constants/tenantAiConfigRules'

const TabPane = Tabs.TabPane

function renderAiFields(
  tab: AiTab,
  draft: Record<string, unknown>,
  setDraft: (next: Record<string, unknown>) => void,
) {
  const provider = String(draft.provider ?? '')
  const def = ruleFor(tab, provider)
  const opts = providerRulesFor(tab).map((x) => ({ value: x.provider, label: x.label }))
  return (
    <Space direction="vertical" size={14} style={{ width: '100%' }}>
      <div>
        <Typography.Text style={{ display: 'block', marginBottom: 6 }}>厂商（provider）</Typography.Text>
        <Select
          style={{ width: '100%', maxWidth: 480 }}
          value={provider}
          options={opts}
          onChange={(v) => setDraft({ ...defaultDraft(tab), provider: String(v) })}
        />
      </div>
      {def?.fields.map((f) => (
        <div key={f.key}>
          <Typography.Text style={{ display: 'block', marginBottom: 6 }}>
            {f.label}
            {f.required ? ' *' : ''}
          </Typography.Text>
          {f.type === 'password' ? (
            <Input.Password
              autoComplete="new-password"
              placeholder={f.placeholder}
              value={String(draft[f.key] ?? '')}
              onChange={(val) => setDraft({ ...draft, [f.key]: val })}
            />
          ) : f.type === 'number' ? (
            <Input
              type="number"
              style={{ maxWidth: 320 }}
              placeholder={f.placeholder}
              value={draft[f.key] === undefined || draft[f.key] === '' ? '' : String(draft[f.key])}
              onChange={(val) => setDraft({ ...draft, [f.key]: val })}
            />
          ) : f.type === 'textarea' ? (
            <Input.TextArea
              placeholder={f.placeholder}
              value={String(draft[f.key] ?? '')}
              autoSize={{ minRows: f.textareaMinRows ?? 10, maxRows: 32 }}
              onChange={(val) => setDraft({ ...draft, [f.key]: val })}
            />
          ) : (
            <Input
              placeholder={f.placeholder}
              value={String(draft[f.key] ?? '')}
              onChange={(val) => setDraft({ ...draft, [f.key]: val })}
            />
          )}
        </div>
      ))}
    </Space>
  )
}

export default function TenantAiConfig() {
  const { tenantId } = useParams<{ tenantId: string }>()
  const navigate = useNavigate()
  const [tenantName, setTenantName] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [aiTab, setAiTab] = useState<AiTab>('asr')
  const [draftAsr, setDraftAsr] = useState<Record<string, unknown>>(() => defaultDraft('asr'))
  const [draftTts, setDraftTts] = useState<Record<string, unknown>>(() => defaultDraft('tts'))
  const [draftLlm, setDraftLlm] = useState<Record<string, unknown>>(() => defaultDraft('llm'))
  const [draftRealtime, setDraftRealtime] = useState<Record<string, unknown>>(() => defaultDraft('realtime'))
  const [voiceMode, setVoiceMode] = useState<'pipeline' | 'realtime'>('pipeline')

  const load = useCallback(async () => {
    if (!tenantId) return
    setLoading(true)
    try {
      const r = await getTenant(tenantId)
      if (r.code !== 200 || !r.data?.tenant) {
        Message.error(r.msg || '加载租户失败')
        navigate('/tenant-management', { replace: true })
        return
      }
      const t = r.data.tenant
      setTenantName(t.name)
      setDraftAsr(normalizeDraft('asr', t.asrConfig))
      setDraftTts(normalizeDraft('tts', t.ttsConfig))
      setDraftLlm(normalizeDraft('llm', t.llmConfig))
      setDraftRealtime(normalizeDraft('realtime', t.realtimeConfig))
      setVoiceMode(t.voiceMode === 'realtime' ? 'realtime' : 'pipeline')
      if (t.voiceMode === 'realtime') {
        setAiTab('realtime')
      }
    } finally {
      setLoading(false)
    }
  }, [navigate, tenantId])

  useEffect(() => {
    void load()
  }, [load])

  const onSave = async () => {
    if (!tenantId) return
    const err =
      voiceMode === 'realtime'
        ? validateDraft('realtime', draftRealtime)
        : validateDraft('asr', draftAsr) || validateDraft('tts', draftTts) || validateDraft('llm', draftLlm)
    if (err) {
      Message.error(err)
      return
    }
    setSaving(true)
    try {
      const r = await updateTenantPlatform(tenantId, {
        asrConfig: draftToPayload('asr', draftAsr),
        ttsConfig: draftToPayload('tts', draftTts),
        llmConfig: draftToPayload('llm', draftLlm),
        voiceMode,
        realtimeConfig: draftToPayload('realtime', draftRealtime),
      })
      if (r.code !== 200) {
        Message.error(r.msg || '保存失败')
        return
      }
      Message.success('已保存 AI 配置')
    } finally {
      setSaving(false)
    }
  }

  const title = tenantName ? `AI 密钥与模型 — ${tenantName}` : 'AI 密钥与模型'

  return (
    <BaseLayout
      title={title}
      description="配置租户语音对话模式、ASR/TTS/LLM 与实时多模态参数。嵌入式 SIP 当前管线模式仅消费腾讯云 qcloud ASR+TTS。"
      actions={
        <Space>
          <Link to="/tenant-management">
            <Button icon={<IconLeft />}>返回租户列表</Button>
          </Link>
          <Button type="primary" loading={saving} disabled={loading} onClick={() => void onSave()}>
            保存配置
          </Button>
        </Space>
      }
    >
      <Card bordered={false} style={{ maxWidth: 960 }}>
        {loading ? (
          <div style={{ padding: 48, textAlign: 'center' }}>
            <Spin />
            <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
              加载租户配置…
            </Typography.Paragraph>
          </div>
        ) : (
          <Space direction="vertical" size={20} style={{ width: '100%' }}>
            <div>
              <Typography.Text style={{ display: 'block', marginBottom: 8 }}>
                <strong>对话模式</strong>
              </Typography.Text>
              <Select
                style={{ width: 360 }}
                value={voiceMode}
                options={[
                  { value: 'pipeline', label: '三层管线（ASR → LLM → TTS）默认' },
                  { value: 'realtime', label: '实时多模态（单条 WS，Qwen-Omni 等）' },
                ]}
                onChange={(v) => {
                  const mode = v as 'pipeline' | 'realtime'
                  setVoiceMode(mode)
                  if (mode === 'realtime') setAiTab('realtime')
                }}
              />
              <Typography.Paragraph type="secondary" style={{ marginBottom: 0, marginTop: 8, fontSize: 13 }}>
                {voiceMode === 'realtime'
                  ? '实时模式下 SIP 仅运行“实时多模态” Tab 的配置；ASR/TTS/LLM Tab 仅作草稿保存。转人工能力仍由模型输出关键词与标记驱动，与三层模式公用。'
                  : '默认走三层管线。实时多模态仅在需要极低延迟且接受“不能插 LLM 工具”的场景启用。'}
              </Typography.Paragraph>
            </div>
            <Tabs activeTab={aiTab} onChange={(k) => setAiTab(k as AiTab)} type="rounded">
              <TabPane key="asr" title="ASR">
                {renderAiFields('asr', draftAsr, setDraftAsr)}
              </TabPane>
              <TabPane key="tts" title="TTS">
                {renderAiFields('tts', draftTts, setDraftTts)}
              </TabPane>
              <TabPane key="llm" title="LLM">
                {renderAiFields('llm', draftLlm, setDraftLlm)}
              </TabPane>
              <TabPane key="realtime" title="实时多模态">
                {renderAiFields('realtime', draftRealtime, setDraftRealtime)}
              </TabPane>
            </Tabs>
          </Space>
        )}
      </Card>
    </BaseLayout>
  )
}
