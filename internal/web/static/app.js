function app() {
    return {
        // State
        isLoading: false,
        theme: localStorage.getItem('theme') || 'system',
        locale: document.documentElement.lang === 'zh-CN' ? 'zh-CN' : 'en',
        supportedLocales: ['en', 'zh-CN'],
        activeTab: 'providers',
        servicePoll: null,
        selectedClient: 'claude',
        integrations: [],
        integrationBusyProduct: '',
        serviceBusyAction: '',
        messages: {
            en: {
                meta: {
                    title: 'Clipal Management'
                },
                header: {
                    subtitle: 'Local ingress for Claude, OpenAI, and Gemini traffic',
                    uptime: 'Uptime: {uptime}'
                },
                nav: {
                    sectionsLabel: 'Sections',
                    providers: 'Providers',
                    integrations: 'CLI Takeover',
                    settings: 'Global Settings',
                    services: 'Services',
                    status: 'System Status'
                },
                common: {
                    none: 'None',
                    save: 'Save',
                    reset: 'Reset',
                    export: 'Export',
                    cancel: 'Cancel',
                    show: 'Show',
                    hide: 'Hide',
                    refresh: 'Refresh',
                    working: 'Working...',
                    enabled: 'Enabled',
                    disabled: 'Disabled',
                    active: 'Active'
                },
                locale: {
                    label: 'Language',
                    english: 'English',
                    chinese: 'Chinese'
                },
                theme: {
                    current: 'Current theme: {theme}',
                    system: 'System',
                    light: 'Light',
                    dark: 'Dark'
                },
                providers: {
                    addTo: 'Add Provider to {client}',
                    modeLabel: 'Mode',
                    modeAuto: 'Auto',
                    modeManual: 'Manual',
                    pinned: 'Pinned:',
                    switchToManual: 'Switch to Manual',
                    backToAuto: 'Back to Auto',
                    enableProviderFirst: 'Enable a provider first',
                    pinHint: 'Pin a provider by clicking 📌 on a provider card.',
                    empty: 'No providers configured for {client}',
                    pinBadge: 'Pinned',
                    baseUrl: 'Base URL',
                    apiKeys: 'API Keys',
                    model: 'Model Override',
                    reasoningEffort: 'Reasoning Effort',
                    thinkingBudgetTokens: 'Thinking Budget Tokens',
                    configuredCount: '{count} configured',
                    pinTitle: 'Pin (Manual)',
                    dragToReorder: 'Drag to reorder providers',
                    edit: 'Edit',
                    delete: 'Delete',
                    disable: 'Disable',
                    enable: 'Enable',
                    pinnedProvider: 'Pinned provider',
                    pinnedProviderCannotDisable: 'Pinned provider cannot be disabled in manual mode',
                    enablePinnedProvider: 'Enable pinned provider',
                    modeHelpManual: 'Manual (Pinned)\nAlways use the pinned provider.\nNo failover; failures return errors.',
                    modeHelpAuto: 'Auto (Failover)\nTries enabled providers by priority.\nSwitches on failures.',
                    enableBeforeManual: 'Enable a provider before switching to manual mode',
                    enableBeforePinning: 'Enable the provider before pinning it',
                    switchedToAutoTitle: '{client} switched to Auto',
                    switchedToAutoMessage: 'Failover now follows enabled providers by priority.',
                    switchedToManualTitle: '{client} switched to Manual',
                    switchedToManualMessagePinned: 'Pinned to {provider}. Requests stay on this provider until you switch back to Auto.',
                    switchedToManualMessage: 'Requests now stay on the pinned provider until you switch back to Auto.',
                    pinnedTitle: 'Pinned {client} to {provider}',
                    pinnedMessage: 'New requests will stay on this provider until you switch back to Auto.',
                    enabledTitle: 'Enabled {provider} for {client}',
                    enabledMessage: 'It is available again for failover selection.',
                    disabledTitle: 'Disabled {provider} for {client}',
                    disabledMessage: 'It has been removed from failover selection.',
                    updatedTitle: 'Updated provider {provider}',
                    updatedMessage: 'Changes are now active for {client}.',
                    addedTitle: 'Added provider {provider}',
                    addedMessage: 'The provider is now available for {client}.',
                    deleteConfirm: 'Are you sure you want to delete provider "{name}"?',
                    deletedTitle: 'Deleted provider {name}',
                    deletedMessage: 'It has been removed from {client}\'s provider list.',
                    clientTypeLabel: 'Client Type'
                },
                modal: {
                    provider: {
                        addTitle: 'Add Provider',
                        editTitle: 'Edit Provider',
                        close: 'Close modal',
                        name: 'Name *',
                        nameHint: 'Letters, numbers, dot (.), underscore (_), and hyphen (-).',
                        baseUrl: 'Base URL *',
                        model: 'Model',
                        modelHint: 'Keep request model.',
                        reasoningEffort: 'Reasoning Effort',
                        reasoningEffortHint: 'Used as OpenAI Responses reasoning.effort.',
                        thinkingBudgetTokens: 'Thinking Budget',
                        thinkingBudgetTokensHint: 'Use 0 to clear Claude thinking.budget_tokens.',
                        apiKeys: 'API Keys',
                        apiKeysRequired: 'API Keys *',
                        onePerLine: 'One API key per line',
                        savedAs: 'Saved as',
                        savedAsSingle: '1 line -> api_key',
                        savedAsMultiple: '2+ lines -> api_keys',
                        keepExistingKey: 'Leave empty to keep the current configured key.',
                        keepExistingKeys: 'Leave empty to keep the current {count} configured keys.',
                        overridesTitle: 'Overrides',
                        overridesOptional: 'Optional',
                        overridesSummaryEmpty: 'Model override, provider-specific request tuning',
                        overridesPanelHint: 'Optional request-level tuning for this provider.',
                        priority: 'Priority',
                        priorityHint: 'Smaller numbers are tried first.',
                        saveProvider: 'Save Provider'
                    }
                },
                settings: {
                    title: 'Global Settings',
                    subtitle: 'Runtime defaults, recovery policy, routing strategy, and local observability.',
                    runtimeTitle: 'Runtime',
                    runtimeCopy: 'Basic listener and request buffering defaults.',
                    listenAddress: 'Listen Address',
                    port: 'Port',
                    logLevel: 'Log Level',
                    maxBodySize: 'Max Body Size',
                    maxBodySizeHint: 'Bytes buffered for retryable requests.',
                    reliabilityTitle: 'Reliability',
                    reliabilityCopy: 'Timeouts, temporary deactivation, and circuit breaker behavior.',
                    reactivateAfter: 'Reactivate After',
                    reactivateAfterHint: 'How long a failed provider stays deactivated.',
                    upstreamIdleTimeout: 'Upstream Idle Timeout',
                    upstreamIdleTimeoutHint: 'Set to 0 to disable stalled-stream protection.',
                    responseHeaderTimeout: 'Response Header Timeout',
                    responseHeaderTimeoutHint: 'Set to 0 to wait indefinitely for headers.',
                    failureThreshold: 'Failure Threshold',
                    failureThresholdHint: '0 disables the circuit breaker.',
                    successThreshold: 'Success Threshold',
                    openTimeout: 'Open Timeout',
                    halfOpenMaxInflight: 'Half-Open Max In-Flight',
                    logsAlertsTitle: 'Logs & Alerts',
                    logsAlertsCopy: 'Local logging, retention, and desktop notification behavior.',
                    logDirectory: 'Log Directory',
                    logDirectoryHint: 'Leave empty to use the default config-dir logs folder.',
                    retentionDays: 'Retention (Days)',
                    retentionDaysHint: '0 keeps logs forever.',
                    notificationLevel: 'Notification Level',
                    logToStdout: 'Log to Stdout',
                    desktopNotifications: 'Desktop Notifications',
                    notifyOnProviderSwitch: 'Notify on Provider Switch',
                    routingTitle: 'Routing Strategy',
                    routingCopy: 'Expose only the routing controls that change user-visible behavior.',
                    stickySessionTtl: 'Sticky Session TTL',
                    stickySessionTtlHint: 'Idle lifetime for explicit session bindings.',
                    shortRetryAfterMax: 'Short Retry-After Max',
                    shortRetryAfterMaxHint: 'Upper bound for honoring short upstream retry-after hints.',
                    maxInlineWait: 'Max Inline Wait',
                    maxInlineWaitHint: 'How long Clipal may wait before overflowing to another provider.',
                    enableStickySessions: 'Enable Sticky Sessions',
                    enableBusyBackpressure: 'Enable Busy Backpressure',
                    footerHint: 'Saving updates `config.yaml`. Some runtime changes may require restart to take full effect.',
                    saveSettings: 'Save Settings',
                    saveSuccess: 'Configuration saved. Some changes may require restart.',
                    exportSuccess: 'Configuration exported successfully',
                    exportFailure: 'Failed to export configuration'
                },
                integrations: {
                    title: 'CLI Takeover',
                    subtitle: 'Let Clipal take over supported CLI clients by modifying their user-level config files.',
                    refresh: 'Refresh',
                    userConfigHint: 'This only edits your user-level config file. Project-local or managed settings may still override the effective behavior.',
                    restartHint: 'Restart the client or open a new session after applying changes.',
                    empty: 'No supported integrations detected.',
                    apply: 'Use Clipal',
                    rollback: 'Restore',
                    stateConfigured: 'Configured',
                    stateNeedsAttention: 'Needs attention',
                    stateNotConfigured: 'Not configured',
                    alreadyUsing: 'Already using Clipal',
                    noBackupYet: 'No backup yet. Apply once before restore becomes available.',
                    restoreOnlyActive: 'Restore is only available while Clipal is active.',
                    dismissResult: 'Dismiss result',
                    detailsPreview: 'View Configuration Details & Preview',
                    backupAvailable: 'Backup Available',
                    noBackupMeta: 'No backup yet',
                    currentFile: 'Current file',
                    fileDoesNotExistYet: 'File does not exist yet.',
                    latestBackup: 'Latest backup',
                    afterApply: 'After apply',
                    backupEmpty: 'Backup is empty.',
                    originalFileMissing: 'Original file did not exist before Clipal takeover.',
                    noPlannedChanges: 'No planned changes.',
                    resultUpdated: 'Updated',
                    resultError: 'Error',
                    resultNotice: 'Notice',
                    resultUsingClipalTitle: 'Now using Clipal',
                    resultRestoredTitle: 'Restored from backup',
                    resultRestartMessage: 'Restart the client or open a new session to apply changes.',
                    resultApplyErrorTitle: 'Couldn’t update this client',
                    resultRestoreErrorTitle: 'Couldn’t restore this client',
                    noteClaude: 'Clipal only updates ANTHROPIC_BASE_URL. ANTHROPIC_AUTH_TOKEN is left untouched.',
                    noteCodex: 'Clipal updates model_provider to clipal and writes [model_providers.clipal] with the local URL and wire_api = "responses".',
                    noteOpencode: 'Clipal adds or updates provider.clipal, points it at the local Clipal OpenAI-compatible URL, and switches the active model to clipal/<current-model>.',
                    noteGemini: 'Clipal only updates GEMINI_API_BASE in ~/.gemini/.env. Other Gemini environment overrides may still take precedence.',
                    noteContinue: 'Clipal adds or updates a user-level Continue model entry that points at the local Clipal OpenAI-compatible URL. You may still need to select that model inside Continue.',
                    noteAider: 'Clipal updates the home-level .aider.conf.yml openai-api-base and a minimal OpenAI-compatible model value. Repo-local config, .env, or CLI flags can still override it.',
                    noteGoose: 'Clipal creates or updates a managed Goose custom provider file. You may still need to select the Clipal provider or model inside Goose.',
                    noteDefault: 'Clipal only edits the user-level config file shown on this card.'
                },
                services: {
                    title: 'Clipal Service',
                    subtitle: 'Manage the OS background service (same as clipal service *)',
                    supported: 'Supported',
                    unsupported: 'Unsupported',
                    installed: 'Installed',
                    notInstalled: 'Not installed',
                    running: 'Running',
                    stopped: 'Stopped',
                    needsAttention: 'Needs attention',
                    unsupportedBuild: 'Service manager is not supported on this OS build.',
                    autostartNotInstalled: 'Autostart service is not installed yet.',
                    installWindows: 'Install requires an elevated console on Windows. Use the command below.',
                    installOther: 'Click Install to register Clipal as a background service for this user.',
                    copyInstallCommand: 'Copy Install Command',
                    installService: 'Install Service',
                    start: 'Start',
                    restart: 'Restart',
                    stop: 'Stop',
                    uninstall: 'Uninstall',
                    check: 'Check',
                    force: 'Force',
                    reinstall: 'Reinstall',
                    forceHint: 'Reinstall or refresh the existing service definition when needed',
                    restartDisconnectHint: 'Restart or stop may temporarily disconnect this page. Status auto-refreshes every 3s while this tab is open.',
                    confirmUninstall: 'Uninstall the system service?',
                    confirmStop: 'Stop the system service?',
                    confirmRestart: 'Restart the system service?',
                    requestedAction: 'Requested service {action}. Refreshing...',
                    installCommandCopied: 'Install command copied',
                    copyCommandFailed: 'Failed to copy command',
                    notSupportedReason: 'Service manager is not supported on this OS',
                    alreadyInstalledReason: 'Already installed. Enable Force to reinstall or refresh the service definition.',
                    installFirst: 'Install the service first',
                    alreadyRunning: 'Service is already running',
                    notRunning: 'Service is not running',
                    serviceNotInstalled: 'Service is not installed'
                },
                statusPage: {
                    systemInfo: 'System Info',
                    uptime: 'Uptime',
                    configDir: 'Config Dir',
                    circuitBreaker: 'Circuit Breaker',
                    disabled: 'Disabled',
                    circuitBreakerSummary: '{failure} fail / {success} succ ({timeout})',
                    pinned: 'Pinned',
                    lastSwitch: 'Last switch:',
                    lastRequest: 'Last request:',
                    recentActivity: 'Recent activity',
                    providersCount: 'Providers: {count}',
                    enabledCount: 'Enabled: {count}',
                    groupCurrent: 'Current',
                    groupActive: 'Active',
                    groupDisabled: 'Disabled',
                    groupCoolingDown: 'Cooling down',
                    groupUnavailable: 'Unavailable',
                    groupRecoveryProbe: 'Recovery probe',
                    keysAvailable: 'Keys available: {available}/{total}'
                },
                toast: {
                    success: 'Success',
                    requestFailed: 'Request failed',
                    notice: 'Notice',
                    error: 'Error',
                    info: 'Info',
                    dismissNotification: 'Dismiss notification'
                },
                level: {
                    debug: 'Debug',
                    info: 'Info',
                    warn: 'Warn',
                    error: 'Error'
                }
            },
            'zh-CN': {
                meta: {
                    title: 'Clipal 管理界面'
                },
                header: {
                    subtitle: '面向 Claude、OpenAI 和 Gemini 流量的本地入口',
                    uptime: '运行时长：{uptime}'
                },
                nav: {
                    sectionsLabel: '功能分区',
                    providers: 'Providers',
                    integrations: 'CLI 接管',
                    settings: '全局设置',
                    services: '服务',
                    status: '系统状态'
                },
                common: {
                    none: '无',
                    save: '保存',
                    reset: '重置',
                    export: '导出',
                    cancel: '取消',
                    show: '展开',
                    hide: '收起',
                    refresh: '刷新',
                    working: '处理中...',
                    enabled: '已启用',
                    disabled: '已禁用',
                    active: '可用'
                },
                locale: {
                    label: '语言',
                    english: '英文',
                    chinese: '中文'
                },
                theme: {
                    current: '当前主题：{theme}',
                    system: '跟随系统',
                    light: '浅色',
                    dark: '深色'
                },
                providers: {
                    addTo: '为 {client} 添加 Provider',
                    modeLabel: '模式',
                    modeAuto: '自动',
                    modeManual: '手动',
                    pinned: '固定：',
                    switchToManual: '切到手动',
                    backToAuto: '返回自动',
                    enableProviderFirst: '请先启用一个 Provider',
                    pinHint: '点击 Provider 卡片上的 📌 可固定 Provider。',
                    empty: '{client} 还没有配置任何 Provider',
                    pinBadge: '已固定',
                    baseUrl: 'Base URL',
                    apiKeys: 'API Keys',
                    model: '模型覆盖',
                    reasoningEffort: '思考强度',
                    thinkingBudgetTokens: '思考预算 Tokens',
                    configuredCount: '已配置 {count} 个',
                    pinTitle: '固定到手动模式',
                    edit: '编辑',
                    delete: '删除',
                    disable: '禁用',
                    enable: '启用',
                    pinnedProvider: '已固定 Provider',
                    pinnedProviderCannotDisable: '手动模式下不能禁用已固定的 Provider',
                    enablePinnedProvider: '先启用已固定的 Provider',
                    modeHelpManual: '手动（固定）\n始终使用固定的 Provider。\n不进行故障切换，失败会直接报错。',
                    modeHelpAuto: '自动（故障切换）\n按优先级尝试已启用的 Provider。\n失败时自动切换。',
                    enableBeforeManual: '切到手动模式前请先启用一个 Provider',
                    enableBeforePinning: '固定前请先启用该 Provider',
                    switchedToAutoTitle: '{client} 已切到自动模式',
                    switchedToAutoMessage: '故障切换将按已启用 Provider 的优先级进行。',
                    switchedToManualTitle: '{client} 已切到手动模式',
                    switchedToManualMessagePinned: '已固定到 {provider}。在切回自动模式前，请求都会停留在这个 Provider 上。',
                    switchedToManualMessage: '在切回自动模式前，请求都会停留在固定的 Provider 上。',
                    pinnedTitle: '已将 {client} 固定到 {provider}',
                    pinnedMessage: '后续请求会停留在这个 Provider 上，直到你切回自动模式。',
                    enabledTitle: '已为 {client} 启用 {provider}',
                    enabledMessage: '它现在会重新参与故障切换选择。',
                    disabledTitle: '已为 {client} 禁用 {provider}',
                    disabledMessage: '它已从故障切换选择中移除。',
                    updatedTitle: '已更新 Provider {provider}',
                    updatedMessage: '{client} 的改动已经生效。',
                    addedTitle: '已添加 Provider {provider}',
                    addedMessage: '这个 Provider 现在可供 {client} 使用。',
                    deleteConfirm: '确认删除 Provider “{name}” 吗？',
                    deletedTitle: '已删除 Provider {name}',
                    deletedMessage: '它已从 {client} 的 Provider 列表中移除。',
                    clientTypeLabel: '客户端类型',
                    dragToReorder: '拖拽调整优先级'
                },
                modal: {
                    provider: {
                        addTitle: '添加 Provider',
                        editTitle: '编辑 Provider',
                        close: '关闭弹窗',
                        name: '名称 *',
                        nameHint: '允许字母、数字、点号 (.)、下划线 (_) 和连字符 (-)。',
                        baseUrl: 'Base URL *',
                        model: '模型',
                        modelHint: '留空则不覆盖。',
                        reasoningEffort: '思考强度',
                        reasoningEffortHint: '写入 OpenAI Responses 的 reasoning.effort。',
                        thinkingBudgetTokens: '思考预算',
                        thinkingBudgetTokensHint: '填 0 清空 Claude 的 thinking.budget_tokens。',
                        apiKeys: 'API Keys',
                        apiKeysRequired: 'API Keys *',
                        onePerLine: '每行一个 API Key',
                        savedAs: '保存方式',
                        savedAsSingle: '1 行 -> api_key',
                        savedAsMultiple: '2 行及以上 -> api_keys',
                        keepExistingKeys: '留空则保留当前已配置的 {count} 个 key。',
                        overridesTitle: 'Overrides',
                        overridesOptional: '可选',
                        overridesSummaryEmpty: '模型覆盖与 Provider 请求调优',
                        overridesPanelHint: '仅在这个 Provider 需要请求级调优时再填写。',
                        priority: '优先级',
                        priorityHint: '数字越小越先尝试。',
                        saveProvider: '保存 Provider'
                    }
                },
                settings: {
                    title: '全局设置',
                    subtitle: '运行时默认项、恢复策略、路由策略以及本地可观测性。',
                    runtimeTitle: '运行时',
                    runtimeCopy: '基础监听配置与请求缓冲默认项。',
                    listenAddress: '监听地址',
                    port: '端口',
                    logLevel: '日志级别',
                    maxBodySize: '最大请求体大小',
                    maxBodySizeHint: '用于可重试请求的缓冲字节数。',
                    reliabilityTitle: '可靠性',
                    reliabilityCopy: '超时、临时停用和熔断器行为。',
                    reactivateAfter: '恢复激活时间',
                    reactivateAfterHint: '失败的 Provider 会停用多久后再恢复。',
                    upstreamIdleTimeout: '上游空闲超时',
                    upstreamIdleTimeoutHint: '设为 0 可关闭流式响应停滞保护。',
                    responseHeaderTimeout: '响应头超时',
                    responseHeaderTimeoutHint: '设为 0 表示无限等待响应头。',
                    failureThreshold: '失败阈值',
                    failureThresholdHint: '设为 0 可关闭熔断器。',
                    successThreshold: '成功阈值',
                    openTimeout: '打开超时',
                    halfOpenMaxInflight: '半开最大并发',
                    logsAlertsTitle: '日志与提醒',
                    logsAlertsCopy: '本地日志、保留策略和桌面通知行为。',
                    logDirectory: '日志目录',
                    logDirectoryHint: '留空则使用默认的配置目录 logs 文件夹。',
                    retentionDays: '保留天数',
                    retentionDaysHint: '设为 0 表示永久保留日志。',
                    notificationLevel: '通知级别',
                    logToStdout: '输出到 Stdout',
                    desktopNotifications: '桌面通知',
                    notifyOnProviderSwitch: 'Provider 切换时通知',
                    routingTitle: '路由策略',
                    routingCopy: '只暴露会影响用户可见行为的路由控制项。',
                    stickySessionTtl: '粘性会话 TTL',
                    stickySessionTtlHint: '显式会话绑定的空闲生存时间。',
                    shortRetryAfterMax: '短 Retry-After 上限',
                    shortRetryAfterMaxHint: '对上游较短 retry-after 提示的最大遵从值。',
                    maxInlineWait: '最大内联等待',
                    maxInlineWaitHint: 'Clipal 在溢出到其他 Provider 前可等待的最长时间。',
                    enableStickySessions: '启用粘性会话',
                    enableBusyBackpressure: '启用 Busy Backpressure',
                    footerHint: '保存会更新 `config.yaml`。部分运行时改动需要重启后才会完全生效。',
                    saveSettings: '保存设置',
                    saveSuccess: '配置已保存。部分改动可能需要重启。',
                    exportSuccess: '配置导出成功',
                    exportFailure: '配置导出失败'
                },
                integrations: {
                    title: 'CLI 接管',
                    subtitle: '让 Clipal 通过修改用户级配置文件接管受支持的 CLI 客户端。',
                    refresh: '刷新',
                    userConfigHint: '这里只会修改你的用户级配置文件。项目级或受管控配置仍可能覆盖最终生效结果。',
                    restartHint: '应用修改后，请重启客户端或重新打开一个会话。',
                    empty: '未检测到受支持的集成。',
                    apply: '使用 Clipal',
                    rollback: '恢复',
                    stateConfigured: '已配置',
                    stateNeedsAttention: '需要关注',
                    stateNotConfigured: '未配置',
                    alreadyUsing: '已经在使用 Clipal',
                    noBackupYet: '目前还没有备份。至少先使用一次 Clipal 后才能恢复。',
                    restoreOnlyActive: '只有在当前由 Clipal 接管时才能恢复。',
                    dismissResult: '关闭结果',
                    detailsPreview: '查看配置详情与预览',
                    backupAvailable: '已有备份',
                    noBackupMeta: '暂无备份',
                    currentFile: '当前文件',
                    fileDoesNotExistYet: '文件尚不存在。',
                    latestBackup: '最新备份',
                    afterApply: '启用后结果',
                    backupEmpty: '备份内容为空。',
                    originalFileMissing: 'Clipal 接管前原始文件不存在。',
                    noPlannedChanges: '没有计划中的变更。',
                    resultUpdated: '已更新',
                    resultError: '错误',
                    resultNotice: '提示',
                    resultUsingClipalTitle: '已开始使用 Clipal',
                    resultRestoredTitle: '已从备份恢复',
                    resultRestartMessage: '请重启客户端或重新打开一个会话以应用改动。',
                    resultApplyErrorTitle: '更新此客户端失败',
                    resultRestoreErrorTitle: '恢复此客户端失败',
                    noteClaude: 'Clipal 只会更新 ANTHROPIC_BASE_URL，不会改动 ANTHROPIC_AUTH_TOKEN。',
                    noteCodex: 'Clipal 会把 model_provider 更新为 clipal，并写入 [model_providers.clipal]，使用本地 URL 和 wire_api = "responses"。',
                    noteOpencode: 'Clipal 会新增或更新 provider.clipal，指向本地 Clipal OpenAI 兼容地址，并把当前 model 切到 clipal/<当前模型>。',
                    noteGemini: 'Clipal 只会更新 ~/.gemini/.env 中的 GEMINI_API_BASE。其他 Gemini 环境覆盖项仍可能优先生效。',
                    noteContinue: 'Clipal 会新增或更新用户级 Continue 模型项，指向本地 Clipal OpenAI 兼容地址。你可能仍需在 Continue 中手动选择该模型。',
                    noteAider: 'Clipal 会更新 home 级 .aider.conf.yml 中的 openai-api-base 和一个最小 OpenAI 兼容 model。仓库级配置、.env 或 CLI 参数仍可能覆盖它。',
                    noteGoose: 'Clipal 会创建或更新受管控的 Goose 自定义 provider 文件。你可能仍需在 Goose 中选择 Clipal provider 或 model。',
                    noteDefault: 'Clipal 只会修改此卡片展示的用户级配置文件。'
                },
                services: {
                    title: 'Clipal 服务',
                    subtitle: '管理操作系统后台服务（等同于 clipal service *）',
                    supported: '支持',
                    unsupported: '不支持',
                    installed: '已安装',
                    notInstalled: '未安装',
                    running: '运行中',
                    stopped: '已停止',
                    needsAttention: '需要关注',
                    unsupportedBuild: '当前操作系统构建不支持服务管理器。',
                    autostartNotInstalled: '自动启动服务尚未安装。',
                    installWindows: 'Windows 上安装需要提升权限的控制台。请使用下方命令。',
                    installOther: '点击“安装服务”即可为当前用户注册 Clipal 后台服务。',
                    copyInstallCommand: '复制安装命令',
                    installService: '安装服务',
                    start: '启动',
                    restart: '重启',
                    stop: '停止',
                    uninstall: '卸载',
                    check: '检查',
                    force: '强制',
                    reinstall: '重新安装',
                    forceHint: '在需要时重新安装或刷新现有服务定义',
                    restartDisconnectHint: '重启或停止服务时此页面可能会暂时断开。当前标签页打开时状态每 3 秒自动刷新一次。',
                    confirmUninstall: '确认卸载系统服务吗？',
                    confirmStop: '确认停止系统服务吗？',
                    confirmRestart: '确认重启系统服务吗？',
                    requestedAction: '已请求执行服务操作：{action}。正在刷新……',
                    installCommandCopied: '安装命令已复制',
                    copyCommandFailed: '复制命令失败',
                    notSupportedReason: '当前操作系统不支持服务管理器',
                    alreadyInstalledReason: '服务已安装。启用“强制”即可重新安装或刷新服务定义。',
                    installFirst: '请先安装服务',
                    alreadyRunning: '服务已在运行中',
                    notRunning: '服务当前未运行',
                    serviceNotInstalled: '服务尚未安装'
                },
                statusPage: {
                    systemInfo: '系统信息',
                    uptime: '运行时长',
                    configDir: '配置目录',
                    circuitBreaker: '熔断器',
                    disabled: '已禁用',
                    circuitBreakerSummary: '{failure} 次失败 / {success} 次成功（{timeout}）',
                    pinned: '已固定',
                    lastSwitch: '最近切换：',
                    lastRequest: '最近请求：',
                    recentActivity: '最近活动',
                    providersCount: '提供方：{count}',
                    enabledCount: '已启用：{count}',
                    groupCurrent: '当前',
                    groupActive: '可用',
                    groupDisabled: '已禁用',
                    groupCoolingDown: '冷却中',
                    groupUnavailable: '不可用',
                    groupRecoveryProbe: '恢复探测',
                    keysAvailable: '可用密钥：{available}/{total}'
                },
                toast: {
                    success: '成功',
                    requestFailed: '请求失败',
                    notice: '提示',
                    error: '错误',
                    info: '信息',
                    dismissNotification: '关闭通知'
                },
                level: {
                    debug: '调试',
                    info: '信息',
                    warn: '警告',
                    error: '错误'
                }
            }
        },
        clientOptions: [
            { value: 'claude', label: 'Claude' },
            { value: 'openai', label: 'OpenAI' },
            { value: 'gemini', label: 'Gemini' }
        ],
        providers: [],
        clientConfig: {
            mode: 'auto',
            pinned_provider: '',
            override_support: null
        },
        // Keep a stable object shape so Alpine x-model bindings don't explode
        // during the initial render (before loadGlobalConfig completes).
        globalConfig: {
            listen_addr: '',
            port: 0,
            log_level: 'info',
            reactivate_after: '',
            upstream_idle_timeout: '',
            response_header_timeout: '',
            max_request_body_bytes: 0,
            log_dir: '',
            log_retention_days: 7,
            log_stdout: true,
            notifications: {
                enabled: false,
                min_level: 'error',
                provider_switch: true
            },
            circuit_breaker: {
                failure_threshold: 4,
                success_threshold: 2,
                open_timeout: '60s',
                half_open_max_inflight: 1
            },
            routing: {
                sticky_sessions: {
                    enabled: true,
                    explicit_ttl: '30m'
                },
                busy_backpressure: {
                    enabled: true,
                    short_retry_after_max: '3s',
                    max_inline_wait: '8s'
                }
            }
        },
        status: {
            version: '',
            uptime: '',
            config_dir: '',
            clients: {}
        },
        serviceStatus: {
            os: '',
            install_command: '',
            install_hint: '',
            supported: true,
            installed: false,
            loaded: false,
            running: false,
            ok: false,
            detail: '',
            output: '',
            error: ''
        },
        serviceForm: {
            force: false,
            stdout_path: '',
            stderr_path: ''
        },
        toasts: [],
        toastCounter: 0,
        toastTimeouts: {},
        integrationResults: {},
        showAddProviderModal: false,
        showEditProviderModal: false,
        providerForm: {
            name: '',
            base_url: '',
            model: '',
            reasoning_effort: '',
            thinking_budget_tokens: 0,
            api_keys_text: '',
            priority: 1,
            enabled: true
        },
        editingProviderName: '',
        editingProviderKeyCount: 0,

        // Helpers
        withDefaultGlobalConfig(cfg) {
            // Merge (shallow) and also ensure nested notifications exists.
            const def = this.globalConfig;
            const out = { ...def, ...(cfg || {}) };
            out.notifications = { ...def.notifications, ...((cfg && cfg.notifications) ? cfg.notifications : {}) };
            out.circuit_breaker = { ...def.circuit_breaker, ...((cfg && cfg.circuit_breaker) ? cfg.circuit_breaker : {}) };
            out.routing = { ...def.routing, ...((cfg && cfg.routing) ? cfg.routing : {}) };
            out.routing.sticky_sessions = {
                ...def.routing.sticky_sessions,
                ...((cfg && cfg.routing && cfg.routing.sticky_sessions) ? cfg.routing.sticky_sessions : {})
            };
            out.routing.busy_backpressure = {
                ...def.routing.busy_backpressure,
                ...((cfg && cfg.routing && cfg.routing.busy_backpressure) ? cfg.routing.busy_backpressure : {})
            };
            return out;
        },

        withDefaultServiceStatus(status) {
            const def = this.serviceStatus;
            return {
                ...def,
                ...(status || {}),
                supported: status && typeof status.supported === 'boolean' ? status.supported : def.supported,
                installed: status && typeof status.installed === 'boolean' ? status.installed : def.installed,
                loaded: status && typeof status.loaded === 'boolean' ? status.loaded : def.loaded,
                running: status && typeof status.running === 'boolean' ? status.running : def.running,
                ok: status && typeof status.ok === 'boolean' ? status.ok : def.ok,
                install_command: String((status && status.install_command) || ''),
                install_hint: String((status && status.install_hint) || ''),
                detail: String((status && status.detail) || ''),
                output: String((status && status.output) || ''),
                error: String((status && status.error) || '')
            };
        },

        normalizeLocale(locale) {
            const value = String(locale || '').trim();
            return this.supportedLocales.includes(value) ? value : 'en';
        },

        lookupMessage(locale, key) {
            const root = this.messages[this.normalizeLocale(locale)];
            return String(key || '')
                .split('.')
                .filter(Boolean)
                .reduce((value, part) => {
                    if (!value || typeof value !== 'object' || !(part in value)) {
                        return null;
                    }
                    return value[part];
                }, root);
        },

        t(key) {
            return this.lookupMessage(this.locale, key)
                ?? this.lookupMessage('en', key)
                ?? key;
        },

        tf(key, params = {}) {
            return String(this.t(key)).replace(/\{(\w+)\}/g, (_, name) => {
                return Object.prototype.hasOwnProperty.call(params, name) ? String(params[name]) : `{${name}}`;
            });
        },

        applyLocale() {
            document.documentElement.lang = this.locale === 'zh-CN' ? 'zh-CN' : 'en';
            document.title = this.t('meta.title');
        },

        initLocale() {
            const stored = localStorage.getItem('clipal.locale');
            this.locale = this.normalizeLocale(stored);
            this.applyLocale();
        },

        setLocale(locale) {
            this.locale = this.normalizeLocale(locale);
            localStorage.setItem('clipal.locale', this.locale);
            this.applyLocale();
        },

        focusLocaleButton(locale) {
            this.$nextTick(() => {
                const ref = locale === 'zh-CN' ? this.$refs.localeZh : this.$refs.localeEn;
                if (ref && typeof ref.focus === 'function') {
                    ref.focus();
                }
            });
        },

        moveLocale(direction) {
            const index = this.supportedLocales.indexOf(this.locale);
            if (index === -1) {
                return;
            }
            const next = (index + direction + this.supportedLocales.length) % this.supportedLocales.length;
            const locale = this.supportedLocales[next];
            this.setLocale(locale);
            this.focusLocaleButton(locale);
        },

        // Initialization
        async init() {
            this.initLocale();
            this.initTheme();
            
            // Initial data load
            this.isLoading = true;
            try {
                await Promise.all([
                    this.refreshStatus(),
                    this.loadServiceStatus(),
                    this.loadProviders(),
                    this.loadGlobalConfig(),
                    this.loadIntegrations(true)
                ]);
                this.$nextTick(() => {
                    this.initSortable();
                });
            } finally {
                this.isLoading = false;
            }

            // Simple poller: 3s refresh while on Services/Status tabs.
            this.servicePoll = setInterval(() => {
                if (this.activeTab === 'services') {
                    this.loadServiceStatus(true);
                }
                if (this.activeTab === 'status') {
                    this.refreshStatus();
                }
            }, 3000);
        },

        // Theme Management
        initTheme() {
            this.applyTheme(this.theme);
            
            // Listen for system preference changes if in system mode
            window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', e => {
                if (this.theme === 'system') {
                    this.applyTheme('system');
                }
            });
        },

        toggleTheme() {
            const modes = ['system', 'light', 'dark'];
            const nextIndex = (modes.indexOf(this.theme) + 1) % modes.length;
            this.theme = modes[nextIndex];
            localStorage.setItem('theme', this.theme);
            this.applyTheme(this.theme);
        },

        applyTheme(theme) {
            const isDark = theme === 'dark' || 
                (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
            
            if (isDark) {
                document.documentElement.classList.add('dark');
            } else {
                document.documentElement.classList.remove('dark');
            }
        },

        get isDarkTheme() {
            return this.theme === 'dark' ||
                (this.theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
        },

        get brandIconSrc() {
            return this.isDarkTheme ? '/static/clipal-icon-dark.svg' : '/static/clipal-icon.svg';
        },

        get themeIcon() {
            if (this.theme === 'system') return '💻';
            return this.theme === 'dark' ? '🌙' : '☀️';
        },

        get themeLabel() {
            switch (this.theme) {
                case 'light':
                    return this.t('theme.light');
                case 'dark':
                    return this.t('theme.dark');
                default:
                    return this.t('theme.system');
            }
        },

        providerStatusEntries(client, state) {
            const current = String((client && client.current_provider) || '').trim();
            const providers = Array.isArray(client && client.providers) ? client.providers : [];
            const entries = providers
                .filter(p => p && String(p.name || '').trim())
                .map(p => ({ ...p }));

            if (current && !entries.some(p => String(p.name || '').trim() === current)) {
                entries.unshift({
                    name: current,
                    enabled: true,
                    priority: 0,
                    key_count: 0,
                    available_key_count: 0,
                    state: 'available',
                    detail: 'Current provider is active but has not appeared in the latest provider snapshot yet.'
                });
            }

            const sorted = entries.sort((a, b) => {
                const aPriority = Number(a && a.priority);
                const bPriority = Number(b && b.priority);
                const aValue = Number.isFinite(aPriority) && aPriority > 0 ? aPriority : Number.MAX_SAFE_INTEGER;
                const bValue = Number.isFinite(bPriority) && bPriority > 0 ? bPriority : Number.MAX_SAFE_INTEGER;
                if (aValue !== bValue) return aValue - bValue;
                return String(a.name || '').localeCompare(String(b.name || ''));
            });

            return sorted.filter(p => {
                const name = String(p.name || '').trim();
                const skip = String(p.skip_reason || '').trim();
                const isCurrent = !!current && name === current;
                const isDisabled = p.enabled === false || !!skip;

                switch (state) {
                    case 'current':
                        return isCurrent;
                    case 'active':
                        return !isCurrent && !isDisabled && p.state === 'available';
                    case 'disabled':
                        return !isCurrent && p.state === 'disabled';
                    case 'cooling_down':
                        return !isCurrent && p.state === 'cooling_down';
                    case 'unavailable':
                        return !isCurrent && p.state === 'unavailable';
                    case 'recovery_probe':
                        return !isCurrent && p.state === 'recovery_probe';
                    default:
                        return false;
                }
            });
        },

        providerStatusGroups(client) {
            const groups = [
                { key: 'current', label: this.t('statusPage.groupCurrent') },
                { key: 'active', label: this.t('statusPage.groupActive') },
                { key: 'disabled', label: this.t('statusPage.groupDisabled') },
                { key: 'cooling_down', label: this.t('statusPage.groupCoolingDown') },
                { key: 'unavailable', label: this.t('statusPage.groupUnavailable') },
                { key: 'recovery_probe', label: this.t('statusPage.groupRecoveryProbe') }
            ];
            return groups.filter(group => this.providerStatusEntries(client, group.key).length > 0);
        },

        providerStatusChipClass(state, p) {
            if (state === 'current') return 'chip-primary';
            if (state === 'disabled') return 'chip-danger';
            if (state === 'cooling_down' || state === 'unavailable') return 'chip-warn';
            if (state === 'recovery_probe') return 'chip-info';
            return '';
        },

        // API Calls
        async apiCall(url, options = {}, background = false, suppressAlert = false) {
            if (!background) this.isLoading = true;
            try {
                // Minimum loading time to prevent flickering for fast requests
                const start = Date.now();
                
                const response = await fetch(url, {
                    ...options,
                    headers: {
                        'X-Clipal-UI': '1',
                        'Content-Type': 'application/json',
                        ...options.headers
                    }
                });

                if (!response.ok) {
                    const error = await response.json();
                    throw new Error(error.error || 'Request failed');
                }

                const data = await response.json();

                // Artificial delay for smoother UX if request was too fast
                if (!background) {
                    const elapsed = Date.now() - start;
                    if (elapsed < 300) await new Promise(r => setTimeout(r, 300 - elapsed));
                }

                return data;
            } catch (error) {
                if (!suppressAlert) {
                    this.showAlert('error', error.message);
                }
                throw error;
            } finally {
                if (!background) this.isLoading = false;
            }
        },

        // Status
        async refreshStatus() {
            try {
                this.status = await this.apiCall('/api/status', {}, true); // Background update
            } catch (error) {
                console.error('Failed to refresh status:', error);
            }
        },

        // Services
        async loadServiceStatus(background = false) {
            try {
                const status = await this.apiCall('/api/service/status', {}, !!background);
                this.serviceStatus = this.withDefaultServiceStatus(status);
                this.serviceBusyAction = '';
            } catch (error) {
                console.error('Failed to load service status:', error);
            }
        },

        async serviceAction(action) {
            const disabledReason = this.serviceActionDisabledReason(action);
            if (disabledReason) {
                return;
            }
            if (action === 'uninstall' && !confirm(this.t('services.confirmUninstall'))) return;
            if (action === 'stop' && !confirm(this.t('services.confirmStop'))) return;
            if (action === 'restart' && !confirm(this.t('services.confirmRestart'))) return;

            // Best-effort request: the service might stop/restart mid-flight.
            this.serviceBusyAction = action;
            try {
                await fetch(`/api/service/${action}`, {
                    method: 'POST',
                    headers: { 'X-Clipal-UI': '1', 'Content-Type': 'application/json' },
                    body: JSON.stringify(this.serviceForm),
                    keepalive: true
                });
            } catch (e) {
                // Ignore network errors (common when the service restarts).
            }

            this.showAlert('info', this.tf('services.requestedAction', { action: this.serviceActionLabel(action) }));
            // Staggered refresh to cover fast/slow restart paths.
            setTimeout(() => this.loadServiceStatus(true), 1500);
            setTimeout(() => this.loadServiceStatus(true), 3500);
            setTimeout(() => this.loadServiceStatus(true), 7000);
            setTimeout(() => {
                if (this.serviceBusyAction === action) {
                    this.serviceBusyAction = '';
                }
            }, 9000);
        },

        async copyToClipboard(text) {
            const value = String(text || '').trim();
            if (!value) return false;

            try {
                if (navigator.clipboard && navigator.clipboard.writeText) {
                    await navigator.clipboard.writeText(value);
                    return true;
                }
            } catch (e) {
                // fall through to legacy approach
            }

            try {
                const ta = document.createElement('textarea');
                ta.value = value;
                ta.setAttribute('readonly', '');
                ta.style.position = 'absolute';
                ta.style.left = '-9999px';
                document.body.appendChild(ta);
                ta.select();
                const ok = document.execCommand('copy');
                document.body.removeChild(ta);
                return ok;
            } catch (e) {
                return false;
            }
        },

        async copyServiceInstallCommand() {
            const ok = await this.copyToClipboard(this.serviceStatus.install_command);
            if (ok) this.showAlert('success', this.t('services.installCommandCopied'));
            else this.showAlert('error', this.t('services.copyCommandFailed'));
        },

        serviceRuntimeLabel() {
            if (!this.serviceStatus.supported) return this.t('services.unsupported');
            if (!this.serviceStatus.installed) return this.t('services.notInstalled');
            if (this.serviceStatus.running) return this.t('services.running');
            if (this.serviceStatus.loaded) return this.t('services.stopped');
            return this.t('services.needsAttention');
        },

        serviceRuntimeClass() {
            if (!this.serviceStatus.supported) return 'pill-danger';
            if (!this.serviceStatus.installed) return 'pill-warning';
            if (this.serviceStatus.running) return 'pill-success';
            if (this.serviceStatus.loaded) return 'pill-warning';
            return 'pill-danger';
        },

        serviceActionIsBusy(action) {
            return this.serviceBusyAction === action;
        },

        serviceActionDisabledReason(action) {
            if (this.serviceActionIsBusy(action) || action === 'check') {
                return '';
            }
            if (!this.serviceStatus.supported) {
                return this.t('services.notSupportedReason');
            }

            switch (String(action || '').trim()) {
                case 'install':
                    if (!this.serviceStatus.installed) return '';
                    return this.serviceForm.force
                        ? ''
                        : this.t('services.alreadyInstalledReason');
                case 'start':
                    if (!this.serviceStatus.installed) return this.t('services.installFirst');
                    if (this.serviceStatus.running) return this.t('services.alreadyRunning');
                    return '';
                case 'stop':
                    if (!this.serviceStatus.installed) return this.t('services.installFirst');
                    if (!this.serviceStatus.running) return this.t('services.notRunning');
                    return '';
                case 'restart':
                    if (!this.serviceStatus.installed) return this.t('services.installFirst');
                    if (!this.serviceStatus.running) return this.t('services.notRunning');
                    return '';
                case 'uninstall':
                    if (!this.serviceStatus.installed) return this.t('services.serviceNotInstalled');
                    return '';
                default:
                    return '';
            }
        },

        integrationStateLabel(state) {
            switch (String(state || '').trim()) {
                case 'configured':
                    return this.t('integrations.stateConfigured');
                case 'error':
                    return this.t('integrations.stateNeedsAttention');
                default:
                    return this.t('integrations.stateNotConfigured');
            }
        },

        integrationStateClass(state) {
            switch (String(state || '').trim()) {
                case 'configured':
                    return 'pill-success';
                case 'error':
                    return 'pill-danger';
                default:
                    return 'pill-warning';
            }
        },

        integrationProductName(product) {
            switch (String(product || '').trim()) {
                case 'claude':
                    return 'Claude Code';
                case 'codex':
                    return 'Codex CLI';
                case 'opencode':
                    return 'OpenCode';
                case 'gemini':
                    return 'Gemini CLI';
                case 'continue':
                    return 'Continue';
                case 'aider':
                    return 'Aider';
                case 'goose':
                    return 'Goose';
                default:
                    return product;
            }
        },


        integrationApplyLabel() {
            return this.t('integrations.apply');
        },

        integrationRollbackLabel() {
            return this.t('integrations.rollback');
        },

        integrationActionIsBusy(integration, action) {
            if (!integration || this.integrationBusyProduct !== integration.product) {
                return false;
            }
            return action === 'apply'
                ? integration.state !== 'configured'
                : integration.state === 'configured';
        },

        integrationActionDisabledReason(integration, action) {
            if (!integration || this.integrationActionIsBusy(integration, action)) {
                return '';
            }
            if (action === 'apply') {
                return integration.state === 'configured' ? this.t('integrations.alreadyUsing') : '';
            }
            if (!integration.backup_available) {
                return this.t('integrations.noBackupYet');
            }
            return integration.state !== 'configured'
                ? this.t('integrations.restoreOnlyActive')
                : '';
        },


        integrationProductNote(product) {
            switch (String(product || '').trim()) {
                case 'claude':
                    return this.t('integrations.noteClaude');
                case 'codex':
                    return this.t('integrations.noteCodex');
                case 'opencode':
                    return this.t('integrations.noteOpencode');
                case 'gemini':
                    return this.t('integrations.noteGemini');
                case 'continue':
                    return this.t('integrations.noteContinue');
                case 'aider':
                    return this.t('integrations.noteAider');
                case 'goose':
                    return this.t('integrations.noteGoose');
                default:
                    return this.t('integrations.noteDefault');
            }
        },


        integrationPreviewValue(content, emptyLabel) {
            const value = String(content || '');
            return value.trim() ? value : emptyLabel;
        },

        integrationSecondaryPreviewLabel(integration) {
            return integration && integration.state === 'configured' && integration.backup_available
                ? this.t('integrations.latestBackup')
                : this.t('integrations.afterApply');
        },

        integrationSecondaryPreviewContent(integration) {
            if (integration && integration.state === 'configured' && integration.backup_available) {
                return String((integration && integration.backup_content) || '');
            }
            return String((integration && integration.planned_content) || '');
        },

        integrationSecondaryPreviewEmptyLabel(integration) {
            if (integration && integration.state === 'configured' && integration.backup_available) {
                return integration.backup_target_existed
                    ? this.t('integrations.backupEmpty')
                    : this.t('integrations.originalFileMissing');
            }
            return this.t('integrations.noPlannedChanges');
        },

        integrationResultFor(integration) {
            const product = String((integration && integration.product) || '').trim();
            if (!product) {
                return null;
            }
            return this.integrationResults[product] || null;
        },

        integrationResultClass(result) {
            const type = String((result && result.type) || 'info').trim() || 'info';
            return `integration-result--${type}`;
        },

        integrationResultBadgeClass(result) {
            const type = String((result && result.type) || 'info').trim() || 'info';
            return `integration-result__badge--${type}`;
        },

        integrationResultBadgeLabel(type) {
            switch (String(type || '').trim()) {
                case 'success':
                    return this.t('integrations.resultUpdated');
                case 'error':
                    return this.t('integrations.resultError');
                default:
                    return this.t('integrations.resultNotice');
            }
        },

        setIntegrationResult(product, type, title, message) {
            const name = String(product || '').trim();
            if (!name) {
                return;
            }
            this.integrationResults = {
                ...this.integrationResults,
                [name]: {
                    type: String(type || 'info').trim() || 'info',
                    title: String(title || '').trim(),
                    message: String(message || '').trim()
                }
            };
        },

        clearIntegrationResult(product) {
            const name = String(product || '').trim();
            if (!name || !this.integrationResults[name]) {
                return;
            }
            const next = { ...this.integrationResults };
            delete next[name];
            this.integrationResults = next;
        },

        normalizeIntegration(item) {
            return {
                ...item,
                name: this.integrationProductName(item.product) || item.name,
                current_content: String((item && item.current_content) || ''),
                planned_content: String((item && item.planned_content) || ''),
                backup_content: String((item && item.backup_content) || ''),
                backup_target_existed: !!(item && item.backup_target_existed)
            };
        },


        async loadIntegrations(background = false) {
            try {
                const items = await this.apiCall('/api/integrations', {}, !!background);
                const integrations = (items || []).map(item => this.normalizeIntegration(item));
                this.integrations = integrations;
                const knownProducts = new Set(integrations.map(item => String((item && item.product) || '').trim()).filter(Boolean));
                this.integrationResults = Object.fromEntries(
                    Object.entries(this.integrationResults).filter(([product]) => knownProducts.has(product))
                );
            } catch (error) {
                console.error('Failed to load integrations:', error);
                this.integrations = [];
            }
        },

        async runIntegrationAction(product, action) {
            const name = String(product || '').trim();
            const op = String(action || '').trim();
            if (!name || !op) return;

            this.clearIntegrationResult(name);
            this.integrationBusyProduct = name;
            try {
                const response = await this.apiCall(`/api/integrations/${encodeURIComponent(name)}/${op}`, {
                    method: 'POST'
                }, false, true);
                this.integrations = (this.integrations || []).map(item =>
                    item && item.product === name
                        ? this.normalizeIntegration({ ...response.status, name: this.integrationProductName(name) || response.status.name })
                        : item
                );
                if (op === 'apply') {
                    this.setIntegrationResult(
                        name,
                        'success',
                        this.t('integrations.resultUsingClipalTitle'),
                        this.t('integrations.resultRestartMessage')
                    );
                } else {
                    this.setIntegrationResult(
                        name,
                        'success',
                        this.t('integrations.resultRestoredTitle'),
                        this.t('integrations.resultRestartMessage')
                    );
                }
            } catch (error) {
                const title = op === 'apply'
                    ? this.t('integrations.resultApplyErrorTitle')
                    : this.t('integrations.resultRestoreErrorTitle');
                this.setIntegrationResult(name, 'error', title, error.message);
                this.showAlert('error', error.message, title);
                console.error(`Failed to ${op} integration:`, error);
            } finally {
                this.integrationBusyProduct = '';
            }
        },

        // Providers
        clientLabel(clientType) {
            const match = this.clientOptions.find(c => c.value === clientType);
            return match ? match.label : clientType;
        },

        providerToastClientLabel() {
            return this.clientLabel(this.selectedClient);
        },

        modeLabel(mode) {
            return String(mode || '').trim() === 'manual'
                ? this.t('providers.modeManual')
                : this.t('providers.modeAuto');
        },

        modeToggleLabel() {
            return String(this.clientConfig.mode || '').trim() === 'auto'
                ? this.t('providers.switchToManual')
                : this.t('providers.backToAuto');
        },

        modeToggleTitle() {
            return String(this.clientConfig.mode || '').trim() === 'auto' && !this.hasEnabledProviders
                ? this.t('providers.enableProviderFirst')
                : '';
        },

        configuredKeyCountLabel(count) {
            return this.tf('providers.configuredCount', { count: Number(count || 0) });
        },

        providerPinBadgeTitle() {
            return this.t('providers.pinnedProvider');
        },

        providerStatusText(enabled) {
            return enabled ? this.t('common.active') : this.t('common.disabled');
        },

        providerToggleTitle(provider) {
            const isPinned = String(this.clientConfig.mode || '').trim() === 'manual'
                && provider
                && provider.name === this.clientConfig.pinned_provider;
            if (isPinned) {
                return provider.enabled
                    ? this.t('providers.pinnedProviderCannotDisable')
                    : this.t('providers.enablePinnedProvider');
            }
            return provider && provider.enabled ? this.t('providers.disable') : this.t('providers.enable');
        },

        providerPinTitle(provider) {
            const isPinned = String(this.clientConfig.mode || '').trim() === 'manual'
                && provider
                && provider.name === this.clientConfig.pinned_provider;
            return isPinned ? this.t('providers.pinnedProvider') : this.t('providers.pinTitle');
        },

        providerEditKeyHint() {
            const count = Number(this.editingProviderKeyCount || 0);
            if (this.locale === 'zh-CN') {
                return this.tf('modal.provider.keepExistingKeys', { count });
            }
            if (count === 1) {
                return this.t('modal.provider.keepExistingKey');
            }
            return this.tf('modal.provider.keepExistingKeys', { count });
        },

        providerOverrideSupport() {
            const support = this.clientConfig && this.clientConfig.override_support;
            if (!support || typeof support !== 'object') {
                return this.defaultProviderOverrideSupport();
            }
            return {
                model: !!support.model,
                openai: {
                    reasoning_effort: !!(support.openai && support.openai.reasoning_effort)
                },
                claude: {
                    thinking_budget_tokens: !!(support.claude && support.claude.thinking_budget_tokens)
                }
            };
        },

        defaultProviderOverrideSupport() {
            return {
                model: false,
                openai: {
                    reasoning_effort: false
                },
                claude: {
                    thinking_budget_tokens: false
                }
            };
        },

        providerSupportsModelOverride() {
            return this.providerOverrideSupport().model;
        },

        providerSupportsReasoningEffort() {
            return this.providerOverrideSupport().openai.reasoning_effort;
        },

        providerSupportsThinkingBudget() {
            return this.providerOverrideSupport().claude.thinking_budget_tokens;
        },

        providerHasAnyOverrideSupport() {
            return this.providerSupportsModelOverride()
                || this.providerSupportsReasoningEffort()
                || this.providerSupportsThinkingBudget();
        },

        normalizeThinkingBudgetTokens(value) {
            const parsed = Number.parseInt(String(value ?? '').trim(), 10);
            if (Number.isNaN(parsed) || parsed < 0) {
                return 0;
            }
            return parsed;
        },

        levelLabel(level) {
            const value = String(level || '').trim().toLowerCase();
            return this.t(`level.${value}`);
        },

        serviceActionLabel(action) {
            switch (String(action || '').trim()) {
                case 'install':
                    return this.t('services.installService');
                case 'start':
                    return this.t('services.start');
                case 'restart':
                    return this.t('services.restart');
                case 'stop':
                    return this.t('services.stop');
                case 'uninstall':
                    return this.t('services.uninstall');
                case 'check':
                    return this.t('services.check');
                default:
                    return action;
            }
        },

        integrationBackupMeta(integration) {
            return integration && integration.backup_available
                ? this.t('integrations.backupAvailable')
                : this.t('integrations.noBackupMeta');
        },

        integrationCurrentPreviewEmptyLabel() {
            return this.t('integrations.fileDoesNotExistYet');
        },

        statusSystemInfoLabel() {
            return this.t('statusPage.systemInfo');
        },

        statusModeLabel(mode) {
            return this.modeLabel(mode || 'auto');
        },

        statusMetricProviders(count) {
            return this.tf('statusPage.providersCount', { count: Number(count || 0) });
        },

        statusMetricEnabled(count) {
            return this.tf('statusPage.enabledCount', { count: Number(count || 0) });
        },

        statusCircuitBreakerSummary() {
            return this.tf('statusPage.circuitBreakerSummary', {
                failure: this.globalConfig.circuit_breaker.failure_threshold,
                success: this.globalConfig.circuit_breaker.success_threshold,
                timeout: this.globalConfig.circuit_breaker.open_timeout
            });
        },

        get hasEnabledProviders() {
            return (this.providers || []).some(p => !!p.enabled);
        },

        modeHelpText() {
            if ((this.clientConfig.mode || '') === 'manual') {
                return this.t('providers.modeHelpManual');
            }
            return this.t('providers.modeHelpAuto');
        },

        providerStatusLabel(p) {
            if (!p) return '';
            const name = String(p.name || '').trim();
            if (!name) return '';
            return name;
        },

        providerStatusTitle(p) {
            const name = String((p && p.name) || '').trim();
            if (!name) return '';

            const label = String((p && p.label) || '').trim();
            const base = label || name;
            if (!base) return '';

            const detail = String((p && p.detail) || '').trim();
            let title = detail ? `${base}\n${detail}` : base;

            const available = Number((p && p.available_key_count) || 0);
            const total = Number((p && p.key_count) || 0);
            if (total > 0) {
                title = `${title}\n${this.tf('statusPage.keysAvailable', { available, total })}`;
            }

            const skip = String((p && p.skip_reason) || '').trim();
            if (skip !== 'deactivated') return title;

            const msg = String((p && p.deactivated_message) || '').trim();
            if (!msg) return title;

            const max = 300;
            const clipped = msg.length > max ? (msg.slice(0, max) + '...') : msg;
            title = `${title}\n${clipped}`;
            return title;
        },

        initSortable() {
            const el = document.getElementById('providers-list');
            if (!el || typeof Sortable === 'undefined') return;
            
            if (this.sortableInstance) {
                this.sortableInstance.destroy();
            }

            this.sortableInstance = Sortable.create(el, {
                animation: 250,
                // Make the entire card draggable
                handle: '.provider-card',
                // Filter out buttons and inputs so they remain clickable
                filter: '.btn, button, input, .pill, .provider-card__actions',
                preventOnFilter: false,
                ghostClass: 'sortable-ghost',
                chosenClass: 'sortable-chosen',
                dragClass: 'sortable-drag',
                forceFallback: false,
                onEnd: async (evt) => {
                    if (evt.oldIndex === evt.newIndex) return;

                    const previousProviders = (this.providers || []).map(provider => ({ ...provider }));
                    const previousOrder = previousProviders.map(provider => provider.name);

                    // Sortable has already mutated the DOM, but Alpine still thinks the
                    // previous keyed order is intact. Restore the old DOM order first so
                    // Alpine can apply the reordered state without scrambling cards.
                    this.syncSortableDomOrder(previousOrder);
                    this.applyLocalProviderReorder(evt.oldIndex, evt.newIndex);
                    await this.afterProviderRender();
                    const names = (this.providers || []).map(provider => provider.name);

                    try {
                        await this.apiCall(`/api/providers/${this.selectedClient}/_reorder`, {
                            method: 'PUT',
                            body: JSON.stringify({ providers: names })
                        }, true);
                    } catch (error) {
                        console.error('Failed to reorder providers:', error);
                        this.providers = previousProviders;
                        await this.afterProviderRender();
                    }
                }
            });
        },

        syncSortableDomOrder(order) {
            if (!this.sortableInstance || typeof this.sortableInstance.sort !== 'function') {
                return;
            }
            this.sortableInstance.sort(Array.isArray(order) ? order : [], false);
        },

        afterProviderRender() {
            return new Promise(resolve => {
                this.$nextTick(() => {
                    this.syncSortableDomOrder((this.providers || []).map(provider => provider.name));
                    resolve();
                });
            });
        },

        applyLocalProviderReorder(oldIndex, newIndex) {
            const providers = Array.isArray(this.providers)
                ? this.providers.map(provider => ({ ...provider }))
                : [];
            if (oldIndex < 0 || newIndex < 0 || oldIndex >= providers.length || newIndex >= providers.length) {
                return;
            }

            const [moved] = providers.splice(oldIndex, 1);
            if (!moved) {
                return;
            }
            providers.splice(newIndex, 0, moved);

            this.providers = providers.map((provider, index) => ({
                ...provider,
                priority: index + 1
            }));
        },

        async loadProviders(background = false) {
            try {
                const [providers, clientCfg] = await Promise.all([
                    this.apiCall(`/api/providers/${this.selectedClient}`, {}, background),
                    this.apiCall(`/api/client-config/${this.selectedClient}`, {}, true)
                ]);
                this.providers = providers || [];
                this.clientConfig = {
                    ...this.clientConfig,
                    ...(clientCfg || {}),
                    override_support: {
                        ...this.defaultProviderOverrideSupport(),
                        ...((clientCfg && clientCfg.override_support) ? clientCfg.override_support : {})
                    }
                };
            } catch (error) {
                console.error('Failed to load providers:', error);
                this.providers = [];
            }
        },

        async selectClient(clientType) {
            if (this.selectedClient === clientType) {
                return;
            }
            this.selectedClient = clientType;
            await this.loadProviders();
            this.$nextTick(() => {
                this.initSortable();
            });
        },

        async saveClientConfig(successToast = null) {
            try {
                await this.apiCall(`/api/client-config/${this.selectedClient}`, {
                    method: 'PUT',
                    body: JSON.stringify(this.clientConfig)
                });
                if (successToast && (successToast.title || successToast.message)) {
                    this.showAlert('success', successToast.message || '', successToast.title || '');
                }
                await this.refreshStatus();
            } catch (error) {
                console.error('Failed to save client config:', error);
            }
        },

        async setClientMode(mode) {
            const m = String(mode || '').toLowerCase();
            if (m !== 'auto' && m !== 'manual') return;

            if (m === 'manual' && !this.hasEnabledProviders) {
                this.showAlert('error', this.t('providers.enableBeforeManual'));
                return;
            }

            this.clientConfig.mode = m;
            if (m === 'auto') {
                this.clientConfig.pinned_provider = '';
            } else {
                // Default pin: prefer current provider, else highest priority enabled provider.
                const pinned = String(this.clientConfig.pinned_provider || '').trim();
                const pinnedProvider = pinned ? (this.providers || []).find(p => p && p.name === pinned) : null;
                const pinnedOk = pinnedProvider && !!pinnedProvider.enabled;

                if (!pinnedOk) {
                    // Best-effort refresh so "current provider" is as accurate as possible.
                    try {
                        await this.refreshStatus();
                    } catch (e) {
                        // Ignore refresh failures; we fall back to local provider list.
                    }

                    const st = (this.status && this.status.clients) ? this.status.clients[this.selectedClient] : null;
                    const cur = st ? String(st.current_provider || '').trim() : '';
                    const curProvider = cur ? (this.providers || []).find(p => p && p.name === cur) : null;

                    if (curProvider && curProvider.name && !!curProvider.enabled) {
                        this.clientConfig.pinned_provider = curProvider.name;
                    } else {
                        const enabled = (this.providers || []).filter(p => p && p.name && !!p.enabled);
                        enabled.sort((a, b) => Number(a.priority || 0) - Number(b.priority || 0));
                        this.clientConfig.pinned_provider = (enabled[0] && enabled[0].name) ? enabled[0].name : '';
                    }
                }
            }
            const client = this.providerToastClientLabel();
            const pinned = String(this.clientConfig.pinned_provider || '').trim();
            if (m === 'auto') {
                await this.saveClientConfig({
                    title: this.tf('providers.switchedToAutoTitle', { client }),
                    message: this.t('providers.switchedToAutoMessage')
                });
                return;
            }
            await this.saveClientConfig({
                title: this.tf('providers.switchedToManualTitle', { client }),
                message: pinned
                    ? this.tf('providers.switchedToManualMessagePinned', { provider: pinned })
                    : this.t('providers.switchedToManualMessage')
            });
        },

        async pinProvider(name) {
            const v = String(name || '').trim();
            if (!v) return;

            const p = (this.providers || []).find(x => x && x.name === v);
            if (p && p.enabled === false) {
                this.showAlert('error', this.t('providers.enableBeforePinning'));
                return;
            }

            this.clientConfig.mode = 'manual';
            this.clientConfig.pinned_provider = v;
            const client = this.providerToastClientLabel();
            await this.saveClientConfig({
                title: this.tf('providers.pinnedTitle', { client, provider: v }),
                message: this.t('providers.pinnedMessage')
            });
        },

        async toggleProvider(provider, event) {
            const oldEnabled = provider.enabled;
            const newEnabled = !!event.target.checked;
            provider.enabled = newEnabled;

            try {
                await this.apiCall(
                    `/api/providers/${this.selectedClient}/${encodeURIComponent(provider.name)}`,
                    {
                        method: 'PUT',
                        body: JSON.stringify({ enabled: newEnabled })
                    },
                    true // Background op for toggles to feel instant
                );
                const client = this.providerToastClientLabel();
                if (newEnabled) {
                    this.showAlert(
                        'success',
                        this.t('providers.enabledMessage'),
                        this.tf('providers.enabledTitle', { provider: provider.name, client })
                    );
                } else {
                    this.showAlert(
                        'success',
                        this.t('providers.disabledMessage'),
                        this.tf('providers.disabledTitle', { provider: provider.name, client })
                    );
                }
                await this.refreshStatus();
            } catch (error) {
                provider.enabled = oldEnabled;
                event.target.checked = oldEnabled;
                console.error('Failed to toggle provider:', error);
            }
        },

        async saveProvider() {
            try {
                const payload = {
                    name: this.providerForm.name,
                    base_url: this.providerForm.base_url,
                    priority: this.providerForm.priority,
                    enabled: this.providerForm.enabled
                };
                const overrides = {};
                if (this.providerSupportsModelOverride()) {
                    overrides.model = String(this.providerForm.model || '');
                }
                if (this.providerSupportsReasoningEffort()) {
                    overrides.openai = {
                        reasoning_effort: String(this.providerForm.reasoning_effort || '')
                    };
                }
                if (this.providerSupportsThinkingBudget()) {
                    overrides.claude = {
                        thinking_budget_tokens: this.normalizeThinkingBudgetTokens(this.providerForm.thinking_budget_tokens)
                    };
                }
                if (Object.keys(overrides).length > 0) {
                    payload.overrides = overrides;
                }
                const keys = String(this.providerForm.api_keys_text || '')
                    .split('\n')
                    .map(v => v.trim())
                    .filter(Boolean);
                if (keys.length === 1) {
                    payload.api_key = keys[0];
                } else if (keys.length > 1) {
                    payload.api_keys = keys;
                }
                if (this.showEditProviderModal) {
                    // Update existing provider
                    await this.apiCall(
                        `/api/providers/${this.selectedClient}/${encodeURIComponent(this.editingProviderName)}`,
                        {
                            method: 'PUT',
                            body: JSON.stringify(payload)
                        }
                    );
                    this.showAlert(
                        'success',
                        this.tf('providers.updatedMessage', { client: this.providerToastClientLabel() }),
                        this.tf('providers.updatedTitle', { provider: payload.name })
                    );
                } else {
                    // Add new provider
                    await this.apiCall(
                        `/api/providers/${this.selectedClient}`,
                        {
                            method: 'POST',
                            body: JSON.stringify(payload)
                        }
                    );
                    this.showAlert(
                        'success',
                        this.tf('providers.addedMessage', { client: this.providerToastClientLabel() }),
                        this.tf('providers.addedTitle', { provider: payload.name })
                    );
                }

                this.closeModals();
                await this.loadProviders();
                await this.refreshStatus();
            } catch (error) {
                console.error('Failed to save provider:', error);
            }
        },

        editProvider(provider) {
            this.providerForm = {
                name: provider.name,
                base_url: provider.base_url,
                model: String((provider.overrides && provider.overrides.model) || ''),
                reasoning_effort: String((provider.overrides && provider.overrides.openai && provider.overrides.openai.reasoning_effort) || ''),
                thinking_budget_tokens: Number((provider.overrides && provider.overrides.claude && provider.overrides.claude.thinking_budget_tokens) || 0),
                api_keys_text: '',
                priority: provider.priority,
                enabled: !!provider.enabled
            };
            this.editingProviderName = provider.name;
            this.editingProviderKeyCount = Number(provider.key_count || 0);
            this.showEditProviderModal = true;
        },

        async deleteProvider(name) {
            if (!confirm(this.tf('providers.deleteConfirm', { name }))) {
                return;
            }

            try {
                await this.apiCall(
                    `/api/providers/${this.selectedClient}/${encodeURIComponent(name)}`,
                    { method: 'DELETE' }
                );
                this.showAlert(
                    'success',
                    this.tf('providers.deletedMessage', { client: this.providerToastClientLabel() }),
                    this.tf('providers.deletedTitle', { name })
                );
                await this.loadProviders();
                await this.refreshStatus();
            } catch (error) {
                console.error('Failed to delete provider:', error);
            }
        },

        openAddProviderModal() {
            const maxPriority = this.providers.reduce((max, p) => {
                const pr = typeof p.priority === 'number' ? p.priority : 0;
                return pr > max ? pr : max;
            }, 0);
            this.providerForm = {
                name: '',
                base_url: '',
                model: '',
                reasoning_effort: '',
                thinking_budget_tokens: 0,
                api_keys_text: '',
                priority: maxPriority + 1,
                enabled: true
            };
            this.editingProviderName = '';
            this.editingProviderKeyCount = 0;
            this.showAddProviderModal = true;
        },

        // Global Config
        async loadGlobalConfig() {
            try {
                const cfg = await this.apiCall('/api/config/global');
                this.globalConfig = this.withDefaultGlobalConfig(cfg);
            } catch (error) {
                console.error('Failed to load global config:', error);
            }
        },

        async saveGlobalConfig() {
            try {
                await this.apiCall('/api/config/global/update', {
                    method: 'PUT',
                    body: JSON.stringify(this.globalConfig)
                });
                this.showAlert('success', this.t('settings.saveSuccess'));
                await this.refreshStatus();
            } catch (error) {
                console.error('Failed to save global config:', error);
            }
        },

        async exportConfig() {
            try {
                const response = await fetch('/api/config/export');
                const blob = await response.blob();
                const url = window.URL.createObjectURL(blob);
                const a = document.createElement('a');
                a.href = url;
                a.download = 'clipal-config.json';
                document.body.appendChild(a);
                a.click();
                window.URL.revokeObjectURL(url);
                document.body.removeChild(a);
                this.showAlert('success', this.t('settings.exportSuccess'));
            } catch (error) {
                this.showAlert('error', this.t('settings.exportFailure'));
                console.error('Failed to export config:', error);
            }
        },

        defaultToastTitle(type) {
            switch (String(type || '').trim()) {
                case 'success':
                    return this.t('toast.success');
                case 'error':
                    return this.t('toast.requestFailed');
                default:
                    return this.t('toast.notice');
            }
        },

        toastTypeLabel(type) {
            switch (String(type || '').trim()) {
                case 'success':
                    return this.t('toast.success');
                case 'error':
                    return this.t('toast.error');
                default:
                    return this.t('toast.info');
            }
        },

        clearToastTimer(id) {
            const key = String(id || '').trim();
            if (!key || !this.toastTimeouts[key]) {
                return;
            }
            clearTimeout(this.toastTimeouts[key]);
            delete this.toastTimeouts[key];
        },

        dismissToast(id) {
            const key = String(id || '').trim();
            if (!key) {
                return;
            }
            this.clearToastTimer(key);
            this.toasts = this.toasts.filter(toast => toast.id !== key);
        },

        pauseToast(id) {
            const key = String(id || '').trim();
            const toast = this.toasts.find(item => item.id === key);
            if (!toast) {
                return;
            }
            if (typeof toast.dismissAt === 'number' && toast.dismissAt > 0) {
                toast.remaining = Math.max(0, toast.dismissAt - Date.now());
            }
            this.clearToastTimer(key);
        },

        resumeToast(id) {
            const key = String(id || '').trim();
            const toast = this.toasts.find(item => item.id === key);
            if (!toast) {
                return;
            }
            const delay = Math.max(1, Number(toast.remaining) || 0);
            toast.dismissAt = Date.now() + delay;
            this.clearToastTimer(key);
            this.toastTimeouts[key] = setTimeout(() => {
                this.dismissToast(key);
            }, delay);
        },

        showAlert(type, message, title = '') {
            const body = String(message || '').trim();
            if (!body) {
                return;
            }

            const kind = ['success', 'error', 'info'].includes(type) ? type : 'info';
            const id = `toast-${Date.now()}-${++this.toastCounter}`;
            const toast = {
                id,
                type: kind,
                title: String(title || '').trim() || this.defaultToastTitle(kind),
                message: body,
                remaining: kind === 'error' ? 7000 : 5200,
                dismissAt: 0
            };

            const nextToasts = [...this.toasts, toast];
            while (nextToasts.length > 3) {
                const removed = nextToasts.shift();
                if (removed && removed.id) {
                    this.clearToastTimer(removed.id);
                }
            }

            this.toasts = nextToasts;
            this.resumeToast(id);
        },

        closeModals() {
            this.showAddProviderModal = false;
            this.showEditProviderModal = false;
            this.providerForm = {
                name: '',
                base_url: '',
                model: '',
                reasoning_effort: '',
                thinking_budget_tokens: 0,
                api_keys_text: '',
                priority: 1,
                enabled: true
            };
            this.editingProviderName = '';
            this.editingProviderKeyCount = 0;
        }
    };
}
