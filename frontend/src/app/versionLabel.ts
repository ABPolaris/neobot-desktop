import type {launcher} from '../../wailsjs/go/models'

export const MAIN_REPOSITORY = 'SuperQuail/NeoBot'

export function sameRepository(left?: string, right?: string): boolean {
    return (left || MAIN_REPOSITORY).toLowerCase() === (right || MAIN_REPOSITORY).toLowerCase()
}

export function displayBranch(branch: string | undefined, repository?: string): string {
    const label = branch ? '分支 ' + branch : '默认分支'
    return sameRepository(repository, MAIN_REPOSITORY) ? label : `${repository} · ${label}`
}

export function displayVersion(item: launcher.Version): string {
    return item.branch ? displayBranch(item.branch, item.repository) : 'NeoBot ' + item.name
}

export function displayVersionName(name: string, installed: launcher.Version[]): string {
    const item = installed.find(version => version.name === name)
    return item ? displayVersion(item) : name
}
