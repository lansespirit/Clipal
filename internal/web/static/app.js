function app() {
    return {
        // State
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
            await this.refreshStatus();
            await this.loadProviders();
            await this.loadGlobalConfig();
        },

        // API Calls
        async apiCall(url, options = {}) {
            try {
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

                return await response.json();
            } catch (error) {
                this.showAlert('error', error.message);
                throw error;
            }
        },

        // Status
        async refreshStatus() {
            try {
                this.status = await this.apiCall('/api/status');
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
                    }
                );
                this.showAlert('success', newEnabled ? 'Provider enabled' : 'Provider disabled');
                await this.loadProviders();
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
                this.showAlert('success', 'Configuration saved successfully. Restart may be required for some changes.');
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
