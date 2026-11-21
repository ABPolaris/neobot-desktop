import {useState} from 'react'
import {Button, Typography} from 'antd'
import {AdvancedCard} from '../components/AdvancedCard'
import {HealthCard} from '../components/HealthCard'
import {StorageCard} from '../components/StorageCard'
import {LogViewer} from '../components/LogViewer'

// 诊断页只放「机器现在怎么样」的东西：体检、磁盘占用、完整日志。
// 「怎么跑」的配置留在设置页，「跑没跑」留在启动页。
export function DiagnosticsPage() {
    const [logsOpen, setLogsOpen] = useState(false)
    return <div className="page">
        <header className="page-heading">
            <div>
                <Typography.Title level={1}>诊断</Typography.Title>
                <Typography.Text type="secondary">启动失败、依赖异常或磁盘告急时，从这里开始排查。</Typography.Text>
            </div>
            <Button onClick={() => setLogsOpen(true)}>查看完整日志</Button>
        </header>
        <HealthCard />
        <StorageCard />
        <AdvancedCard />
        <LogViewer open={logsOpen} onClose={() => setLogsOpen(false)} />
    </div>
}
