import {useEffect, useRef, useState} from 'react'
import {Alert, AutoComplete, Button, Flex, Form, Input, Modal, Radio, Space, Tag, Typography} from 'antd'
import {InstallBranch, ListGitHubBranches} from '../../wailsjs/go/main/App'
import type {launcher} from '../../wailsjs/go/models'
import {useLauncher} from '../app/LauncherProvider'
import {MAIN_REPOSITORY, sameRepository} from '../app/versionLabel'

type Fields = {repository?: string; branch?: string}
type BranchRequest = {repository: string; force: boolean}
type BranchResult = {request: BranchRequest; branches?: launcher.Branch[]; error?: string}

function parseRepository(value = ''): string | undefined {
    const match = value.trim().match(/^(?:https:\/\/github\.com\/)?([a-z0-9_-]{1,39})\/([a-z0-9._-]{1,104})\/?$/i)
    if (!match) return undefined
    const name = match[2].replace(/\.git$/i, '')
    if (name.length > 100 || !/[^.]/.test(name) || /\.git$/i.test(name)) return undefined
    return match[1] + '/' + name
}

export function GitHubInstallModal({open, snapshot, onClose}: {
    open: boolean; snapshot: launcher.Snapshot; onClose: () => void
}) {
    const {refresh} = useLauncher()
    const [form] = Form.useForm<Fields>()
    const [source, setSource] = useState<'main' | 'custom'>('main')
    const address = Form.useWatch('repository', form)?.trim() || ''
    const branch = Form.useWatch('branch', form)?.trim() || ''
    const repository = source === 'custom' ? address : ''
    const repositoryKey = source === 'custom' ? parseRepository(address) : MAIN_REPOSITORY
    const [request, setRequest] = useState<BranchRequest>()
    const [result, setResult] = useState<BranchResult>()
    const [installError, setInstallError] = useState('')
    const [submitting, setSubmitting] = useState(false)
    const submitLock = useRef(false)

    useEffect(() => {
        if (!open || !request) return
        let live = true
        ListGitHubBranches(request.repository, request.force).then(
            branches => { if (live) setResult({request, branches: branches || []}) },
            failure => { if (live) setResult({request, error: String(failure)}) },
        )
        return () => { live = false }
    }, [open, request])

    function loadBranches(force: boolean) {
        const value: string = source === 'custom' ? form.getFieldValue('repository')?.trim() || '' : ''
        if (source === 'custom' && !parseRepository(value)) return
        setRequest(previous => !force && previous?.repository === value ? previous : {repository: value, force})
    }

    const currentRequest = request?.repository === repository ? request : undefined
    const list = result?.request === currentRequest ? result : undefined
    const loadingList = Boolean(currentRequest && !list)
    const installed = new Map<string, string>()
    for (const item of snapshot.installed) {
        if (repositoryKey && item.branch && sameRepository(item.repository, repositoryKey)) {
            installed.set(item.branch, item.name)
        }
    }
    const target = installed.get(branch)
    const running = Boolean(target && target === snapshot.running)
    const blocked = snapshot.job.state === 'running' ? '请等待当前安装任务结束。'
        : snapshot.starting || snapshot.stopping ? '请等待 NeoBot 完成启动或停止。'
        : running ? '该分支正在运行，请停止后再更新。' : ''

    function clearBranch() {
        form.setFieldValue('branch', '')
        setRequest(undefined)
    }

    function close() {
        setSource('main')
        form.resetFields()
        setRequest(undefined)
        setInstallError('')
        onClose()
    }

    async function install(values: Fields) {
        const repo = source === 'custom' ? values.repository?.trim() : ''
        if (repo === undefined || blocked || submitLock.current) return
        submitLock.current = true
        setSubmitting(true)
        setInstallError('')
        try {
            await InstallBranch(repo, values.branch?.trim() || '', false, true)
            close()
        } catch (failure) {
            setInstallError(String(failure))
        } finally {
            submitLock.current = false
            setSubmitting(false)
            void refresh()
        }
    }

    return <Modal open={open} title="从 GitHub 安装" destroyOnHidden
        onCancel={() => { if (!submitLock.current) close() }} cancelText="取消"
        closable={!submitting} cancelButtonProps={{disabled: submitting}} confirmLoading={submitting}
        okText={target ? '更新分支' : branch ? '下载并安装' : '安装或更新'}
        okButtonProps={{form: 'github-install', htmlType: 'submit', disabled: Boolean(blocked) || submitting}}>
        <Form name="github-install" form={form} layout="vertical" disabled={submitting} scrollToFirstError={{focus: true}}
            onFinish={install} onValuesChange={(changed: Partial<Fields>) => {
                setInstallError('')
                if ('repository' in changed) clearBranch()
            }}>
            <Form.Item label="仓库来源" htmlFor="github-install-main">
                <Radio.Group name="github-install-source" aria-label="仓库来源" value={source} onChange={event => {
                    setSource(event.target.value)
                    setInstallError('')
                    clearBranch()
                }}>
                    <Radio id="github-install-main" value="main">主仓库（{MAIN_REPOSITORY}）</Radio>
                    <Radio value="custom">自定义仓库</Radio>
                </Radio.Group>
            </Form.Item>
            {source === 'custom' && <Form.Item name="repository" label="GitHub 仓库" required
                extra="支持公开的 NeoBot fork 仓库。" validateTrigger="onBlur"
                rules={[{validator: (_, value) => parseRepository(value) ? Promise.resolve()
                    : Promise.reject(new Error('请输入 GitHub 仓库的 HTTPS 地址或 owner/repo。'))}]}>
                <Input placeholder="https://github.com/owner/repo 或 owner/repo" autoComplete="off" spellCheck={false} allowClear />
            </Form.Item>}
            <Form.Item name="branch" label="分支" extra={<Flex align="center" justify="space-between" wrap gap={8}>
                <span>留空使用该仓库的默认分支。</span>
                <Button type="link" size="small" loading={loadingList} disabled={submitting || !repositoryKey || loadingList}
                    onClick={() => loadBranches(true)}>刷新分支</Button>
            </Flex>}>
                <AutoComplete className="full" allowClear virtual={false} defaultActiveFirstOption={false}
                    placeholder="选择或手动输入分支名" onFocus={() => loadBranches(false)}
                    showSearch={{filterOption: (input, option) => String(option?.value ?? '').toLowerCase().includes(input.toLowerCase())}}
                    options={list?.branches?.map(item => ({value: item.name, label: <Space size={6}>
                        <span>{item.name}</span>
                        {item.commit && <Typography.Text type="secondary">{item.commit.slice(0, 7)}</Typography.Text>}
                        {installed.has(item.name) && (installed.get(item.name) === snapshot.running
                            ? <Tag color="green">正在运行</Tag> : <Tag color="blue">已安装</Tag>)}
                    </Space>}))} />
            </Form.Item>
            {list?.error && <Alert type="warning" showIcon title="分支列表不可用，仍可手动输入分支名。" description={list.error} />}
            {blocked && <Alert type="info" showIcon title={blocked} />}
            {installError && <Alert type="error" showIcon title="未能开始安装" description={installError} />}
        </Form>
    </Modal>
}
