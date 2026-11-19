import {App, Card, Collapse, Form, Input, Radio, Typography} from 'antd'
import {BrowserOpenURL} from '../../wailsjs/runtime/runtime'
import {useEffect, useState} from 'react'
import {SaveSources} from '../../wailsjs/go/main/App'
import {launcher} from '../../wailsjs/go/models'
import {useLauncher} from '../app/LauncherProvider'
import {SupervisorCard} from '../components/SupervisorCard'
import {UpdateCard} from '../components/UpdateCard'

export function SettingsPage() {
    const {action, state} = useLauncher()
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

    return <div className="page">
        <header className="page-heading"><Typography.Title level={1}>设置</Typography.Title></header>
        <Card className="form-card" title="下载源">
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
        <SupervisorCard />
        <UpdateCard />
        {/* 「关于」类信息统一放页脚：不属于任何一张卡片的操作流。 */}
        <footer className="settings-footer">
            <Typography.Link onClick={() => BrowserOpenURL('https://github.com/SuperQuail/neobot-desktop')}>
                启动器主页
            </Typography.Link>
            <Typography.Link onClick={() => BrowserOpenURL('https://github.com/SuperQuail/neobot-desktop/issues')}>
                问题反馈
            </Typography.Link>
            <Typography.Link onClick={() => BrowserOpenURL('https://github.com/SuperQuail/NeoBot')}>
                NeoBot 项目
            </Typography.Link>
            {state.update.page_url && <Typography.Link
                onClick={() => BrowserOpenURL(state.update.page_url)}>最新发布页</Typography.Link>}
        </footer>
    </div>
}
