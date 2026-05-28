import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Button,
  Card,
  Drawer,
  Form,
  Input,
  Modal,
  Radio,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Typography,
  Upload,
} from '@arco-design/web-react'
import type { ColumnProps } from '@arco-design/web-react/es/Table'
import { IconDelete, IconEdit, IconPlayArrow, IconRefresh, IconSound } from '@arco-design/web-react/icon'
import BaseLayout from '@/components/Layout/BaseLayout'
import {
  createXunfeiTrainingTask,
  deleteVoiceClone,
  deleteVoiceSynthesisRecord,
  getVoiceCloneCapabilities,
  getVoiceTrainingTexts,
  listVoiceClones,
  listVoiceSynthesisHistory,
  queryVolcengineTrainingTask,
  queryXunfeiTrainingTask,
  submitVolcengineTrainingAudio,
  submitXunfeiTrainingAudio,
  synthesizeWithVoiceClone,
  updateVoiceClone,
  type VoiceCloneCapabilities,
  type VoiceCloneRow,
  type VoiceProvider,
  type VoiceSynthesisRow,
  type VoiceTrainingTaskRow,
  type VoiceTrainingTextSegment,
  type VolcQueryTaskResult,
} from '@/api/voiceClone'
import { showAlert } from '@/utils/notification'
import { useTranslation } from '@/i18n'

const { TabPane } = Tabs

function xunfeiStatusTag(status: number, t: (k: string) => string) {
  switch (status) {
    case 1:
      return <Tag color="green">{t('voiceClones.statusSuccess')}</Tag>
    case 0:
      return <Tag color="red">{t('voiceClones.statusFailed')}</Tag>
    case 2:
      return <Tag color="arcoblue">{t('voiceClones.statusQueued')}</Tag>
    default:
      return <Tag color="orange">{t('voiceClones.statusTraining')}</Tag>
  }
}

function volcStatusTag(status: number, t: (k: string) => string) {
  switch (status) {
    case 2:
      return <Tag color="green">{t('voiceClones.statusSuccess')}</Tag>
    case 3:
      return <Tag color="red">{t('voiceClones.statusFailed')}</Tag>
    case 1:
      return <Tag color="orange">{t('voiceClones.statusTraining')}</Tag>
    default:
      return <Tag color="arcoblue">{t('voiceClones.statusQueued')}</Tag>
  }
}

