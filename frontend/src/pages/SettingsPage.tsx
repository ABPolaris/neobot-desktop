import {App, Button, Card, Collapse, Form, Input, Radio, Space, Switch, Typography} from 'antd'
import {SaveSources, SetVersionIsolation} from '../../wailsjs/go/main/App'
import {launcher} from '../../wailsjs/go/models'
import {useLauncher} from '../app/LauncherProvider'

export function SettingsPage() {
    const {action, state} = useLauncher()
    const {modal} = App.useApp()
    const [form] = Form.useForm<launcher.Sources>()
    const preset = Form.useWatch('preset', form) ?? state?.sources.preset
    if (!state) return null
    const downloading = state.job.state === 'running'
    const isolationBusy = state.starting || state.stopping || downloading || Boolean(state.running)
    const isolationHint = state.starting || state.stopping || state.running
        ? '停止 NeoBot 后可修改。'
        : downloading ? '下载任务结束后可修改。' : ''

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
            <Form form={form} initialValues={state.sources} layout="vertical" onFinish={values =>
                void action(() => SaveSources(launcher.Sources.createFrom(values)))}>
                <Form.Item name="preset" label="下载方案">
                    <Radio.Group options={[
                        {value: 'official', label: '官方源'},
                        {value: 'accelerated', label: '国内加速'},
                        {value: 'custom', label: '自定义'},
                    ]} />
                </Form.Item>
                <Collapse defaultActiveKey={state.sources.preset === 'custom' ? ['sources'] : []}
                    items={[{key: 'sources', label: '高级下载源', children: <>
                        {preset !== 'accelerated' && <Form.Item name="uv" label="uv 发行包地址前缀"
                            rules={[{required: preset === 'custom'}]}><Input disabled={preset !== 'custom'} /></Form.Item>}
                        <Form.Item name="python" label="Python 运行时下载前缀"
                            extra="留空使用 uv 官方默认值。"><Input disabled={preset !== 'custom'} /></Form.Item>
                        <Form.Item name="index" label="Python 软件包索引"
                            rules={[{required: preset === 'custom'}]}><Input disabled={preset !== 'custom'} /></Form.Item>
                    </>}]} />
                <Button className="spaced" type="primary" htmlType="submit">保存</Button>
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
    </div>
}
