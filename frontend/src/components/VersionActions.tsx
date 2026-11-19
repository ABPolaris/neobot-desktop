import {useRef} from 'react'
import {App, Button, Checkbox, Dropdown} from 'antd'
import {RemoveVersion, Repair} from '../../wailsjs/go/main/App'
import type {launcher} from '../../wailsjs/go/models'
import {useLauncher} from '../app/LauncherProvider'
import {displayBranch} from '../app/versionLabel'
import {LauncherIcon} from './LauncherIcon'

const repairHint = '重新安装这个版本的 Python 环境与依赖，data/ 里的数据不受影响。'
const branchRepairHint = '分支版本由源码安装，请用「更新」重新安装。'

// VersionActions 是版本列表里的「更多操作」菜单：修复和卸载都不常用且不可逆，
// 摊成三个并排按钮会把「设为启动版本」这个主操作淹掉，收进菜单更清爽。
export function VersionActions({item, disabled}: {item: launcher.Version; disabled: boolean}) {
    const {action} = useLauncher()
    const {message, modal} = App.useApp()
    const dropData = useRef(false)
    const label = item.branch ? displayBranch(item.branch, item.repository) : item.name

    function repair() {
        if (item.branch) return
        modal.confirm({
            title: `修复 ${label}？`,
            content: repairHint,
            okText: '开始修复',
            cancelText: '取消',
            onOk: () => action(async () => {
                await Repair(item.name)
                void message.success('已开始修复，进度显示在「安装」卡片里')
            }),
        })
    }

    function remove() {
        dropData.current = false
        modal.confirm({
            title: `卸载 ${label}？`,
            content: <div style={{display: 'grid', gap: 10}}>
                <span>会删除这个版本的安装目录；共用的 data/ 不受影响，历史快照需要单独清理。</span>
                <Checkbox onChange={event => { dropData.current = event.target.checked }}>
                    同时删除它的独立数据目录（isolated/，删除后无法恢复）
                </Checkbox>
            </div>,
            okText: '卸载',
            cancelText: '取消',
            okButtonProps: {danger: true},
            onOk: () => action(async () => {
                await RemoveVersion(item.name, dropData.current)
                void message.success('已卸载 ' + label)
            }),
        })
    }

    return <Dropdown trigger={['click']} placement="bottomRight" disabled={disabled}
        menu={{
            items: [
                {
                    key: 'repair',
                    label: '修复安装',
                    disabled: Boolean(item.branch),
                    title: item.branch ? branchRepairHint : repairHint,
                },
                {type: 'divider'},
                {key: 'remove', label: '卸载版本', danger: true},
            ],
            onClick: ({key}) => {
                if (key === 'repair') repair()
                if (key === 'remove') remove()
            },
        }}>
        <Button aria-label={'更多操作：' + label} icon={<LauncherIcon name="more" size={16} />} />
    </Dropdown>
}