const VoiceClones = () => {
  const { t } = useTranslation()
  const [tab, setTab] = useState('clones')

  const [clones, setClones] = useState<VoiceCloneRow[]>([])
  const [clonesLoading, setClonesLoading] = useState(false)

  const [history, setHistory] = useState<VoiceSynthesisRow[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)

  const [capabilities, setCapabilities] = useState<VoiceCloneCapabilities | null>(null)
  const [trainProvider, setTrainProvider] = useState<VoiceProvider>('xunfei')
  const [trainName, setTrainName] = useState('')
  const [trainSex, setTrainSex] = useState(1)
  const [trainAge, setTrainAge] = useState(2)
  const [trainTask, setTrainTask] = useState<VoiceTrainingTaskRow | null>(null)
  const [volcSpeakerId, setVolcSpeakerId] = useState('')
  const [volcLanguage, setVolcLanguage] = useState('zh')
  const [volcStatus, setVolcStatus] = useState<VolcQueryTaskResult | null>(null)
  const [segments, setSegments] = useState<VoiceTrainingTextSegment[]>([])
  const [selectedSegId, setSelectedSegId] = useState<number | null>(null)
  const [audioFile, setAudioFile] = useState<File | null>(null)
  const [trainBusy, setTrainBusy] = useState(false)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const [editOpen, setEditOpen] = useState(false)
  const [editRow, setEditRow] = useState<VoiceCloneRow | null>(null)
  const [editName, setEditName] = useState('')
  const [editDesc, setEditDesc] = useState('')
  const [editSaving, setEditSaving] = useState(false)

  const [delClone, setDelClone] = useState<VoiceCloneRow | null>(null)
  const [delHist, setDelHist] = useState<VoiceSynthesisRow | null>(null)

  const [synthOpen, setSynthOpen] = useState(false)
  const [synthClone, setSynthClone] = useState<VoiceCloneRow | null>(null)
  const [synthText, setSynthText] = useState('')
  const [synthBusy, setSynthBusy] = useState(false)
  const [synthResult, setSynthResult] = useState<VoiceSynthesisRow | null>(null)

  const loadClones = useCallback(async () => {
    setClonesLoading(true)
    try {
      const res = await listVoiceClones()
      if (res.code === 200) {
        setClones(res.data || [])
      } else {
        showAlert(res.msg || t('common.loadFailed'), 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.loadFailed'), 'error')
    } finally {
      setClonesLoading(false)
    }
  }, [t])

  const loadHistory = useCallback(async () => {
    setHistoryLoading(true)
    try {
      const res = await listVoiceSynthesisHistory(30)
      if (res.code === 200) {
        setHistory(res.data || [])
      } else {
        showAlert(res.msg || t('common.loadFailed'), 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.loadFailed'), 'error')
    } finally {
      setHistoryLoading(false)
    }
  }, [t])

  useEffect(() => {
    void loadClones()
    void (async () => {
      try {
        const res = await getVoiceCloneCapabilities()
        if (res.code === 200 && res.data) {
          setCapabilities(res.data)
          if (res.data.xunfei.configured) setTrainProvider('xunfei')
          else if (res.data.volcengine.configured) setTrainProvider('volcengine')
        }
      } catch {
        /* optional */
      }
    })()
  }, [loadClones])

  useEffect(() => {
    if (tab === 'history') void loadHistory()
  }, [tab, loadHistory])

  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [])

  const stopPoll = () => {
    if (pollRef.current) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
  }

  const startXunfeiPoll = (taskId: string) => {
    stopPoll()
    pollRef.current = setInterval(async () => {
      try {
        const res = await queryXunfeiTrainingTask(taskId)
        if (res.code !== 200 || !res.data) return
        setTrainTask(res.data)
        if (res.data.status === 1 || res.data.status === 0) {
          stopPoll()
          if (res.data.status === 1) {
            showAlert(t('voiceClones.trainSuccess'), 'success')
            void loadClones()
          } else {
            showAlert(res.data.failedReason || t('voiceClones.trainFailed'), 'error')
          }
        }
      } catch {
        /* ignore */
      }
    }, 4000)
  }

  const startVolcPoll = (speakerId: string, taskName: string) => {
    stopPoll()
    pollRef.current = setInterval(async () => {
      try {
        const res = await queryVolcengineTrainingTask(speakerId, taskName)
        if (res.code !== 200 || !res.data) return
        setVolcStatus(res.data)
        if (res.data.status === 2 || res.data.status === 3) {
          stopPoll()
          if (res.data.status === 2) {
            showAlert(t('voiceClones.trainSuccess'), 'success')
            void loadClones()
          } else {
            showAlert(res.data.failedDesc || t('voiceClones.trainFailed'), 'error')
          }
        }
      } catch {
        /* ignore */
      }
    }, 4000)
  }

  const loadTrainingText = async () => {
    const res = await getVoiceTrainingTexts(5001)
    if (res.code === 200 && res.data?.textSegments?.length) {
      setSegments(res.data.textSegments)
      const first = res.data.textSegments[0]
      const n = Number(first.segId)
      setSelectedSegId(Number.isFinite(n) ? n : null)
    }
  }

  const onCreateTrainTask = async () => {
    const name = trainName.trim()
    if (!name) {
      showAlert(t('voiceClones.taskNameRequired'), 'warning')
      return
    }
    setTrainBusy(true)
    try {
      const res = await createXunfeiTrainingTask({
        taskName: name,
        sex: trainSex,
        ageGroup: trainAge,
        language: 'zh',
      })
      if (res.code === 200 && res.data) {
        setTrainTask(res.data)
        await loadTrainingText()
        showAlert(t('voiceClones.taskCreated'), 'success')
      } else {
        showAlert(res.msg || t('common.failed'), 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.failed'), 'error')
    } finally {
      setTrainBusy(false)
    }
  }

  const onSubmitXunfeiAudio = async () => {
    if (!trainTask?.taskId) {
      showAlert(t('voiceClones.createTaskFirst'), 'warning')
      return
    }
    if (!selectedSegId) {
      showAlert(t('voiceClones.pickSegment'), 'warning')
      return
    }
    if (!audioFile) {
      showAlert(t('voiceClones.pickAudio'), 'warning')
      return
    }
    setTrainBusy(true)
    try {
      const res = await submitXunfeiTrainingAudio(trainTask.taskId, selectedSegId, audioFile)
      if (res.code === 200) {
        showAlert(t('voiceClones.audioSubmitted'), 'success')
        const q = await queryXunfeiTrainingTask(trainTask.taskId)
        if (q.code === 200 && q.data) setTrainTask(q.data)
        startXunfeiPoll(trainTask.taskId)
      } else {
        showAlert(res.msg || t('common.failed'), 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.failed'), 'error')
    } finally {
      setTrainBusy(false)
    }
  }

  const onSubmitVolcAudio = async () => {
    const speakerId = volcSpeakerId.trim()
    if (!speakerId) {
      showAlert(t('voiceClones.volcSpeakerRequired'), 'warning')
      return
    }
    if (!audioFile) {
      showAlert(t('voiceClones.pickAudio'), 'warning')
      return
    }
    setTrainBusy(true)
    try {
      const res = await submitVolcengineTrainingAudio(speakerId, volcLanguage, audioFile, trainName.trim() || undefined)
      if (res.code === 200) {
        showAlert(t('voiceClones.audioSubmitted'), 'success')
        startVolcPoll(speakerId, trainName.trim() || `火山音色 ${speakerId}`)
      } else {
        showAlert(res.msg || t('common.failed'), 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.failed'), 'error')
    } finally {
      setTrainBusy(false)
    }
  }

  const onRefreshXunfeiTrain = async () => {
    if (!trainTask?.taskId) return
    setTrainBusy(true)
    try {
      const res = await queryXunfeiTrainingTask(trainTask.taskId)
      if (res.code === 200 && res.data) {
        setTrainTask(res.data)
        if (res.data.status === 1) void loadClones()
      } else {
        showAlert(res.msg || t('common.failed'), 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.failed'), 'error')
    } finally {
      setTrainBusy(false)
    }
  }

  const onRefreshVolcTrain = async () => {
    const speakerId = volcSpeakerId.trim()
    if (!speakerId) return
    setTrainBusy(true)
    try {
      const res = await queryVolcengineTrainingTask(speakerId, trainName.trim() || undefined)
      if (res.code === 200 && res.data) {
        setVolcStatus(res.data)
        if (res.data.status === 2) void loadClones()
      } else {
        showAlert(res.msg || t('common.failed'), 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.failed'), 'error')
    } finally {
      setTrainBusy(false)
    }
  }

  const openEdit = (row: VoiceCloneRow) => {
    setEditRow(row)
    setEditName(row.voiceName)
    setEditDesc(row.voiceDescription || '')
    setEditOpen(true)
  }

  const saveEdit = async () => {
    if (!editRow) return
    if (!editName.trim()) {
      showAlert(t('voiceClones.nameRequired'), 'warning')
      return
    }
    setEditSaving(true)
    try {
      const res = await updateVoiceClone({
        id: editRow.id,
        voiceName: editName.trim(),
        voiceDescription: editDesc.trim(),
      })
      if (res.code === 200) {
        showAlert(t('common.saveSuccess'), 'success')
        setEditOpen(false)
        void loadClones()
      } else {
        showAlert(res.msg || t('common.saveFailed'), 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.saveFailed'), 'error')
    } finally {
      setEditSaving(false)
    }
  }

  const confirmDeleteClone = async () => {
    if (!delClone) return
    try {
      const res = await deleteVoiceClone(delClone.id)
      if (res.code === 200) {
        showAlert(t('common.deleteSuccess'), 'success')
        setDelClone(null)
        void loadClones()
      } else {
        showAlert(res.msg || t('common.deleteFailed'), 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.deleteFailed'), 'error')
    }
  }

  const confirmDeleteHist = async () => {
    if (!delHist) return
    try {
      const res = await deleteVoiceSynthesisRecord(delHist.id)
      if (res.code === 200) {
        showAlert(t('common.deleteSuccess'), 'success')
        setDelHist(null)
        void loadHistory()
      } else {
        showAlert(res.msg || t('common.deleteFailed'), 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.deleteFailed'), 'error')
    }
  }

  const openSynth = (row: VoiceCloneRow) => {
    setSynthClone(row)
    setSynthText('')
    setSynthResult(null)
    setSynthOpen(true)
  }

  const runSynth = async () => {
    if (!synthClone) return
    const text = synthText.trim()
    if (!text) {
      showAlert(t('voiceClones.synthTextRequired'), 'warning')
      return
    }
    setSynthBusy(true)
    try {
      const res = await synthesizeWithVoiceClone({ voiceCloneId: synthClone.id, text, language: 'zh' })
      if (res.code === 200 && res.data) {
        setSynthResult(res.data)
        showAlert(t('voiceClones.synthDone'), 'success')
        void loadHistory()
        void loadClones()
      } else {
        showAlert(res.msg || t('common.failed'), 'error')
      }
    } catch (e: unknown) {
      showAlert(e instanceof Error ? e.message : t('common.failed'), 'error')
    } finally {
      setSynthBusy(false)
    }
  }

  const cloneColumns: ColumnProps<VoiceCloneRow>[] = useMemo(
    () => [
      { title: t('voiceClones.colName'), dataIndex: 'voiceName', width: 160 },
      {
        title: t('voiceClones.colProvider'),
        dataIndex: 'provider',
        width: 100,
        render: (p: string) => <Tag>{p}</Tag>,
      },
      { title: t('voiceClones.colUsage'), dataIndex: 'usageCount', width: 80 },
      {
        title: t('common.actions'),
        width: 220,
        render: (_: unknown, row: VoiceCloneRow) => (
          <Space>
            <Button size="mini" icon={<IconSound />} onClick={() => openSynth(row)}>
              {t('voiceClones.trySynth')}
            </Button>
            <Button size="mini" icon={<IconEdit />} onClick={() => openEdit(row)} />
            <Button size="mini" status="danger" icon={<IconDelete />} onClick={() => setDelClone(row)} />
          </Space>
        ),
      },
    ],
    [t],
  )

  const histColumns: ColumnProps<VoiceSynthesisRow>[] = useMemo(
    () => [
      { title: t('voiceClones.colText'), dataIndex: 'text', ellipsis: true },
      {
        title: t('voiceClones.colAudio'),
        dataIndex: 'audioUrl',
        width: 120,
        render: (url: string) =>
          url ? (
            <a href={url} target="_blank" rel="noreferrer" style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
              <IconPlayArrow />
              {t('voiceClones.play')}
            </a>
          ) : (
            t('common.dash')
          ),
      },
      {
        title: t('common.actions'),
        width: 80,
        render: (_: unknown, row: VoiceSynthesisRow) => (
          <Button size="mini" status="danger" icon={<IconDelete />} onClick={() => setDelHist(row)} />
        ),
      },
    ],
    [t],
  )

  return (
    <BaseLayout title={t('pages.voiceClones.title')} description={t('pages.voiceClones.description')}>
      <Tabs activeTab={tab} onChange={setTab}>
        <TabPane key="clones" title={t('voiceClones.tabClones')}>
          <Card bordered={false}>
            <div style={{ marginBottom: 12, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Typography.Text type="secondary">{t('voiceClones.clonesHint')}</Typography.Text>
              <Space>
                <Button icon={<IconRefresh />} onClick={() => void loadClones()}>
                  {t('common.search')}
                </Button>
                <Button type="primary" onClick={() => setTab('train')}>
                  {t('voiceClones.newTrain')}
                </Button>
              </Space>
            </div>
            <Table
              rowKey="id"
              loading={clonesLoading}
              columns={cloneColumns}
              data={clones}
              pagination={false}
              noDataElement={t('common.noData')}
            />
          </Card>
        </TabPane>

        <TabPane key="train" title={t('voiceClones.tabTrain')}>
          <Card bordered={false} style={{ maxWidth: 760 }}>
            <Form layout="vertical">
              <Form.Item label={t('voiceClones.provider')}>
                <Radio.Group
                  type="button"
                  value={trainProvider}
                  onChange={(v) => {
                    setTrainProvider(v as VoiceProvider)
                    stopPoll()
                  }}
                >
                  <Radio value="xunfei" disabled={capabilities ? !capabilities.xunfei.configured : false}>
                    {t('voiceClones.providerXunfei')}
                    {capabilities?.xunfei.configured ? (
                      <Tag color="green" size="small" style={{ marginLeft: 6 }}>
                        OK
                      </Tag>
                    ) : (
                      <Tag size="small" style={{ marginLeft: 6 }}>
                        {t('voiceClones.notConfigured')}
                      </Tag>
                    )}
                  </Radio>
                  <Radio value="volcengine" disabled={capabilities ? !capabilities.volcengine.configured : false}>
                    {t('voiceClones.providerVolcengine')}
                    {capabilities?.volcengine.configured ? (
                      <Tag color="green" size="small" style={{ marginLeft: 6 }}>
                        OK
                      </Tag>
                    ) : (
                      <Tag size="small" style={{ marginLeft: 6 }}>
                        {t('voiceClones.notConfigured')}
                      </Tag>
                    )}
                  </Radio>
                </Radio.Group>
              </Form.Item>

              <Form.Item label={t('voiceClones.taskName')}>
                <Input value={trainName} onChange={setTrainName} placeholder={t('voiceClones.taskNamePlaceholder')} />
              </Form.Item>

              {trainProvider === 'xunfei' ? (
                <>
                  <Form.Item label={t('voiceClones.sex')}>
                    <Radio.Group value={trainSex} onChange={setTrainSex}>
                      <Radio value={1}>{t('voiceClones.sexMale')}</Radio>
                      <Radio value={2}>{t('voiceClones.sexFemale')}</Radio>
                    </Radio.Group>
                  </Form.Item>
                  <Form.Item label={t('voiceClones.ageGroup')}>
                    <Select
                      value={trainAge}
                      onChange={setTrainAge}
                      options={[
                        { value: 1, label: t('voiceClones.ageChild') },
                        { value: 2, label: t('voiceClones.ageYouth') },
                        { value: 3, label: t('voiceClones.ageMiddle') },
                        { value: 4, label: t('voiceClones.ageElder') },
                      ]}
                    />
                  </Form.Item>
                  <Space>
                    <Button type="primary" loading={trainBusy} onClick={() => void onCreateTrainTask()}>
                      {t('voiceClones.createTask')}
                    </Button>
                    {trainTask ? (
                      <Button icon={<IconRefresh />} loading={trainBusy} onClick={() => void onRefreshXunfeiTrain()}>
                        {t('voiceClones.refreshStatus')}
                      </Button>
                    ) : null}
                  </Space>
                </>
              ) : (
                <>
                  <Form.Item label={t('voiceClones.volcSpeakerId')} required>
                    <Input
                      value={volcSpeakerId}
                      onChange={setVolcSpeakerId}
                      placeholder={t('voiceClones.volcSpeakerPlaceholder')}
                    />
                  </Form.Item>
                  <Form.Item label={t('voiceClones.volcLanguage')}>
                    <Select
                      value={volcLanguage}
                      onChange={setVolcLanguage}
                      options={[
                        { value: 'zh', label: 'zh' },
                        { value: 'en', label: 'en' },
                      ]}
                    />
                  </Form.Item>
                  <Button icon={<IconRefresh />} loading={trainBusy} onClick={() => void onRefreshVolcTrain()}>
                    {t('voiceClones.refreshStatus')}
                  </Button>
                </>
              )}
            </Form>

            {trainProvider === 'xunfei' && trainTask ? (
              <div style={{ marginTop: 20, padding: 16, background: 'var(--color-fill-2)', borderRadius: 8 }}>
                <Space direction="vertical" style={{ width: '100%' }}>
                  <div>
                    <Typography.Text bold>{trainTask.taskName}</Typography.Text>
                    <span style={{ marginLeft: 8 }}>{xunfeiStatusTag(trainTask.status, t)}</span>
                  </div>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    taskId: {trainTask.taskId}
                  </Typography.Text>
                  {trainTask.failedReason ? (
                    <Typography.Text type="error">{trainTask.failedReason}</Typography.Text>
                  ) : null}
                  {segments.length > 0 ? (
                    <Form.Item label={t('voiceClones.trainSegment')} style={{ marginBottom: 0 }}>
                      <Select
                        value={selectedSegId ?? undefined}
                        onChange={(v) => setSelectedSegId(Number(v))}
                        options={segments.map((s) => ({
                          value: Number(s.segId),
                          label: s.segText,
                        }))}
                      />
                    </Form.Item>
                  ) : null}
                  <Upload
                    accept="audio/*,.wav,.mp3,.m4a"
                    limit={1}
                    autoUpload={false}
                    onChange={(fileList) => {
                      const f = fileList[0]?.originFile
                      setAudioFile(f instanceof File ? f : null)
                    }}
                  >
                    <Button>{t('voiceClones.uploadAudio')}</Button>
                  </Upload>
                  <Button type="primary" loading={trainBusy} onClick={() => void onSubmitXunfeiAudio()}>
                    {t('voiceClones.submitTrain')}
                  </Button>
                </Space>
              </div>
            ) : null}

            {trainProvider === 'volcengine' ? (
              <div style={{ marginTop: 20, padding: 16, background: 'var(--color-fill-2)', borderRadius: 8 }}>
                <Space direction="vertical" style={{ width: '100%' }}>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                    {t('voiceClones.volcHint')}
                  </Typography.Text>
                  {volcStatus ? (
                    <div>
                      <Typography.Text bold>speakerId: {volcStatus.speakerId}</Typography.Text>
                      <span style={{ marginLeft: 8 }}>{volcStatusTag(volcStatus.status, t)}</span>
                      {volcStatus.failedDesc ? (
                        <Typography.Text type="error" style={{ display: 'block' }}>
                          {volcStatus.failedDesc}
                        </Typography.Text>
                      ) : null}
                    </div>
                  ) : null}
                  <Upload
                    accept="audio/*,.wav,.mp3,.m4a"
                    limit={1}
                    autoUpload={false}
                    onChange={(fileList) => {
                      const f = fileList[0]?.originFile
                      setAudioFile(f instanceof File ? f : null)
                    }}
                  >
                    <Button>{t('voiceClones.uploadAudio')}</Button>
                  </Upload>
                  <Button type="primary" loading={trainBusy} onClick={() => void onSubmitVolcAudio()}>
                    {t('voiceClones.submitTrain')}
                  </Button>
                </Space>
              </div>
            ) : null}
          </Card>
        </TabPane>

        <TabPane key="history" title={t('voiceClones.tabHistory')}>
          <Card bordered={false}>
            <div style={{ marginBottom: 12 }}>
              <Button icon={<IconRefresh />} onClick={() => void loadHistory()}>
                {t('common.search')}
              </Button>
            </div>
            <Table
              rowKey="id"
              loading={historyLoading}
              columns={histColumns}
              data={history}
              pagination={false}
              noDataElement={t('common.noData')}
            />
          </Card>
        </TabPane>
      </Tabs>

      <Drawer
        width={420}
        title={t('voiceClones.editClone')}
        visible={editOpen}
        onCancel={() => setEditOpen(false)}
        footer={
          <Space>
            <Button onClick={() => setEditOpen(false)}>{t('common.cancel')}</Button>
            <Button type="primary" loading={editSaving} onClick={() => void saveEdit()}>
              {t('common.save')}
            </Button>
          </Space>
        }
      >
        <Form layout="vertical">
          <Form.Item label={t('voiceClones.colName')} required>
            <Input value={editName} onChange={setEditName} />
          </Form.Item>
          <Form.Item label={t('voiceClones.description')}>
            <Input.TextArea value={editDesc} onChange={setEditDesc} autoSize={{ minRows: 3 }} />
          </Form.Item>
        </Form>
      </Drawer>

      <Drawer
        width={480}
        title={t('voiceClones.trySynth')}
        visible={synthOpen}
        onCancel={() => setSynthOpen(false)}
        footer={
          <Button type="primary" loading={synthBusy} onClick={() => void runSynth()}>
            {t('voiceClones.runSynth')}
          </Button>
        }
      >
        <Typography.Text type="secondary" style={{ display: 'block', marginBottom: 12 }}>
          {synthClone?.voiceName}
        </Typography.Text>
        <Input.TextArea
          value={synthText}
          onChange={setSynthText}
          placeholder={t('voiceClones.synthPlaceholder')}
          autoSize={{ minRows: 4, maxRows: 8 }}
        />
        {synthResult?.audioUrl ? (
          <div style={{ marginTop: 16 }}>
            <audio controls src={synthResult.audioUrl} style={{ width: '100%' }} />
          </div>
        ) : null}
      </Drawer>

      <Modal
        title={t('common.confirmDelete')}
        visible={!!delClone}
        onOk={() => void confirmDeleteClone()}
        onCancel={() => setDelClone(null)}
      >
        {delClone?.voiceName}
      </Modal>
      <Modal
        title={t('common.confirmDelete')}
        visible={!!delHist}
        onOk={() => void confirmDeleteHist()}
        onCancel={() => setDelHist(null)}
      />
    </BaseLayout>
  )
}

export default VoiceClones
