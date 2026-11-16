import {Button, Card, Select, Tag, Typography} from 'antd'
import {Link, useNavigate} from 'react-router'
import {OpenDashboard, SelectVersion} from '../../wailsjs/go/main/App'
import {useLauncher} from '../app/LauncherProvider'
import {useRuntimeActions} from '../app/actions'
import {JobCard, RuntimeError} from '../components/JobCard'
import {LauncherIcon} from '../components/LauncherIcon'
import {LogCard} from '../components/LogCard'
import {displayVersion, displayVersionName} from '../app/versionLabel'

type Tone = 'ready' | 'pending' | 'idle' | 'error'

function Status({tone, children}: {tone: Tone; children: string}) {
    return <span className={`launcher-status status-${tone}`}><i aria-hidden="true" />{children}</span>
}

function OverviewCard({icon, title, status, tone}: {icon: 'qq' | 'model'; title: string; status: string; tone: Tone}) {
    return <Card size="small" className="overview-card">
        <div className="overview-card-content">
            <span className="overview-icon"><LauncherIcon name={icon} size={27} /></span>
            <div className="overview-details"><h2>{title}</h2><Status tone={tone}>{status}</Status></div>
        </div>
    </Card>
}

export function HomePage() {
    const {state, action} = useLauncher()
    const navigate = useNavigate()
    const runtime = useRuntimeActions()
    if (!state) return null
    const {qq, model} = state.setup
    const active = Boolean(state.running)
    const selectedVersion = state.installed.find(item => item.name === state.selected)
    const branchSharedData = Boolean(selectedVersion?.branch && !state.isolate_data)
    const setupComplete = qq.connected && !qq.needs_restart && model.applied && !model.needs_restart
    const canConfigure = active && !state.starting && !state.stopping && state.setup_supported
    const installing = state.job.state === 'running'
    const showDashboard = Boolean(state.dashboard_url)
    const runtimeTone: Tone = state.runtime_error ? 'error' : state.stopping || state.starting ? 'pending' : active ? 'ready' : 'idle'
    const runtimeStatus = state.stopping ? '正在停止' : state.starting ? '正在启动' : active ? '运行中' : '未运行'
    const waiting = state.starting ? '正在启动' : !active ? '等待启动' : !state.setup_supported ? '请在网页查看' : ''
    const qqStatus = waiting || (qq.needs_restart ? '需重启' : qq.connected ? '已连接' : qq.bound ? '等待连接' : '待绑定')
    const modelStatus = waiting || (model.needs_restart ? '需重启' : model.applied ? '已配置' : model.configured ? '尚未生效' : '待配置')

    return <div className="launcher-home">
        <section className="launch-pane" aria-label="启动控制">
            <div className="launch-emblem"><LauncherIcon name="bot" size={43} /></div>
            <label className="version-label" htmlFor="launch-version">{active && state.running !== state.selected ? '下次启动版本' : '当前版本'}</label>
            <Select id="launch-version" aria-label="选择启动版本" className="launch-select" size="large" virtual={false} value={state.selected || undefined}
                placeholder="尚未选择版本" disabled={state.starting || state.stopping || installing || !state.installed.length}
                onChange={version => void action(() => SelectVersion(version))}
                options={state.installed.map(item => ({value: item.name, label: <span className="version-option">
                    <span>{displayVersion(item)}</span>{item.branch
                        ? <Tag color="purple">GitHub</Tag>
                        : <Tag color={item.preview ? 'orange' : 'green'}>{item.preview ? '预览版' : '稳定版'}</Tag>}
                </span>}))} />
            <div className="launch-state" role="status"><Status tone={runtimeTone}>{runtimeStatus}</Status>
                {active && state.running !== state.selected && <span className="running-version">
                    正在运行 {displayVersionName(state.running, state.installed)}
                </span>}
            </div>
            {branchSharedData && <Typography.Text type="warning" className="branch-data-warning">
                分支版本与正式版共用数据目录，启动时可能迁移配置或数据库。<Link to="/settings">去设置</Link>
            </Typography.Text>}
            {!state.selected && <Link className="installed-link" to="/versions">前往版本管理</Link>}

            <div className="launch-actions">
                {!active && (!state.selected
                    ? <Button type="primary" size="large" onClick={() => navigate('/setup')}>一键准备</Button>
                    : <Button type="primary" size="large" loading={state.starting} disabled={installing}
                        onClick={() => void runtime.start()}>启动 NeoBot</Button>)}
                {active && <>
                    {showDashboard && <Button type="primary" size="large" onClick={() => void action(OpenDashboard)}>打开网页管理</Button>}
                    {!showDashboard && canConfigure && <Button type="primary" size="large" onClick={() => navigate('/setup')}>
                        {setupComplete ? '修改 QQ 或模型' : '继续快捷配置'}
                    </Button>}
                    <Button size="large" className="stop-button" loading={state.stopping} onClick={() => void runtime.stop()}>停止运行</Button>
                    {state.runtime_error && <Button danger onClick={runtime.forceStop}>强制停止</Button>}
                </>}
            </div>
        </section>
        <section className="launch-overview" aria-labelledby="overview-heading">
            <div className="overview-heading">
                <h1 id="overview-heading">运行概览</h1>
                {showDashboard && canConfigure && <Link to="/setup">{setupComplete ? '修改 QQ 或模型' : '继续快捷配置'}</Link>}
            </div>
            <div className="overview-grid">
                <OverviewCard icon="qq" title="QQ 官方机器人" status={qqStatus}
                    tone={waiting ? 'idle' : qq.needs_restart ? 'pending' : qq.connected ? 'ready' : 'pending'} />
                <OverviewCard icon="model" title="回复模型" status={modelStatus}
                    tone={waiting ? 'idle' : model.needs_restart ? 'pending' : model.applied ? 'ready' : 'pending'} />
            </div>
            <RuntimeError />
            <JobCard />
            <LogCard key={state.log_run} running={active || state.starting} />
        </section>
    </div>
}
