import {Fragment, useEffect, useRef} from 'react'
import {App, Button, Card, Switch, Typography} from 'antd'
import {CleanUsage, OpenFolder, RefreshUsage, SetVersionIsolation} from '../../wailsjs/go/main/App'
import {useLauncher} from '../app/LauncherProvider'

const byteUnits = ['B', 'KB', 'MB', 'GB', 'TB']

// 把字节数变成人能读的大小：自动选 GB/MB，0 显示 0 B。
function formatBytes(bytes: number): string {
    if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
    let value = bytes
    let unit = 0
    while (value >= 1024 && unit < byteUnits.length - 1) {
        value /= 1024
        unit += 1
    }
    return `${unit === 0 || value >= 100 ? Math.round(value) : value.toFixed(1)} ${byteUnits[unit]}`
}

function formatScannedAt(at: number): string {
    if (!at) return ''
    const date = new Date(at * 1000)
    if (Number.isNaN(date.getTime())) return ''
    const pad = (value: number) => String(value).padStart(2, '0')
    return `统计于 ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

type CleanTarget = 'cache' | 'tmp' | 'backups'

// 每个清理目标都必须说清删掉什么、什么不受影响、什么后果无法撤销。
const cleanText: Record<CleanTarget, {title: string; content: string; name: string}> = {
    cache: {
        title: '清理下载缓存？',
        content: '将删除安装和更新时下载的软件包缓存。已安装的版本不受影响，下次安装或修复会重新下载依赖。',
        name: '缓存',
    },
    tmp: {
        title: '清理临时文件？',
        content: '将删除安装、更新过程中留下的临时文件。这些文件只在任务进行中才需要，删除后不影响已安装版本。',
        name: '临时文件',
    },
    backups: {
        title: '清理历史快照？',
        content: '将删除切换版本时备份的数据快照。清理后降级时将无法恢复旧版数据，当前数据目录不受影响。',
        name: '快照',
    },
}

// 版本用同一色系的深浅区分；其余分类各占一个色相，色块同时出现在比例条和图例里。
const versionColors = ['#2563eb', '#3b82f6', '#60a5fa', '#93c5fd']
const otherCategories = [
    {key: 'python', label: 'Python 运行时', color: '#8b5cf6'},
    {key: 'cache', label: '下载缓存', color: '#38bdf8'},
    {key: 'data', label: '共用数据', color: '#10b981'},
    {key: 'isolated', label: '隔离数据', color: '#14b8a6'},
    {key: 'logs', label: '日志', color: '#ec4899'},
    {key: 'backups', label: '历史快照', color: '#f59e0b'},
    {key: 'temp', label: '临时文件', color: '#94a3b8'},
    {key: 'tools', label: 'uv 工具', color: '#64748b'},
] as const

type LegendItem = {key: string; label: string; color: string; bytes: number}

export function StorageCard() {
    const {state, action} = useLauncher()
    const {modal, message} = App.useApp()
    // 后端可能因为正在扫描而直接返回（此时 scanned_at 仍是 0），所以不能只请求一次；
    // 但也不能无限重试，限 3 次即可。
    const attempts = useRef(0)
    const usage = state?.usage
    const versions = usage?.versions ?? []
    const scannedAt = usage?.scanned_at ?? 0

    useEffect(() => {
        if (!state || scannedAt !== 0 || usage?.scanning || attempts.current >= 3) return
        attempts.current += 1
        void action(() => RefreshUsage())
    }, [state, scannedAt, usage?.scanning, action])

    const total = usage?.total ?? 0
    const scanning = Boolean(usage?.scanning)
    const installable = state?.job.state === 'running'
    const busyReason = state?.starting ? 'NeoBot 正在启动，请等启动完成后再清理。'
        : state?.stopping ? 'NeoBot 正在停止，请等停止完成后再清理。'
            : state?.running ? '停止 NeoBot 后可清理。'
                : installable ? '安装或更新任务结束后可清理。' : ''
    // 统计进行中也不要清理：清理会立刻再统计一次，但这次统计被后端复用了，
    // 界面会继续显示清理前的数字，容易让人以为没生效。
    const cleaningDisabled = !state || Boolean(state.running) || Boolean(state.starting) ||
        Boolean(state.stopping) || installable || scanning

    const versionLegend: LegendItem[] = versions.map((item, index) => ({
        key: 'version:' + item.name,
        label: item.label || item.name,
        color: versionColors[index % versionColors.length],
        bytes: item.bytes || 0,
    }))
    const otherLegend: LegendItem[] = otherCategories.map(item => ({
        key: item.key,
        label: item.label,
        color: item.color,
        bytes: (usage?.[item.key] as number | undefined) ?? 0,
    }))
    const everything = [...versionLegend, ...otherLegend]
    // 比例条只画有内容的分类；图例把 0 值也列出来，但压成灰色，方便对照「为什么合计是这么多」。
    const drawn = everything.filter(item => item.bytes > 0)
    const cleanable = (usage?.cache ?? 0) + (usage?.temp ?? 0) + (usage?.backups ?? 0)
    const ariaSummary = drawn.map(item => `${item.label} ${formatBytes(item.bytes)}`).join('，') || '暂无内容'

    function clean(target: CleanTarget) {
        const text = cleanText[target]
        modal.confirm({
            title: text.title,
            content: text.content,
            okText: '清理',
            cancelText: '取消',
            okButtonProps: target === 'backups' ? {danger: true} : undefined,
            onOk: () => action(async () => {
                const freed = await CleanUsage(target)
                void message.success(freed > 0
                    ? `已清理${text.name}，释放 ${formatBytes(freed)}`
                    : `没有可清理的${text.name}`)
            }),
        })
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

    const isolationBusy = Boolean(state?.starting || state?.stopping || state?.running) || installable
    const storagePath = state?.data_dir || ''

    return <Card className="stack-card" size="small" title="存储" extra={
        <Button size="small" type="text" loading={scanning}
            onClick={() => void action(() => RefreshUsage())}>刷新</Button>
    }>
        <div className="info-row">
            <div className="info-row-main">
                <div className="info-row-title">数据目录</div>
                <div className="info-row-hint tabular" style={{overflowWrap: 'anywhere'}}>{storagePath}</div>
            </div>
            <div className="info-row-side">
                <Typography.Text copyable={{text: storagePath}} style={{fontSize: 12}} />
                <Button size="small" type="text"
                    onClick={() => void action(() => OpenFolder('data'))}>打开</Button>
            </div>
        </div>
        <div className="info-row">
            <div className="info-row-main">
                <div className="info-row-title">版本隔离</div>
                <div className="info-row-hint">
                    开启后每个版本使用独立数据目录，切版本不会互相改动配置和数据库。
                    {isolationBusy ? '停止 NeoBot 后才能修改。' : ''}
                </div>
            </div>
            <div className="info-row-side">
                <Switch aria-label="版本隔离" checked={Boolean(state?.isolate_data)}
                    disabled={isolationBusy} onChange={toggleIsolation} />
            </div>
        </div>
        <div className="storage-head">
            <div>
                <div className="storage-total tabular">{formatBytes(total)}<small>总计</small></div>
                <div className="storage-sub">
                    {cleanable > 0 ? `其中可清理 ${formatBytes(cleanable)}` : '没有可清理的内容'}
                </div>
            </div>
            <span className="state-dot tone-idle" role="status">
                {scanning ? '正在统计…' : formatScannedAt(scannedAt) || '尚未统计'}
            </span>
        </div>

        {usage?.error && <div className="card-empty" style={{color: '#8a530d'}}>{usage.error}</div>}

        <div className="storage-bar" role="img" aria-label={'存储构成：' + ariaSummary}>
            {drawn.map(item => <Fragment key={item.key}>
                <span style={{width: (item.bytes / Math.max(total, 1)) * 100 + '%', background: item.color}}
                    title={`${item.label} ${formatBytes(item.bytes)}`} />
            </Fragment>)}
        </div>

        <div className="storage-legend">
            {everything.map(item => <div key={item.key}
                className={'storage-legend-row' + (item.bytes > 0 ? '' : ' is-muted')}>
                <span style={{background: item.color}} aria-hidden="true" />
                <em title={item.label}>{item.label}</em>
                <b>{formatBytes(item.bytes)}</b>
            </div>)}
        </div>

        {busyReason && <div className="card-empty" style={{paddingTop: 10}}>{busyReason}</div>}

        <div className="card-actions">
            <Button size="small" disabled={cleaningDisabled} onClick={() => clean('cache')}>清理缓存</Button>
            <Button size="small" disabled={cleaningDisabled} onClick={() => clean('tmp')}>清理临时文件</Button>
            <Button size="small" disabled={cleaningDisabled} onClick={() => clean('backups')}>清理快照</Button>
            <Typography.Text type="secondary" style={{fontSize: 12, marginLeft: 'auto'}}>
                Python 运行时、已装版本与日志不能清理
            </Typography.Text>
        </div>
    </Card>
}
