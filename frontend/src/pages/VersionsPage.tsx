import {useEffect, useState} from 'react'
import {Alert, AutoComplete, Button, Card, Collapse, Empty, List, Space, Tag, Typography} from 'antd'
import {Install, InstallBranch, ListGitHubBranches, ListVersions, SelectVersion} from '../../wailsjs/go/main/App'
import type {launcher} from '../../wailsjs/go/models'
import {useLauncher} from '../app/LauncherProvider'
import {JobCard} from '../components/JobCard'

export function VersionsPage() {
    const {state, action} = useLauncher()
    const [available, setAvailable] = useState<launcher.Version[]>([])
    const [loading, setLoading] = useState(true)
    const [error, setError] = useState('')
    const [attempt, setAttempt] = useState(0)
    const [branches, setBranches] = useState<launcher.Branch[]>([])
    const [branch, setBranch] = useState('')
    const [branchesLoading, setBranchesLoading] = useState(false)
    const [branchesError, setBranchesError] = useState('')
    const [branchAttempt, setBranchAttempt] = useState(0)
    const ready = Boolean(state)

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

    useEffect(() => {
        if (!ready) return
        let live = true
        setBranchesLoading(true)
        setBranchesError('')
        ListGitHubBranches(branchAttempt > 0).then(
            items => { if (live) setBranches(items || []) },
            failure => { if (live) setBranchesError(String(failure)) },
        ).finally(() => { if (live) setBranchesLoading(false) })
        return () => { live = false }
    }, [branchAttempt, ready])

    if (!state) return null
    const snapshot = state
    const versions = available.filter(v => !state.installed.some(i => i.name === v.name))
    const downloading = state.job.state === 'running'
    const branchName = branch.trim()
    const installedBranch = state.installed.find(item => item.branch === branchName)
    const branchRunning = Boolean(installedBranch && snapshot.running === installedBranch.name)
    const branchJob = downloading && snapshot.job.branch === branchName
    const installedBranches = new Set(state.installed.flatMap(item => item.branch ? [item.branch] : []))

    function installBranch() {
        const name = branchName
        if (!name) return
        const installed = snapshot.installed.some(item => item.branch === name)
        void action(() => InstallBranch(name, false, installed))
    }

    function list(entries: launcher.Version[], installed: boolean) {
        return <List dataSource={entries} locale={{emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={installed ? '尚未安装版本' : '没有可下载版本'} />}}
            renderItem={item => {
                const actions = []
                if (installed && item.branch) {
                    actions.push(<Button key="update"
                        disabled={downloading || snapshot.starting || snapshot.stopping || snapshot.running === item.name}
                        onClick={() => void action(() => InstallBranch(item.branch!, false, true))}>更新</Button>)
                }
                actions.push(installed
                    ? <Button key="select" disabled={snapshot.selected === item.name || snapshot.starting || downloading}
                        onClick={() => void action(() => SelectVersion(item.name))}>设为启动版本</Button>
                    : <Button key="download" disabled={downloading}
                        onClick={() => void action(() => Install(item.name, false, false))}>下载</Button>)
                return <List.Item actions={actions}>
                    <Space wrap>
                        <strong>{item.branch ? '分支 ' + item.branch : item.name}</strong>
                        {item.branch && <Tag color="purple">GitHub</Tag>}
                        {item.preview && <Tag color="orange">预览版</Tag>}
                        {snapshot.selected === item.name && <Tag color="blue">下次启动</Tag>}
                        {snapshot.running === item.name && <Tag color="green">正在运行</Tag>}
                    </Space>
                </List.Item>
            }} />
    }

    return <div className="page">
        <header className="page-heading"><div><Typography.Title level={1}>版本管理</Typography.Title>
            <Typography.Text type="secondary">切换运行版本前请先停止 NeoBot。</Typography.Text></div></header>
        <JobCard />
        <Card title="已安装">{list(state.installed, true)}</Card>
        <Card title="从 GitHub 分支安装" extra={<Button onClick={() => setBranchAttempt(n => n + 1)}
            disabled={branchesLoading || downloading}>刷新列表</Button>}>
            <Space orientation="vertical" size="middle" className="full">
                <Space.Compact className="full">
                    <AutoComplete className="branch-input" value={branch} options={branches.map(item => ({
                        value: item.name,
                        label: <Space size={6}>
                            <span>{item.name}</span>
                            {item.commit && <Typography.Text type="secondary">{item.commit.slice(0, 7)}</Typography.Text>}
                            {installedBranches.has(item.name) && <Tag color="blue">已安装</Tag>}
                        </Space>,
                    }))} filterOption={(input, option) =>
                        String(option?.value ?? '').toLowerCase().includes(input.toLowerCase())}
                        onChange={value => setBranch(value ?? '')} allowClear
                        placeholder="选择或输入分支名（例如 main、dev）"
                        disabled={downloading || snapshot.starting || snapshot.stopping} />
                    <Button type="primary" loading={branchJob}
                        disabled={!branchName || downloading || snapshot.starting || snapshot.stopping || branchRunning}
                        onClick={installBranch}>{installedBranch ? '更新并重新安装' : '下载并安装'}</Button>
                </Space.Compact>
                {branchesLoading && <Typography.Text type="secondary">正在读取分支列表…</Typography.Text>}
                {branchesError && <Alert type="warning" showIcon title="无法获取分支列表"
                    description={branchesError + '。仍可直接输入分支名。'}
                    action={<Button size="small" onClick={() => setBranchAttempt(n => n + 1)}>重试</Button>} />}
                {branchRunning && <Alert type="info" showIcon title="该分支正在运行，停止后才能更新。" />}
            </Space>
        </Card>
        <Card title="可下载的稳定版" loading={loading} extra={<Button onClick={() => setAttempt(n => n + 1)} disabled={loading}>刷新</Button>}>
            {error ? <Alert type="warning" title={error} /> : list(versions.filter(v => !v.preview), false)}
        </Card>
        {!error && !loading && <Collapse items={[{key: 'preview', label: '预览版', children: list(versions.filter(v => v.preview), false)}]} />}
    </div>
}
