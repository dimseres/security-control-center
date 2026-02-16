const BackupsPage = (() => {
  const TAB_ROUTE = {
    'backups-tab-overview': '/backups',
    'backups-tab-history': '/backups/history',
    'backups-tab-contents': '/backups/contents',
    'backups-tab-restore': '/backups/restore',
    'backups-tab-plan': '/backups/plan',
  };
  const ROUTE_TAB = {
    '/backups': 'backups-tab-overview',
    '/backups/history': 'backups-tab-history',
    '/backups/contents': 'backups-tab-contents',
    '/backups/restore': 'backups-tab-restore',
    '/backups/plan': 'backups-tab-plan',
  };
  const state = {
    activeTab: 'backups-tab-overview',
    popstateBound: false,
  };

  function init() {
    bindTabs();
    if (typeof BackupsOverview !== 'undefined') BackupsOverview.init();
    if (typeof BackupsHistory !== 'undefined') BackupsHistory.init();
    if (typeof BackupsContents !== 'undefined') BackupsContents.init();
    if (typeof BackupsRestore !== 'undefined') BackupsRestore.init();
    if (typeof BackupsPlan !== 'undefined') BackupsPlan.init();
    syncRouteTab();
    if (!state.popstateBound) {
      state.popstateBound = true;
      window.addEventListener('popstate', () => syncRouteTab());
    }
  }

  function bindTabs() {
    document.querySelectorAll('#backups-tabs .tab-btn').forEach((tab) => {
      tab.addEventListener('click', (e) => {
        e.preventDefault();
        const tabId = tab.dataset.tab;
        if (!tabId) return;
        switchTab(tabId, true);
      });
    });
  }

  function switchTab(tabId, updatePath = false) {
    state.activeTab = tabId;
    document.querySelectorAll('#backups-tabs .tab-btn').forEach((tab) => {
      tab.classList.toggle('active', tab.dataset.tab === tabId);
    });
    document.querySelectorAll('#backups-page .tab-panel').forEach((panel) => {
      panel.hidden = panel.dataset.tab !== tabId;
    });
    if (updatePath) {
      const nextPath = TAB_ROUTE[tabId] || '/backups';
      if (window.location.pathname !== nextPath) {
        window.history.pushState({}, '', nextPath);
      }
    }
    if (tabId === 'backups-tab-overview' && typeof BackupsOverview !== 'undefined') BackupsOverview.load();
    if (tabId === 'backups-tab-history' && typeof BackupsHistory !== 'undefined') BackupsHistory.load();
    if (tabId === 'backups-tab-contents' && typeof BackupsContents !== 'undefined') BackupsContents.load();
    if (tabId === 'backups-tab-restore' && typeof BackupsRestore !== 'undefined') BackupsRestore.load();
    if (tabId === 'backups-tab-plan' && typeof BackupsPlan !== 'undefined') BackupsPlan.load();
  }

  function syncRouteTab() {
    const path = normalizePath(window.location.pathname);
    switchTab(ROUTE_TAB[path] || 'backups-tab-overview', false);
  }

  function normalizePath(path) {
    const raw = (path || '/backups').replace(/\/+$/, '');
    return raw || '/backups';
  }

  function setAlert(kind, key, fallback) {
    const box = document.getElementById('backups-alert');
    if (!box) return;
    if (!key && !fallback) {
      box.hidden = true;
      box.className = 'alert';
      box.textContent = '';
      return;
    }
    box.hidden = false;
    box.className = `alert ${kind || ''}`.trim();
    box.textContent = t(key) || fallback || '';
  }

  function t(key) {
    return BerkutI18n.t(key || '');
  }

  function statusLabel(status) {
    const normalized = (status || '').toString().trim().toLowerCase();
    if (!normalized) return t('backups.status.undefined') || '-';
    const key = `backups.status.${normalized}`;
    const translated = t(key);
    if (translated && translated !== key) return translated;
    return t('backups.status.undefined') || normalized;
  }

  function formatDateTime(value) {
    if (!value) return '-';
    if (typeof AppTime !== 'undefined' && AppTime.formatDateTime) {
      return AppTime.formatDateTime(value);
    }
    return value;
  }

  function formatBytes(value) {
    if (value === null || value === undefined || Number.isNaN(Number(value))) return '-';
    const n = Number(value);
    if (n < 1024) return `${n} B`;
    const kb = n / 1024;
    if (kb < 1024) return `${kb.toFixed(1)} KB`;
    const mb = kb / 1024;
    if (mb < 1024) return `${mb.toFixed(1)} MB`;
    return `${(mb / 1024).toFixed(1)} GB`;
  }

  function parseError(err) {
    const fallback = {
      code: 'common.error',
      i18nKey: 'common.serverError',
      status: 0,
    };
    if (!err) return fallback;
    const msg = (err.message || '').trim();
    if (!msg) return fallback;
    try {
      const payload = JSON.parse(msg);
      const src = payload.error || payload;
      return {
        code: src.code || fallback.code,
        i18nKey: src.i18n_key || fallback.i18nKey,
        status: payload.status || fallback.status,
      };
    } catch (_) {
      if (msg.includes('403')) {
        return { code: 'backups.forbidden', i18nKey: 'backups.error.forbidden', status: 403 };
      }
      return fallback;
    }
  }

  async function apiGet(path) {
    return Api.get(path);
  }

  async function apiPost(path, body) {
    return Api.post(path, body);
  }

  async function apiPut(path, body) {
    return Api.put(path, body);
  }

  async function apiDelete(path, body) {
    return Api.del(path, body);
  }

  async function listBackups() {
    const res = await apiGet('/api/backups');
    return Array.isArray(res.items) ? res.items : [];
  }

  async function createBackup(payload = {}) {
    return apiPost('/api/backups', payload);
  }

  return {
    init,
    switchTab,
    syncRouteTab,
    t,
    setAlert,
    statusLabel,
    formatDateTime,
    formatBytes,
    parseError,
    apiGet,
    apiPost,
    apiPut,
    apiDelete,
    listBackups,
    createBackup,
  };
})();
