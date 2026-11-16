const shapes = {
    bot: <><rect x="4" y="8" width="16" height="12" rx="4" /><path d="M12 8V4M2 13v3m20-3v3M8.5 14v1m7-1v1" /><circle cx="12" cy="3" r="1" /></>,
    play: <path d="M7 4.8a.8.8 0 0 1 1.2-.7l12 7.2a.8.8 0 0 1 0 1.4l-12 7.2a.8.8 0 0 1-1.2-.7z" fill="currentColor" stroke="none" />,
    layers: <><path d="m12 3 9 5-9 5-9-5z" /><path d="m3 12 9 5 9-5M3 16l9 5 9-5" /></>,
    download: <path d="M12 3v12m-4-4 4 4 4-4M4 15v5h16v-5" />,
    settings: <><path d="M4 6h8M16 6h4M4 12h3M11 12h9M4 18h8M16 18h4" /><circle cx="14" cy="6" r="2" /><circle cx="9" cy="12" r="2" /><circle cx="14" cy="18" r="2" /></>,
    qq: <path d="M12 3c-3.1 0-5 2.4-5 5.6v1.7L4.2 15c-1 1.7.6 2.9 2.1 1.9l.7-.5-.5 2.1C6.1 20 8 21 12 21s5.9-1 5.5-2.5l-.5-2.1.7.5c1.5 1 3.1-.2 2.1-1.9L17 10.3V8.6C17 5.4 15.1 3 12 3Z" />,
    model: <path d="m12 3 8 4.5v9L12 21l-8-4.5v-9zm0 9 8-4.5M12 12 4 7.5m8 4.5v9" />,
    copy: <><rect x="8" y="3" width="11" height="14" rx="2" /><path d="M15 21H7a2 2 0 0 1-2-2V7" /></>,
    minimize: <path d="M5 12h14" />,
    maximize: <rect x="5" y="5" width="14" height="14" rx="1" />,
    restore: <><rect x="4" y="8" width="12" height="12" rx="1" /><path d="M8 8V5a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v10a1 1 0 0 1-1 1h-3" /></>,
    close: <path d="m6 6 12 12M18 6 6 18" />,
}

export function LauncherIcon({name, size = 20}: {name: keyof typeof shapes; size?: number}) {
    return <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
        strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
        {shapes[name]}
    </svg>
}
