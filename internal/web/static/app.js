function app() {
    return {
        // State
        isLoading: false,
        theme: localStorage.getItem('theme') || 'system',
        activeTab: 'providers',
        servicePoll: null,
        selectedClient: 'claude-code',
        clientOptions: [
            { value: 'claude-code', label: 'Claude Code' },
            { value: 'codex', label: 'Codex' },
            { value: 'gemini', label: 'Gemini' }
        ],
        providers: [],
        clientConfig: {
            mode: 'auto',
            pinned_provider: ''
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
            ignore_count_tokens_failover: false
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
            ok: false,
            output: '',
            error: ''
        },
        serviceForm: {
            force: false,
            stdout_path: '',
            stderr_path: ''
        },
        alert: {
            show: false,
            type: 'info',
            message: ''
        },
        showAddProviderModal: false,
        showEditProviderModal: false,
        providerForm: {
            name: '',
            base_url: '',
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
            return out;
        },

        // Initialization
        async init() {
            this.initTheme();
            
            // Initial data load
            this.isLoading = true;
            try {
                await Promise.all([
                    this.refreshStatus(),
                    this.loadServiceStatus(),
                    this.loadProviders(),
                    this.loadGlobalConfig()
                ]);
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

        get themeIcon() {
            if (this.theme === 'system') return '💻';
            return this.theme === 'dark' ? '🌙' : '☀️';
        },

        get themeLabel() {
            return this.theme.charAt(0).toUpperCase() + this.theme.slice(1);
        },

        // API Calls
        async apiCall(url, options = {}, background = false) {
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
                this.showAlert('error', error.message);
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
                this.serviceStatus = await this.apiCall('/api/service/status', {}, !!background);
            } catch (error) {
                console.error('Failed to load service status:', error);
            }
        },

        async serviceAction(action) {
            if (action === 'uninstall' && !confirm('Uninstall the system service?')) return;
            if (action === 'stop' && !confirm('Stop the system service?')) return;
            if (action === 'restart' && !confirm('Restart the system service?')) return;

            // Best-effort request: the service might stop/restart mid-flight.
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

            this.showAlert('info', `Requested service ${action}. Refreshing...`);
            // Staggered refresh to cover fast/slow restart paths.
            setTimeout(() => this.loadServiceStatus(true), 1500);
            setTimeout(() => this.loadServiceStatus(true), 3500);
            setTimeout(() => this.loadServiceStatus(true), 7000);
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
            if (ok) this.showAlert('success', 'Install command copied');
            else this.showAlert('error', 'Failed to copy command');
        },

        // Providers
        clientLabel(clientType) {
            const match = this.clientOptions.find(c => c.value === clientType);
            return match ? match.label : clientType;
        },

        get hasEnabledProviders() {
            return (this.providers || []).some(p => !!p.enabled);
        },

        modeHelpText() {
            if ((this.clientConfig.mode || '') === 'manual') {
                return 'Manual (Pinned)\nAlways use the pinned provider.\nNo failover; failures return errors.';
            }
            return 'Auto (Failover)\nTries enabled providers by priority.\nSwitches on failures.';
        },

        providerStatusLabel(p) {
            if (!p) return '';
            const name = String(p.name || '').trim();
            if (!name) return '';

            const label = String(p.label || '').trim();
            if (label) return label;

            if (p.enabled === false) return `${name} (disabled)`;

            const skip = String(p.skip_reason || '').trim();
            if (skip === 'deactivated') {
                const r = String(p.deactivated_reason || '').trim() || 'deactivated';
                const d = String(p.deactivated_in || '').trim();
                return d ? `${name} (${r} ${d})` : `${name} (${r})`;
            }
            if (skip === 'circuit_open') {
                const d = String(p.circuit_open_in || '').trim();
                return d ? `${name} (circuit ${d})` : `${name} (circuit open)`;
            }
            if (skip === 'keys_exhausted') {
                const available = Number(p.available_key_count || 0);
                const total = Number(p.key_count || 0);
                return total > 0 ? `${name} (${available}/${total} keys available)` : `${name} (no keys available)`;
            }
            if (skip === 'disabled') return `${name} (disabled)`;

            const st = String(p.circuit_state || '').trim();
            if (st && st !== 'closed') {
                const d = String(p.circuit_open_in || '').trim();
                return d ? `${name} (${st} ${d})` : `${name} (${st})`;
            }

            return name;
        },

        providerStatusTitle(p) {
            const base = this.providerStatusLabel(p);
            if (!base) return '';

            const detail = String(p.detail || '').trim();
            let title = detail ? `${base}\n${detail}` : base;

            const available = Number(p.available_key_count || 0);
            const total = Number(p.key_count || 0);
            if (total > 0) {
                title = `${title}\nKeys available: ${available}/${total}`;
            }

            const skip = String(p.skip_reason || '').trim();
            if (skip !== 'deactivated') return title;

            const msg = String(p.deactivated_message || '').trim();
            if (!msg) return title;

            const max = 300;
            const clipped = msg.length > max ? (msg.slice(0, max) + '...') : msg;
            title = `${title}\n${clipped}`;
            return title;
        },

        async loadProviders() {
            try {
                const [providers, clientCfg] = await Promise.all([
                    this.apiCall(`/api/providers/${this.selectedClient}`),
                    this.apiCall(`/api/client-config/${this.selectedClient}`, {}, true)
                ]);
                this.providers = providers || [];
                this.clientConfig = { ...this.clientConfig, ...(clientCfg || {}) };
            } catch (error) {
                console.error('Failed to load providers:', error);
                this.providers = [];
            }
        },

        selectClient(clientType) {
            if (this.selectedClient === clientType) {
                return;
            }
            this.selectedClient = clientType;
            this.loadProviders();
        },

        async saveClientConfig() {
            try {
                await this.apiCall(`/api/client-config/${this.selectedClient}`, {
                    method: 'PUT',
                    body: JSON.stringify(this.clientConfig)
                });
                this.showAlert('success', 'Client configuration saved');
                await this.refreshStatus();
            } catch (error) {
                console.error('Failed to save client config:', error);
            }
        },

        async setClientMode(mode) {
            const m = String(mode || '').toLowerCase();
            if (m !== 'auto' && m !== 'manual') return;

            if (m === 'manual' && !this.hasEnabledProviders) {
                this.showAlert('error', 'Enable a provider before switching to manual mode');
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
            await this.saveClientConfig();
        },

        async pinProvider(name) {
            const v = String(name || '').trim();
            if (!v) return;

            const p = (this.providers || []).find(x => x && x.name === v);
            if (p && p.enabled === false) {
                this.showAlert('error', 'Enable the provider before pinning it');
                return;
            }

            this.clientConfig.mode = 'manual';
            this.clientConfig.pinned_provider = v;
            await this.saveClientConfig();
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
                this.showAlert('success', newEnabled ? 'Provider enabled' : 'Provider disabled');
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
                    this.showAlert('success', 'Provider updated successfully');
                } else {
                    // Add new provider
                    await this.apiCall(
                        `/api/providers/${this.selectedClient}`,
                        {
                            method: 'POST',
                            body: JSON.stringify(payload)
                        }
                    );
                    this.showAlert('success', 'Provider added successfully');
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
                api_keys_text: '',
                priority: provider.priority,
                enabled: !!provider.enabled
            };
            this.editingProviderName = provider.name;
            this.editingProviderKeyCount = Number(provider.key_count || 0);
            this.showEditProviderModal = true;
        },

        async deleteProvider(name) {
            if (!confirm(`Are you sure you want to delete provider "${name}"?`)) {
                return;
            }

            try {
                await this.apiCall(
                    `/api/providers/${this.selectedClient}/${encodeURIComponent(name)}`,
                    { method: 'DELETE' }
                );
                this.showAlert('success', 'Provider deleted successfully');
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
                this.showAlert('success', 'Configuration saved. Some changes may require restart.');
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
                this.showAlert('success', 'Configuration exported successfully');
            } catch (error) {
                this.showAlert('error', 'Failed to export configuration');
                console.error('Failed to export config:', error);
            }
        },

        // UI Helpers
        showAlert(type, message) {
            this.alert = {
                show: true,
                type: type,
                message: message
            };

            // Auto-hide after 5 seconds
            setTimeout(() => {
                this.alert.show = false;
            }, 5000);
        },

        closeModals() {
            this.showAddProviderModal = false;
            this.showEditProviderModal = false;
            this.providerForm = {
                name: '',
                base_url: '',
                api_keys_text: '',
                priority: 1,
                enabled: true
            };
            this.editingProviderName = '';
            this.editingProviderKeyCount = 0;
        }
    };
}
