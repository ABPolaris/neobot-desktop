import {createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode} from 'react'
import {App} from 'antd'
import {GetSnapshot} from '../../wailsjs/go/main/App'
import {EventsOn} from '../../wailsjs/runtime/runtime'
import type {launcher} from '../../wailsjs/go/models'

type Context = {
    state?: launcher.Snapshot
    error?: string
    refresh: () => Promise<void>
    action: (work: () => Promise<void>) => Promise<void>
}
const LauncherContext = createContext<Context | undefined>(undefined)

export function LauncherProvider({children}: {children: ReactNode}) {
    const [state, setState] = useState<launcher.Snapshot>()
    const [error, setError] = useState<string>()
    const request = useRef(0)
    const {notification} = App.useApp()
    const refresh = useCallback(async () => {
        const id = ++request.current
        try {
            const snapshot = await GetSnapshot()
            if (request.current === id) {
                setState(snapshot)
                setError(undefined)
            }
        } catch (failure) {
            if (request.current === id) setError(String(failure))
        }
    }, [])

    useEffect(() => {
        const off = EventsOn('launcher:changed', () => void refresh())
        void refresh()
        return () => {
            ++request.current
            off()
        }
    }, [refresh])

    async function action(work: () => Promise<void>) {
        try {
            await work()
        } catch (failure) {
            notification.error({title: '操作未完成', description: String(failure)})
        } finally {
            await refresh()
        }
    }

    return <LauncherContext.Provider value={{state, error, refresh, action}}>{children}</LauncherContext.Provider>
}

export function useLauncher() {
    const value = useContext(LauncherContext)
    if (!value) throw new Error('LauncherProvider is missing')
    return value
}
