import {Alert, Button, Layout, Spin} from 'antd'
import {useLayoutEffect, useRef} from 'react'
import {Navigate, Route, Routes, useLocation} from 'react-router'
import {useLauncher} from './LauncherProvider'
import {HomePage} from '../pages/HomePage'
import {SetupPage} from '../pages/SetupPage'
import {VersionsPage} from '../pages/VersionsPage'
import {DiagnosticsPage} from '../pages/DiagnosticsPage'
import {SettingsPage} from '../pages/SettingsPage'
import {TitleBar} from './TitleBar'

export function AppShell() {
    const {pathname} = useLocation()
    const {state, error, refresh} = useLauncher()
    const workspace = useRef<HTMLElement>(null)

    useLayoutEffect(() => {
        if (workspace.current) workspace.current.scrollTop = 0
    }, [pathname])

    return <Layout className="shell">
        <TitleBar />
        <Layout.Content ref={workspace} tabIndex={0} aria-label="页面内容"
            className={pathname === '/' && state && !error ? 'workspace workspace-home' : 'workspace'}>
            {error ? <Alert type="error" title="启动器未就绪" description={error} action={<Button onClick={() => void refresh()}>重试</Button>} />
                : !state ? <Spin aria-label="正在读取启动器状态" />
                : <Routes>
                    <Route path="/" element={<HomePage />} />
                    <Route path="/setup" element={<SetupPage />} />
                    <Route path="/versions" element={<VersionsPage />} />
                    <Route path="/diagnostics" element={<DiagnosticsPage />} />
                    <Route path="/settings" element={<SettingsPage />} />
                    <Route path="*" element={<Navigate to="/" replace />} />
                </Routes>}
        </Layout.Content>
    </Layout>
}
