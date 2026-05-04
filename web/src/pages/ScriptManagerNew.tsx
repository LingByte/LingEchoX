import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ChevronLeft } from 'lucide-react'
import AdminLayout from '@/components/Layout/AdminLayout'
import Button from '@/components/UI/Button'
import ScriptSpecEditor from '@/pages/ContactCenter/ScriptSpecEditor'
import {
  newHybridScriptDraftWithAutoIdentity,
  parseHybridScriptDraft,
  serializeHybridScriptDraft,
} from '@/pages/ContactCenter/scriptSpecTypes'
import { createSIPScriptTemplate } from '@/api/sipContactCenter'
import { showAlert } from '@/utils/notification'

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
    <AdminLayout
      title="新建脚本"
      description="脚本逻辑 ID 与版本由系统自动生成；请填写模板名称并编排流程步骤。"
      actions={
        <Button
          variant="outline"
          size="sm"
          leftIcon={<ChevronLeft className="h-4 w-4" />}
          onClick={() => navigate('/script-manager')}
        >
          返回列表
        </Button>
      }
    >
      <div className="mt-4 max-w-4xl space-y-4">
        <input
          className="border border-border rounded-md px-3 py-2 bg-card w-full text-sm"
          placeholder="脚本名称（必填）"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <textarea
          className="border border-border rounded-md px-3 py-2 bg-card w-full text-sm min-h-[72px]"
          placeholder="描述（可选）"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
        <label className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          启用
        </label>
        <ScriptSpecEditor
          value={scriptSpec}
          onChange={setScriptSpec}
          lockedScriptIdentity={created.lockedIdentity}
        />
        <div className="flex flex-wrap gap-2 pb-8">
          <Button onClick={() => void save()} disabled={saving}>
            {saving ? '保存中...' : '创建脚本'}
          </Button>
          <Button variant="outline" disabled={saving} onClick={() => navigate('/script-manager')}>
            取消
          </Button>
        </div>
      </div>
    </AdminLayout>
  )
}
