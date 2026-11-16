import React from 'react'
import {createRoot} from 'react-dom/client'
import {App as AntApp, ConfigProvider} from 'antd'
import zhCN from 'antd/locale/zh_CN'
import {HashRouter} from 'react-router'
import 'antd/dist/reset.css'
import './styles/global.css'
import {AppShell} from './app/AppShell'
import {LauncherProvider} from './app/LauncherProvider'

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
    <React.StrictMode>
        <ConfigProvider locale={zhCN} theme={{token: {colorPrimary: '#1551c3', borderRadius: 10, fontSize: 14}}}>
            <AntApp>
                <HashRouter>
                    <LauncherProvider>
                        <AppShell />
                    </LauncherProvider>
                </HashRouter>
            </AntApp>
        </ConfigProvider>
    </React.StrictMode>
)
