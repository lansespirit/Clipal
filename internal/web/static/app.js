function app() {
    return {
        // State
        isLoading: false,
        theme: localStorage.getItem('theme') || 'system',
        activeTab: 'providers',
        selectedClient: 'claude-code',
        clientOptions: [
            { value: 'claude-code', label: 'Claude Code' },
            { value: 'codex', label: 'Codex' },
            { value: 'gemini', label: 'Gemini' }
        ],
        providers: [],
        globalConfig: {},
        status: {
            version: '',
            uptime: '',
            config_dir: '',
            clients: {}
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
            api_key: '',
            priority: 1,
            enabled: true
        },
        editingProviderName: '',

        // Initialization
        async init() {
            this.initTheme();
            
            // Initial data load
            this.isLoading = true;
            try {
                await Promise.all([
                    this.refreshStatus(),
                    this.loadProviders(),
                    this.loadGlobalConfig()
                ]);
            } finally {
                this.isLoading = false;
            }
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

        // Providers
        clientLabel(clientType) {
            const match = this.clientOptions.find(c => c.value === clientType);
            return match ? match.label : clientType;
        },

        async loadProviders() {
            try {
                this.providers = await this.apiCall(`/api/providers/${this.selectedClient}`);
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
                if (this.showEditProviderModal) {
                    // Update existing provider
                    await this.apiCall(
                        `/api/providers/${this.selectedClient}/${encodeURIComponent(this.editingProviderName)}`,
                        {
                            method: 'PUT',
                            body: JSON.stringify(this.providerForm)
                        }
                    );
                    this.showAlert('success', 'Provider updated successfully');
                } else {
                    // Add new provider
                    await this.apiCall(
                        `/api/providers/${this.selectedClient}`,
                        {
                            method: 'POST',
                            body: JSON.stringify(this.providerForm)
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
                api_key: '', // Don't populate for security
                priority: provider.priority,
                enabled: !!provider.enabled
            };
            this.editingProviderName = provider.name;
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
                api_key: '',
                priority: maxPriority + 1,
                enabled: true
            };
            this.editingProviderName = '';
            this.showAddProviderModal = true;
        },

        // Global Config
        async loadGlobalConfig() {
            try {
                this.globalConfig = await this.apiCall('/api/config/global');
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
                api_key: '',
                priority: 1,
                enabled: true
            };
            this.editingProviderName = '';
        }
    };
}
