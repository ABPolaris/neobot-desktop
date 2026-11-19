import {useState} from 'react'
import {Alert, Button, Card, Progress, Space} from 'antd'
import {Link, useNavigate} from 'react-router'
import {Install, InstallBranch, RefreshHealth} from '../../wailsjs/go/main/App'
import {useLauncher} from '../app/LauncherProvider'
import {displayBranch} from '../app/versionLabel'

export function JobCard() {
    const {state, action} = useLauncher()
    const [retrying, setRetrying] = useState(false)
    const job = state?.job
    if (!job?.state) return null
    const sourceJob = Boolean(job.branch || job.repository)
    const label = sourceJob ? displayBranch(job.branch, job.repository) : job.version

    async function retry(work: () => Promise<void>) {
        if (retrying) return
        setRetrying(true)
        try { await action(work) } finally { setRetrying(false) }
    }

    return <Card title={<span title={label}>准备 NeoBot {label}</span>} size="small">
        <Space orientation="vertical" size="middle" className="full">
            <Progress percent={Math.round(job.completed * 100 / job.total)} status={job.state === 'failed' ? 'exception' : job.state === 'running' ? 'active' : 'success'} />
            <div role="status">{job.message || '正在准备安装'}</div>
            {job.state === 'failed' && <Space wrap>
                {sourceJob
                    ? <Button loading={retrying} onClick={() => void retry(() => InstallBranch(job.repository || '', job.branch || '', job.start_after, true))}>重试</Button>
                    : <>
                        <Button disabled={retrying} onClick={() => void retry(() => Install(job.version, false, job.start_after))}>重试</Button>
                        <Button disabled={retrying} onClick={() => void retry(() => Install(job.version, true, job.start_after))}>用官方源重试</Button>
                        <Link to="/settings">更换下载源</Link>
                    </>}
            </Space>}
        </Space>
    </Card>
}

export function RuntimeError() {
    const {state, action} = useLauncher()
    const navigate = useNavigate()
    if (!state?.runtime_error) return null
    // 出问题时把「体检」直接推到眼前：诊断页平时不需要用户主动去想。
    return <Alert type="error" showIcon title={state.runtime_error}
        action={<Button size="small" onClick={() => {
            void action(() => RefreshHealth())
            navigate('/diagnostics')
        }}>运行体检</Button>} />
}
