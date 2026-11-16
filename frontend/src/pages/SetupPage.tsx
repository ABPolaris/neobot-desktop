import {useRef, useState} from 'react'
import {Alert, Button, Card, Collapse, Form, Input, QRCode, Space, Steps, Tag, Typography} from 'antd'
import {Link, useNavigate} from 'react-router'
import {BindQQ, Install, OpenDashboard, SaveModel, SaveQQ, Stop, Start} from '../../wailsjs/go/main/App'
import {useLauncher} from '../app/LauncherProvider'
import {useRuntimeActions} from '../app/actions'
import {JobCard, RuntimeError} from '../components/JobCard'

type QQForm = {id: string; secret: string}
type ModelForm = {key: string; url?: string; name?: string}

export function SetupPage() {
    const {state, action} = useLauncher()
    const navigate = useNavigate()
    const runtime = useRuntimeActions()
    const [qqForm] = Form.useForm<QQForm>()
    const [modelForm] = Form.useForm<ModelForm>()
    const [saving, setSaving] = useState<'qq' | 'model' | 'scan' | 'restart'>()
    const qqRevision = useRef<string | undefined>(undefined)
    const modelRevision = useRef<{config: string; env: string} | undefined>(undefined)
    if (!state) return null

    const {qq, model} = state.setup
    // A configured model and a connected robot are independent milestones.
    const ready = qq.connected && model.applied && !qq.needs_restart
    const configuration = Boolean(state.running && state.setup_supported && !state.starting)
    const current = ready ? 2 : configuration ? 1 : 0
    const stale = state.running && state.selected !== state.running
    const canRestart = Boolean(state.running && !stale && !state.stopping)
    const busy = saving !== undefined

    async function perform(kind: typeof saving, work: () => Promise<void>) {
        setSaving(kind)
        try { await action(work) } finally { setSaving(undefined) }
    }

    const qrURL = (() => {
        if (qq.phase !== 'pending' || !qq.qr_url) return ''
        try {
            const url = new URL(qq.qr_url)
            return url.protocol === 'https:' ? qq.qr_url : ''
        } catch { return '' }
    })()

    return <div className="page">
        <header className="page-heading">
            <div><Typography.Title level={1}>首次配置</Typography.Title><Typography.Text type="secondary">准备环境、绑定 QQ，再启用回复模型。</Typography.Text></div>
            <Link to="/">稍后继续</Link>
        </header>
        <Card><Steps current={current} items={[{title: '准备环境'}, {title: '配置 QQ 和模型'}, {title: '测试回复'}]} /></Card>
        <RuntimeError />
        <JobCard />
        {!configuration && <Card title="准备 NeoBot">
            <Space orientation="vertical" size="middle" className="full">
                {state.running ? <>
                    {state.starting ? <Alert type="info" title="NeoBot 正在启动" />
                        : <Alert type="info" title="此版本请使用网页管理完成配置" />}
                    {state.dashboard_url && <Button type="primary" onClick={() => void action(OpenDashboard)}>打开网页管理</Button>}
                    <Button loading={state.stopping} onClick={() => void runtime.stop()}>停止 NeoBot</Button>
                </> : state.selected ? <Button type="primary" loading={state.starting}
                    disabled={state.job.state === 'running'} onClick={() => void runtime.start()}>启动 NeoBot</Button>
                    : state.recommended ? <Button type="primary" loading={state.job.state === 'running'}
                        onClick={() => void action(() => Install(state.recommended, false, true))}>安装并启动推荐版本</Button>
                        : <Alert type="info" title="请选择要安装的版本"
                            description="目前没有推荐版本。可安装任意发行版，再到网页里完成配置。"
                            action={<Button onClick={() => navigate('/versions')}>前往版本管理</Button>} />}
            </Space>
        </Card>}
        {configuration && <>
            <Card title="绑定 QQ 官方机器人" extra={<Tag color={qq.connected ? 'green' : 'default'}>
                {qq.connected ? '已连接' : qq.bound ? '已绑定 · 等待连接' : '待绑定'}
            </Tag>}>
                <Space orientation="vertical" size="middle" className="full">
                    {qq.nickname && <Typography.Text>机器人：{qq.nickname}</Typography.Text>}
                    {qrURL && <div className="qr-block">
                        <QRCode value={qrURL} size={200} marginSize={4} />
                        <Typography.Text>用手机 QQ 扫码，约 {qq.expires_in} 秒后过期。</Typography.Text>
                    </div>}
                    {qq.phase === 'expired' && <Alert type="warning" title="二维码已过期，请重新生成" showIcon />}
                    {qq.phase === 'error' && <Alert type="warning" title="二维码生成失败，请检查网络后重试" showIcon />}
                    <Button disabled={busy || state.stopping} loading={saving === 'scan'} onClick={() =>
                        void perform('scan', () => BindQQ(state.setup.revision, Boolean(qrURL)))}>{qrURL ? '刷新二维码' : qq.connected ? '重新绑定' : '扫码绑定'}</Button>
                    <Collapse items={[{key: 'manual', label: '手动配置 AppID / AppSecret', children:
                        <Form form={qqForm} layout="vertical" disabled={busy || state.stopping}
                            onValuesChange={() => { qqRevision.current ??= state.setup.revision }}
                            onFinish={(values) => void perform('qq', async () => {
                            const revision = qqRevision.current ?? state.setup.revision
                            qqRevision.current = undefined
                            await SaveQQ(revision, values.id, values.secret)
                            qqForm.resetFields()
                        })}>
                            <Form.Item label="AppID" name="id" rules={[{required: true}]}><Input maxLength={128} /></Form.Item>
                            <Form.Item label="AppSecret" name="secret" rules={[{required: true}]}><Input.Password maxLength={1024} autoComplete="new-password" /></Form.Item>
                            <Button htmlType="submit" loading={saving === 'qq'}>保存 QQ 配置</Button>
                        </Form>}]}/>
                </Space>
            </Card>
            <Card title="配置回复模型" extra={<Tag color={model.applied ? 'green' : 'default'}>{model.applied ? '已生效' : model.configured ? '已保存' : '待配置'}</Tag>}>
                <Typography.Paragraph>{model.provider || 'DeepSeek'}{model.model_name ? ` · ${model.model_name}` : ''}</Typography.Paragraph>
                <Form form={modelForm} layout="vertical" disabled={busy || state.stopping}
                    onValuesChange={() => {
                        modelRevision.current ??= {config: state.setup.revision, env: state.setup.env_revision}
                    }}
                    onFinish={(values) => void perform('model', async () => {
                    const revision = modelRevision.current ?? {config: state.setup.revision, env: state.setup.env_revision}
                    modelRevision.current = undefined
                    await SaveModel(revision.config, revision.env, values.key, values.url || '', values.name || '')
                    modelForm.resetFields()
                })}>
                    <Form.Item name="key" label="API Key" rules={[{required: true, message: '请粘贴 API Key'}]}>
                        <Input.Password maxLength={2048} placeholder={model.configured ? '已配置，填入新的 Key 可更换' : '粘贴供应商提供的 API Key'} autoComplete="new-password" />
                    </Form.Item>
                    <Collapse items={[{key: 'advanced', label: '高级：自定义供应商和模型', children: <>
                        <Form.Item name="url" label="供应商 HTTPS API 地址"><Input type="url" maxLength={1024} /></Form.Item>
                        <Form.Item name="name" label="模型名"><Input maxLength={256} /></Form.Item>
                    </>}]}/>
                    <Button type="primary" htmlType="submit" loading={saving === 'model'} className="spaced">测试并保存</Button>
                </Form>
            </Card>
            {(qq.needs_restart || model.needs_restart) && <Alert showIcon type="warning" title="配置已保存，需要重启 NeoBot"
                action={<Button loading={saving === 'restart'} disabled={!canRestart} onClick={() => void perform('restart', async () => {
                    await Stop(false)
                    await Start(false)
                })}>重启当前版本</Button>} />}
            {ready && <Alert type="success" title="QQ 已连接，模型配置已生效" description="打开手机 QQ，给机器人发一条消息，确认能收到第一条回复。" showIcon />}
            {state.dashboard_url && <Button className="self-start" onClick={() => void action(OpenDashboard)}>打开网页管理</Button>}
        </>}
    </div>
}
