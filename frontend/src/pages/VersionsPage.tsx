import {useEffect, useState} from 'react'
import {Alert, Button, Card, Collapse, Empty, List, Space, Tag, Typography} from 'antd'
import {Install, InstallBranch, ListVersions, SelectVersion} from '../../wailsjs/go/main/App'
import type {launcher} from '../../wailsjs/go/models'
import {useLauncher} from '../app/LauncherProvider'
import {displayBranch} from '../app/versionLabel'
import {GitHubInstallModal} from '../components/GitHubInstallModal'
import {JobCard} from '../components/JobCard'
import {LauncherIcon} from '../components/LauncherIcon'
import {VersionActions} from '../components/VersionActions'

export function VersionsPage() {
    const {state, action} = useLauncher()
    const [available, setAvailable] = useState<launcher.Version[]>([])
    const [loading, setLoading] = useState(true)
    const [error, setError] = useState('')
    const [attempt, setAttempt] = useState(0)
    const [installOpen, setInstallOpen] = useState(false)

    useEffect(() => {
        let live = true
        setLoading(true)
        setError('')
        ListVersions().then(
            versions => { if (live) setAvailable(versions || []) },
            failure => { if (live) setError(String(failure)) },
        ).finally(() => { if (live) setLoading(false) })
        return () => { live = false }
    }, [attempt])

    if (!state) return null
    const snapshot = state
    const versions = available.filter(v => !state.installed.some(i => i.name === v.name))
    const downloading = state.job.state === 'running'

    function list(entries: launcher.Version[], installed: boolean) {
        return <List dataSource={entries} locale={{emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={installed ? '尚未安装版本' : '没有可下载版本'} />}}
            renderItem={item => {
                const actions = []
                if (installed && item.branch) {
                    actions.push(<Button key="update"
                        disabled={downloading || snapshot.starting || snapshot.stopping || snapshot.running === item.name}
                        onClick={() => void action(() => InstallBranch(item.repository || '', item.branch!, false, true))}>更新</Button>)
                }
                actions.push(installed
                    ? <Button key="select" disabled={snapshot.selected === item.name || snapshot.starting || downloading}
                        onClick={() => void action(() => SelectVersion(item.name))}>设为启动版本</Button>
                    : <Button key="download" disabled={downloading}
                        onClick={() => void action(() => Install(item.name, false, false))}>下载</Button>)
                if (installed) {
                    // 修复要重装依赖、卸载会删目录：两者都不能和运行中的实例或任务并发。
                    actions.push(<VersionActions key="manage" item={item}
                        disabled={downloading || snapshot.starting || snapshot.stopping || snapshot.running === item.name} />)
                }
                return <List.Item actions={actions}>
                    <Space wrap>
                        <strong className="version-name">{item.branch ? displayBranch(item.branch, item.repository) : item.name}</strong>
                        {item.branch && <Tag color="purple">GitHub</Tag>}
                        {item.preview && <Tag color="orange">预览版</Tag>}
                        {snapshot.selected === item.name && <Tag color="blue">下次启动</Tag>}
                        {snapshot.running === item.name && <Tag color="green">正在运行</Tag>}
                    </Space>
                </List.Item>
            }} />
    }

    return <div className="page">
        <header className="page-heading versions-heading"><div><Typography.Title level={1}>版本管理</Typography.Title>
            <Typography.Text type="secondary">切换运行版本前请先停止 NeoBot。</Typography.Text></div>
            <Button type="primary" icon={<LauncherIcon name="download" size={16} />}
                onClick={() => setInstallOpen(true)}>从 GitHub 安装</Button>
        </header>
        <GitHubInstallModal open={installOpen} snapshot={state} onClose={() => setInstallOpen(false)} />
        <JobCard />
        <Card title="已安装">{list(state.installed, true)}</Card>
        <Card title="可下载的稳定版" loading={loading} extra={<Button onClick={() => setAttempt(n => n + 1)} disabled={loading}>刷新</Button>}>
            {error ? <Alert type="warning" title={error} /> : list(versions.filter(v => !v.preview), false)}
        </Card>
        {!error && !loading && <Collapse items={[{key: 'preview', label: '预览版', children: list(versions.filter(v => v.preview), false)}]} />}
    </div>
}
