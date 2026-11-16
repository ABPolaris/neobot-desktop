import type {launcher} from '../../wailsjs/go/models'

export function displayVersion(item: launcher.Version): string {
    return item.branch ? '分支 ' + item.branch : 'NeoBot ' + item.name
}

export function displayVersionName(name: string, installed: launcher.Version[]): string {
    const item = installed.find(version => version.name === name)
    return item ? displayVersion(item) : name
}
