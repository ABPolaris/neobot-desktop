import {useEffect, useState} from 'react'
import {Link, useLocation} from 'react-router'
import {Quit, WindowIsMaximised, WindowMinimise, WindowToggleMaximise} from '../../wailsjs/runtime/runtime'
import {LauncherIcon} from '../components/LauncherIcon'

const tabs = [
    {path: '/', label: '启动', icon: 'play'},
    {path: '/versions', label: '版本', icon: 'layers'},
    {path: '/settings', label: '设置', icon: 'settings'},
] as const

export function TitleBar() {
    const {pathname} = useLocation()
    const current = pathname === '/setup' ? '/' : pathname
    const [maximized, setMaximized] = useState(false)

    useEffect(() => {
        let active = true
        const syncState = async () => {
            try {
                const value = await WindowIsMaximised()
                if (active) setMaximized(value)
            } catch { /* Browser preview may not have window controls. */ }
        }
        void syncState()
        window.addEventListener('resize', syncState)
        return () => {
            active = false
            window.removeEventListener('resize', syncState)
        }
    }, [])

    const sizeLabel = maximized ? '还原窗口' : '最大化窗口'
    return <header className="title-bar" onDoubleClick={event => {
        if (event.target instanceof Element && !event.target.closest('button, a, nav')) WindowToggleMaximise()
    }}>
        <Link to="/" className="brand" aria-label="NeoBot Desktop 启动页">
            <LauncherIcon name="bot" size={30} /><span>NeoBot Desktop</span>
        </Link>
        <nav className="title-nav" aria-label="主导航">
            {tabs.map(({path, label, icon}) => <Link key={path} to={path}
                aria-current={current === path ? 'page' : undefined}>
                <LauncherIcon name={icon} /><span>{label}</span>
            </Link>)}
        </nav>
        <div className="window-controls" role="group" aria-label="窗口控制">
            <button type="button" aria-label="最小化窗口" title="最小化" onClick={WindowMinimise}>
                <LauncherIcon name="minimize" size={16} />
            </button>
            <button type="button" aria-label={sizeLabel} title={sizeLabel} onClick={WindowToggleMaximise}>
                <LauncherIcon name={maximized ? 'restore' : 'maximize'} size={16} />
            </button>
            <button type="button" className="window-close" aria-label="关闭窗口" title="关闭" onClick={Quit}>
                <LauncherIcon name="close" size={16} />
            </button>
        </div>
    </header>
}
