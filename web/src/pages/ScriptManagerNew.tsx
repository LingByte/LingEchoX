import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Checkbox, Input, Space } from '@arco-design/web-react'
import { IconLeft } from '@arco-design/web-react/icon'
import BaseLayout from '@/components/Layout/BaseLayout.tsx'
import ScriptSpecEditor from '@/pages/ContactCenter/ScriptSpecEditor'
import {
  newHybridScriptDraftWithAutoIdentity,
  parseHybridScriptDraft,
  serializeHybridScriptDraft,
} from '@/pages/ContactCenter/scriptSpecTypes'
import { createSIPScriptTemplate } from '@/api/sipScripts'
import { showAlert } from '@/utils/notification'

const TextArea = Input.TextArea

export default function ScriptManagerNew() {
  const navigate = useNavigate()
  const created = useMemo(() => {
    const draft = newHybridScriptDraftWithAutoIdentity()
    return {
      lockedIdentity: { id: draft.id, version: draft.version },
      scriptSpec: serializeHybridScriptDraft(draft),
    }
  }, [])

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [scriptSpec, setScriptSpec] = useState(created.scriptSpec)
  const [saving, setSaving] = useState(false)

  const save = async () => {
    if (!name.trim()) return showAlert('脚本名称不能为空', 'error')
    const check = parseHybridScriptDraft(scriptSpec.trim())
    if (!check.ok) return showAlert(`脚本内容有误：${check.error}`, 'error')
    setSaving(true)
    try {
      const body = {
        name: name.trim(),
        description: description.trim(),
        enabled,
        scriptSpec: scriptSpec.trim(),
      }
      const res = await createSIPScriptTemplate(body)
      if (res.code === 200) {
        showAlert('创建成功', 'success')
        navigate('/script-manager')
      } else showAlert(res.msg || '创建失败', 'error')
    } catch (e: unknown) {
      showAlert((e as { msg?: string })?.msg || '创建失败', 'error')
    } finally {
      setSaving(false)
    }
  }

  return (
    <BaseLayout
      title="新建脚本"
      description="脚本逻辑 ID 与版本由系统自动生成；请填写模板名称并编排流程步骤。"
      actions={
        <Button type="outline" size="small" icon={<IconLeft />} onClick={() => navigate('/script-manager')}>
          返回列表
        </Button>
      }
    >
      <div className="mt-4 max-w-4xl space-y-4">
        <Input placeholder="脚本名称（必填）" value={name} onChange={setName} />
        <TextArea placeholder="描述（可选）" autoSize={{ minRows: 3 }} value={description} onChange={setDescription} />
        <Checkbox checked={enabled} onChange={(c) => setEnabled(!!c)}>启用</Checkbox>
        <ScriptSpecEditor
          value={scriptSpec}
          onChange={setScriptSpec}
          lockedScriptIdentity={created.lockedIdentity}
        />
        <Space className="pb-8">
          <Button type="primary" onClick={() => void save()} disabled={saving}>
            {saving ? '保存中...' : '创建脚本'}
          </Button>
          <Button type="outline" disabled={saving} onClick={() => navigate('/script-manager')}>
            取消
          </Button>
        </Space>
      </div>
    </BaseLayout>
  )
}
