import {useEffect, useLayoutEffect, useRef, useState} from 'react'
import {Alert, Button, Drawer, Empty, Select, Space, Spin, Typography} from 'antd'
import {HostLogFiles, ReadHostLog} from '../../wailsjs/go/main/App'
import type {launcher} from '../../wailsjs/go/models'

// 一次读多少行：第一次打开读最新的一页，「加载更早」每次再往前一页。
const PAGE = 500

function formatBytes(bytes: number): string {
    if (!bytes || bytes < 1024) return (bytes || 0) + ' B'
    const units = ['KB', 'MB', 'GB', 'TB']
    let value = bytes
    let unit = ''
    for (const next of units) {
        value = value / 1024
        unit = next
        if (value < 1024) break
    }
    return value.toFixed(1) + ' ' + unit
}

export function LogViewer({open, onClose}: {open: boolean; onClose: () => void}) {
    const [files, setFiles] = useState<launcher.HostLogFile[]>([])
    const [file, setFile] = useState('')
    const [lines, setLines] = useState<string[]>([])
    const [start, setStart] = useState(0)
    const [total, setTotal] = useState(0)
    const [loading, setLoading] = useState(false)
    const [loadingOlder, setLoadingOlder] = useState(false)
    const [error, setError] = useState('')
    const region = useRef<HTMLDivElement>(null)
    // 读完一页后怎么挪滚动条：新打开要贴底，往前翻页要保持正在看的内容不动。
    const pending = useRef<'bottom' | 'keep' | null>(null)
    const previousHeight = useRef(0)
    // 换文件时作废还在路上的请求，避免旧文件的内容盖到新文件上。
    const request = useRef(0)

    useEffect(() => {
        if (!open) return
        let live = true
        setError('')
        HostLogFiles().then(list => {
            if (!live) return
            const entries = list || []
            setFiles(entries)
            setFile(current => entries.some(item => item.name === current) ? current : entries[0]?.name || '')
        }, failure => {
            if (live) setError(String(failure))
        })
        return () => {
            live = false
        }
    }, [open])

    useEffect(() => {
        const id = ++request.current
        pending.current = null
        setLines([])
        setStart(0)
        setTotal(0)
        setLoadingOlder(false)
        setLoading(open && Boolean(file))
        if (!open || !file) return
        setError('')
        ReadHostLog(file, -1, PAGE).then(page => {
            if (id !== request.current) return
            pending.current = 'bottom'
            setLines(page.lines || [])
            setStart(page.start)
            setTotal(page.total)
        }, failure => {
            if (id === request.current) setError(String(failure))
        }).finally(() => {
            if (id === request.current) setLoading(false)
        })
        return () => { request.current++ }
    }, [open, file])

    useLayoutEffect(() => {
        const element = region.current
        if (!element || !pending.current) return
        if (pending.current === 'bottom') element.scrollTop = element.scrollHeight
        else element.scrollTop += element.scrollHeight - previousHeight.current
        pending.current = null
    }, [lines])

    async function loadOlder() {
        if (!file || start <= 0 || loadingOlder) return
        const id = request.current
        setLoadingOlder(true)
        previousHeight.current = region.current?.scrollHeight ?? 0
        try {
            const count = Math.min(PAGE, start)
            const page = await ReadHostLog(file, start - count, count)
            if (id !== request.current) return
            pending.current = 'keep'
            setLines(current => [...(page.lines || []), ...current])
            setStart(page.start)
            setTotal(page.total)
        } catch (failure) {
            if (id === request.current) setError(String(failure))
        } finally {
            if (id === request.current) setLoadingOlder(false)
        }
    }

    return <Drawer open={open} onClose={onClose} width="min(1080px, 94vw)" destroyOnHidden
        title={<Space size={10} align="center" wrap>
            <span>宿主日志</span>
            <Select size="small" value={file || undefined} disabled={files.length === 0} virtual={false}
                aria-label="选择日志文件" placeholder={files.length ? '选择日志文件' : '暂无日志'}
                style={{minWidth: 240, maxWidth: 460}} onChange={value => setFile(value)}
                options={files.map(item => ({
                    value: item.name,
                    title: item.name,
                    label: item.name + '（' + formatBytes(item.bytes) + '）',
                }))} />
        </Space>}
        extra={<Space size={8} wrap align="center">
            {total > 0 && <Typography.Text type="secondary" style={{fontSize: 12}}>
                共 {total} 行{lines.length < total ? '，已加载 ' + lines.length + ' 行' : ''}
            </Typography.Text>}
            <Button size="small" loading={loadingOlder} disabled={loadingOlder || loading || start <= 0}
                onClick={() => void loadOlder()}>加载更早</Button>
        </Space>}>
        <Space orientation="vertical" size={10} style={{width: '100%'}}>
            {error && <Alert type="warning" showIcon title="日志读取失败" description={error} />}
            {loading && lines.length === 0
                ? <Space align="center" size={8}><Spin size="small" /><Typography.Text type="secondary">正在读取日志…</Typography.Text></Space>
                : lines.length === 0
                    ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE}
                        description={files.length ? '这个日志文件还是空的' : '还没有 NeoBot 日志，启动一次 NeoBot 后这里会出现'} />
                    : <div ref={region} role="region" aria-label="宿主日志内容" tabIndex={0} style={{
                        height: 'calc(100dvh - 190px)', overflow: 'auto', overscrollBehavior: 'contain',
                        padding: '12px 14px', border: '1px solid #d7e5f3', borderRadius: 6,
                        background: '#edf4fb', color: '#18263b',
                    }}>
                        <pre style={{
                            margin: 0, fontFamily: 'Consolas, "Cascadia Mono", "Microsoft YaHei UI", monospace',
                            fontSize: 12, lineHeight: 1.7, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere',
                        }}>{lines.join('\n')}</pre>
                    </div>}
        </Space>
    </Drawer>
}
