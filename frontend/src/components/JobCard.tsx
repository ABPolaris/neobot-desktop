import {Alert, Button, Card, Progress, Space} from 'antd'
import {Link} from 'react-router'
import {Install, InstallBranch} from '../../wailsjs/go/main/App'
import {useLauncher} from '../app/LauncherProvider'

export function JobCard() {
    const {state, action} = useLauncher()
    const job = state?.job
    if (!job?.state) return null
    return <Card title={`准备 NeoBot ${job.branch ? '分支 ' + job.branch : job.version}`} size="small">
        <Space orientation="vertical" size="middle" className="full">
            <Progress percent={Math.round(job.completed * 100 / job.total)} status={job.state === 'failed' ? 'exception' : job.state === 'running' ? 'active' : 'success'} />
            <div role="status">{job.message || '正在准备安装'}</div>
            {job.state === 'failed' && <Space wrap>
                {job.branch
                    ? <Button onClick={() => void action(() => InstallBranch(job.branch!, job.start_after, true))}>重试</Button>
                    : <>
                        <Button onClick={() => void action(() => Install(job.version, false, job.start_after))}>重试</Button>
                        <Button onClick={() => void action(() => Install(job.version, true, job.start_after))}>用官方源重试</Button>
                        <Link to="/settings">更换下载源</Link>
                    </>}
            </Space>}
        </Space>
    </Card>
}

export function RuntimeError() {
    const {state} = useLauncher()
    return state?.runtime_error ? <Alert type="error" showIcon title={state.runtime_error} /> : null
}
