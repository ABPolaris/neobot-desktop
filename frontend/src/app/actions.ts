import {App} from 'antd'
import {Start, Stop, SwitchInfo} from '../../wailsjs/go/main/App'
import {useLauncher} from './LauncherProvider'

export function useRuntimeActions() {
    const {modal} = App.useApp()
    const {action} = useLauncher()
    return {
        start: () => action(async () => {
            const change = await SwitchInfo()
            if (!change.restore) return Start(false)
            modal.confirm({
                title: `恢复 ${change.to} 的数据快照？`,
                content: `自切换到 ${change.from} 后产生的聊天、配置或插件数据不会在旧版中显示。当前数据会另存备份。`,
                okText: '恢复快照并启动', cancelText: '取消',
                onOk: () => action(() => Start(true)),
            })
        }),
        stop: () => action(() => Stop(false)),
        forceStop: () => modal.confirm({
            title: '强制停止 NeoBot？',
            content: '正在处理的消息或尚未保存的数据可能丢失。',
            okText: '强制停止', cancelText: '取消', okButtonProps: {danger: true},
            onOk: () => action(() => Stop(true)),
        }),
    }
}
