import {App, Card, Collapse, Form, Input, Radio, Space, Switch, Typography} from 'antd'
import {useEffect, useState} from 'react'
import {SaveSources, SetVersionIsolation} from '../../wailsjs/go/main/App'
import {launcher} from '../../wailsjs/go/models'
import {BrowserOpenURL} from '../../wailsjs/runtime/runtime'
import {useLauncher} from '../app/LauncherProvider'
import {APP_VERSION} from '../app/version'

export function SettingsPage() {
    const {action, state} = useLauncher()
    const {modal} = App.useApp()
    const [form] = Form.useForm<launcher.Sources>()
    const [openKeys, setOpenKeys] = useState<string[]>(state?.sources.preset === 'custom' ? ['sources'] : [])
    const saved = state?.sources
    const preset = Form.useWatch('preset', form) ?? saved?.preset
    // 表单只在挂载时读一次快照；保存后把后端归一化的地址写回输入框，避免继续显示上一个方案的值。
    useEffect(() => {
        if (!saved) return
        form.setFieldsValue({preset: saved.preset, uv: saved.uv, python: saved.python, index: saved.index})
    }, [form, saved?.preset, saved?.uv, saved?.python, saved?.index])
    // 切换方案立即写入：官方源和国内加速由后端重建默认地址，自定义沿用输入框里的值。
    useEffect(() => {
        if (!saved || !preset || preset === saved.preset) return
        save({...form.getFieldsValue(), preset})
    }, [preset, saved?.preset])
    useEffect(() => {
        if (preset === 'custom') setOpenKeys(['sources'])
    }, [preset])
    if (!state) return null

    const downloading = state.job.state === 'running'
    const isolationBusy = state.starting || state.stopping || downloading || Boolean(state.running)
    const isolationHint = state.starting || state.stopping || state.running
        ? '停止 NeoBot 后可修改。'
        : downloading ? '下载任务结束后可修改。' : ''

    function save(values: launcher.Sources) {
        void action(() => SaveSources(launcher.Sources.createFrom(values)))
    }

    // 自定义地址失焦即写入，值没变就不发请求。
    function saveCustom() {
        if (preset !== 'custom' || !saved) return
        const values = form.getFieldsValue()
        if (values.uv === saved.uv && values.python === saved.python && values.index === saved.index) return
        save({...values, preset: 'custom'})
    }

    function toggleIsolation(enabled: boolean) {
        modal.confirm({
            title: enabled ? '开启版本隔离？' : '关闭版本隔离？',
            content: enabled
                ? '开启后每个版本使用独立数据目录，首次启动需要重新配置。现有共用数据不会复制、移动或删除。'
                : '关闭后所有版本重新使用共用数据目录；已有的独立数据会保留但不再使用。',
            okText: enabled ? '开启' : '关闭',
            cancelText: '取消',
            onOk: () => action(() => SetVersionIsolation(enabled)),
        })
    }

    return <div className="page">
        <header className="page-heading"><Typography.Title level={1}>设置</Typography.Title></header>
        <Card title="下载源">
            <Form form={form} initialValues={state.sources} layout="vertical">
                <Form.Item name="preset" label="下载方案">
                    <Radio.Group options={[
                        {value: 'official', label: '官方源'},
                        {value: 'accelerated', label: '国内加速'},
                        {value: 'custom', label: '自定义'},
                    ]} />
                </Form.Item>
                <Collapse activeKey={openKeys} onChange={keys => setOpenKeys(keys)}
                    items={[{key: 'sources', label: '高级下载源', children: <>
                        {preset !== 'accelerated' && <Form.Item name="uv" label="uv 发行包地址前缀"
                            rules={[{required: preset === 'custom'}]}><Input disabled={preset !== 'custom'} onBlur={saveCustom} /></Form.Item>}
                        <Form.Item name="python" label="Python 运行时下载前缀"
                            extra="留空使用 uv 官方默认值。"><Input disabled={preset !== 'custom'} onBlur={saveCustom} /></Form.Item>
                        <Form.Item name="index" label="Python 软件包索引"
                            rules={[{required: preset === 'custom'}]}><Input disabled={preset !== 'custom'} onBlur={saveCustom} /></Form.Item>
                    </>}]} />
            </Form>
        </Card>
        <Card title="数据目录" extra={<Space size="small"><Typography.Text>版本隔离</Typography.Text>
            <Switch aria-label="版本隔离" checked={state.isolate_data} disabled={isolationBusy}
                onChange={toggleIsolation} /></Space>}>
            <div className="data-dir">
                <Typography.Text className="data-path" code copyable>{state.data_dir}</Typography.Text>
                {isolationBusy && <Typography.Text type="secondary">{isolationHint}</Typography.Text>}
            </div>
        </Card>
        <Collapse className="about-collapse" items={[{
            key: 'about',
            label: '关于',
            extra: <Typography.Text type="secondary">v{APP_VERSION}</Typography.Text>,
            children: <Space direction="vertical" size={10}>
                <Space size={8}>
                    <Typography.Text type="secondary">版本号</Typography.Text>
                    <Typography.Text code copyable={{text: APP_VERSION}}>{APP_VERSION}</Typography.Text>
                </Space>
                <Space size={16} wrap>
                    <Typography.Link onClick={() => BrowserOpenURL('https://github.com/SuperQuail/NeoBot')}>项目主页</Typography.Link>
                    <Typography.Link onClick={() => BrowserOpenURL('https://github.com/SuperQuail/neobot-desktop/issues')}>问题反馈</Typography.Link>
                </Space>
            </Space>,
        }]} />
    </div>
}
