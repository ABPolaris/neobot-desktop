import {Fragment, useState} from 'react'
import {App, Button, Card, Spin, Tooltip} from 'antd'
import {ExportDiagnostics, OpenFolder, RefreshHealth} from '../../wailsjs/go/main/App'
import {useLauncher} from '../app/LauncherProvider'

// 体检结果的四种状态：颜色和文案集中在这里，条与图例只按 key 取。
const tones: Record<string, {color: string; label: string}> = {
    ok: {color: '#14723a', label: '正常'},
    warn: {color: '#8a530d', label: '注意'},
    fail: {color: '#a92b23', label: '异常'},
    skip: {color: '#94a3b8', label: '已跳过'},
}

export function HealthCard() {
    const {state, action} = useLauncher()
    const {message} = App.useApp()
    const [exporting, setExporting] = useState(false)
    const [bundle, setBundle] = useState('')
    const health = state?.health
    const running = Boolean(health?.running)
    const items = health?.items ?? []
    const checkedAt = health?.at ? new Date(health.at * 1000).toLocaleString() : ''
    const problems = items.filter(item => item.state === 'fail' || item.state === 'warn').length

    // 导出结果的入口只放在标题栏：标题栏高度固定、又总在卡片顶部，
    // 不会像在正文下面追加一行那样把后面的卡片顶下去，也不用滚动才看得见。
    async function exportBundle() {
        if (exporting) return
        setExporting(true)
        try {
            const path = await ExportDiagnostics()
            setBundle(path)
            void message.success('诊断包已导出：' + path.slice(path.search(/[\\/][^\\/]*$/) + 1))
        } catch (failure) {
            void message.error('无法导出诊断包：' + String(failure))
        } finally {
            setExporting(false)
        }
    }

    const bundleName = bundle ? bundle.slice(bundle.search(/[\\/][^\\/]*$/) + 1) : ''

    return <Card className="stack-card" size="small" title="体检" extra={
        <div className="health-actions">
            <Button size="small" loading={running} disabled={running || exporting}
                onClick={() => void action(() => RefreshHealth())}>开始体检</Button>
            <Button size="small" type="text" loading={exporting} disabled={exporting}
                onClick={() => void exportBundle()}>导出诊断包</Button>
            {bundle && <Tooltip title={bundle} placement="bottomRight">
                <Button size="small" type="text" aria-label={'打开诊断包所在文件夹：' + bundleName}
                    onClick={() => void action(() => OpenFolder('diagnostics'))}>打开所在文件夹</Button>
            </Tooltip>}
        </div>
    }>
        {/* 内容区高度固定：没体检、正在体检、有结果三种状态卡片一样高，页面不会跳。 */}
        <div className="health-body">
        {running && <div className="health-empty">
            <Spin size="small" />
            <span>正在检查已安装版本、依赖、NeoBot 自检、本机接口、数据目录和磁盘…</span>
        </div>}

        {!running && health?.error && <div className="health-empty" style={{color: '#a92b23'}}>{health.error}</div>}

        {/* 进行中和结果二选一：两者同时出现时，上一轮结果会被挤到看不见的地方。 */}
        {!running && items.length > 0 && <>
            <div className="health-grid">
                {items.map(item => {
                    const tone = tones[item.state] ?? tones.skip
                    // 正常项只占一行；有问题的项才在下面展开说明和下一步动作。
                    const showDetail = item.state !== 'ok' && item.state !== 'skip'
                    return <Fragment key={item.key}>
                        <div className="health-item">
                            <i style={{background: tone.color}} aria-hidden="true" />
                            <em>{item.title}</em>
                            <span style={{color: tone.color}}>{tone.label}</span>
                        </div>
                        {showDetail && (item.detail || item.hint) && <div className="health-detail">
                            {item.detail}
                            {item.detail && item.hint ? <br /> : null}
                            {item.hint}
                        </div>}
                    </Fragment>
                })}
            </div>
            {checkedAt && <div className="health-meta">
                上次体检：{checkedAt}{problems > 0 ? ` · 有 ${problems} 项需要留意` : ' · 全部正常'}
            </div>}
        </>}

        {items.length === 0 && !running && !health?.error && <div className="health-empty">
            <span>还没有体检结果。启动失败时先跑一次体检，再把诊断包发过来。</span>
        </div>}
        </div>
    </Card>
}
