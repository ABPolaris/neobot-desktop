import {Fragment, useEffect, useLayoutEffect, useRef, useState} from 'react'
import {App, Button, Card, Checkbox, Typography} from 'antd'
import {GetLogs} from '../../wailsjs/go/main/App'
import {ClipboardSetText} from '../../wailsjs/runtime/runtime'
import type {launcher} from '../../wailsjs/go/models'
import {LauncherIcon} from './LauncherIcon'

const logLevels = /^(?:(\d{2}:\d{2}:\d{2}(?:\.\d+)?|\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[+-]\d{2}:?\d{2}|Z)?)([\t |]+))?(TRACE|DEBUG|INFO|SUCCESS|WARN(?:ING)?|ERROR|CRITICAL)(?=[\t |:]|$)/

function LogLine({line}: {line: string}) {
    const match = line.match(logLevels)
    if (!match) return line
    return <>{match[1] && <><span className="log-time">{match[1]}</span>{match[2]}</>}
        <span className={`log-level log-level-${match[3].toLowerCase()}`}>{match[3]}</span>{line.slice(match[0].length)}</>
}

export function LogCard({running}: {running: boolean}) {
    const [logs, setLogs] = useState<launcher.LogSnapshot>()
    const [unavailable, setUnavailable] = useState(false)
    const [follow, setFollow] = useState(true)
    const region = useRef<HTMLDivElement>(null)
    const {message} = App.useApp()
    const lines = logs?.lines ?? []
    const gap = logs?.omitted ? `── 已省略中间 ${logs.omitted} 行，可在网页管理查看详细日志 ──` : ''

    async function copyLogs() {
        const text = gap && logs ? [...lines.slice(0, logs.gap_after), gap, ...lines.slice(logs.gap_after)].join('\n') : lines.join('\n')
        try {
            if (!await ClipboardSetText(text)) throw new Error('clipboard unavailable')
        } catch {
            try { await navigator.clipboard.writeText(text) }
            catch { void message.error('无法复制日志，可选中文字后手动复制。'); return }
        }
        void message.success('已复制当前日志')
    }

    useEffect(() => {
        let cancelled = false
        let revision = 0
        let timer: ReturnType<typeof setTimeout>
        async function read() {
            try {
                const next = await GetLogs(revision)
                if (cancelled) return
                if (revision === 0 || next.revision !== revision) {
                    revision = next.revision
                    setLogs(next)
                }
                setUnavailable(false)
            } catch {
                if (!cancelled) setUnavailable(true)
            } finally {
                if (!cancelled && running) timer = setTimeout(read, 500)
            }
        }
        void read()
        return () => {
            cancelled = true
            clearTimeout(timer)
        }
    }, [running])

    useLayoutEffect(() => {
        if (follow && region.current) region.current.scrollTop = region.current.scrollHeight
    }, [logs, follow])

    return <Card size="small" title={<h2>日志</h2>} className="log-card" extra={
        <div className="log-toolbar">
            <Checkbox checked={follow} disabled={!lines.length} onChange={event => setFollow(event.target.checked)}>跟随最新</Checkbox>
            <Button type="text" size="small" icon={<LauncherIcon name="copy" size={16} />} disabled={!lines.length}
                onClick={() => void copyLogs()}>复制</Button>
        </div>
    }>
        {logs?.version && <Typography.Text className="log-version" type="secondary">
            NeoBot {logs.branch ? '分支 ' + logs.branch : logs.version}
        </Typography.Text>}
        {unavailable && <Typography.Text type="warning" role="status">暂时无法读取日志。</Typography.Text>}
        <div className="log-output" role="region" aria-label="NeoBot 日志" tabIndex={0} ref={region}
            onScroll={event => {
                const el = event.currentTarget
                if (follow && el.scrollHeight - el.clientHeight - el.scrollTop > 24) setFollow(false)
            }}>
            {lines.length ? <pre>{lines.map((line, i) => <Fragment key={i}>
                {gap && i === logs?.gap_after && <span className="log-gap">{gap}{'\n'}</span>}
                <LogLine line={line} />{i < lines.length - 1 ? '\n' : ''}
            </Fragment>)}</pre> : <p>{running ? '正在等待 NeoBot 输出…' : '启动 NeoBot 后，日志会显示在这里。'}</p>}
        </div>
    </Card>
}
