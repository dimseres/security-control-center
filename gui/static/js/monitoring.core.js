const MonitoringPage = (() => {
  const state = {
    monitors: [],
    selectedId: null,
    filters: { q: '', status: '', active: '', tags: [] },
    settings: null,
    currentUser: null,
    permissions: [],
    notificationChannels: [],
  };

  function t(key) {
    return (typeof BerkutI18n !== 'undefined' && BerkutI18n.t) ? (BerkutI18n.t(key) || key) : key;
  }

  function hasPermission(perm) {
    if (!perm) return true;
    const perms = Array.isArray(state.permissions) ? state.permissions : [];
    if (!perms.length) return true;
    return perms.includes(perm);
  }

  async function init() {
    const page = document.getElementById('monitoring-page');
    if (!page) return;
    state.currentUser = await loadCurrentUser();
    const deep = resolveMonitorDeepLink();
    if (deep) {
      state.selectedId = deep;
    }
    if (MonitoringPage.bindTabs) MonitoringPage.bindTabs();
    if (MonitoringPage.bindList) MonitoringPage.bindList();
    if (MonitoringPage.bindDetail) MonitoringPage.bindDetail();
    if (MonitoringPage.bindModal) MonitoringPage.bindModal();
    if (MonitoringPage.bindSettings) MonitoringPage.bindSettings();
    if (MonitoringPage.bindCerts) MonitoringPage.bindCerts();
    if (MonitoringPage.bindEventsCenter) MonitoringPage.bindEventsCenter();
    if (MonitoringPage.bindMaintenance) MonitoringPage.bindMaintenance();
    if (MonitoringPage.bindNotifications) MonitoringPage.bindNotifications();
    if (MonitoringPage.bindSLA) MonitoringPage.bindSLA();
    await MonitoringPage.loadMonitors?.();
  }

  function resolveMonitorDeepLink() {
    const url = new URL(window.location.href);
    const raw = url.searchParams.get('monitor') || '';
    const id = parseInt(raw, 10);
    if (Number.isFinite(id) && id > 0) return id;
    return null;
  }

  function setMonitorDeepLink(id) {
    const url = new URL(window.location.href);
    const val = parseInt(String(id || ''), 10);
    if (Number.isFinite(val) && val > 0) {
      url.searchParams.set('monitor', String(val));
    } else {
      url.searchParams.delete('monitor');
    }
    window.history.replaceState({}, '', url.toString());
  }

  async function waitMonitorCheckedAfter(id, sinceISO, timeoutMs = 7000) {
    const monitorID = parseInt(String(id || ''), 10);
    if (!Number.isFinite(monitorID) || monitorID <= 0) return false;
    let sinceTs = 0;
    if (sinceISO) {
      const parsed = Date.parse(String(sinceISO));
      if (Number.isFinite(parsed)) sinceTs = parsed;
    }
    const stepMs = 450;
    const deadline = Date.now() + Math.max(1000, Number(timeoutMs) || 7000);
    while (Date.now() < deadline) {
      try {
        const res = await Api.get(`/api/monitoring/monitors/${monitorID}/state`);
        const item = res?.item || null;
        const checked = item?.last_checked_at ? Date.parse(item.last_checked_at) : 0;
        if (checked && checked > sinceTs) return true;
      } catch (_) {
        return false;
      }
      await new Promise((resolve) => setTimeout(resolve, stepMs));
    }
    return false;
  }

  async function loadCurrentUser() {
    try {
      const res = await Api.get('/api/auth/me');
      const me = res.user;
      state.permissions = Array.isArray(me?.permissions) ? me.permissions : [];
      return me;
    } catch (err) {
      state.permissions = [];
      return null;
    }
  }

  function formatDate(value) {
    if (!value) return '-';
    try {
      if (typeof AppTime !== 'undefined' && AppTime.formatDateTime) {
        return AppTime.formatDateTime(value);
      }
      const dt = new Date(value);
      const pad = (num) => `${num}`.padStart(2, '0');
      return `${pad(dt.getDate())}.${pad(dt.getMonth() + 1)}.${dt.getFullYear()} ${pad(dt.getHours())}:${pad(dt.getMinutes())}`;
    } catch (err) {
      return value;
    }
  }

  function formatDateShort(value) {
    if (!value) return '-';
    try {
      if (typeof AppTime !== 'undefined' && AppTime.formatDate) {
        return AppTime.formatDate(value);
      }
      const dt = new Date(value);
      const pad = (num) => `${num}`.padStart(2, '0');
      return `${pad(dt.getDate())}.${pad(dt.getMonth() + 1)}.${dt.getFullYear()}`;
    } catch (err) {
      return value;
    }
  }

  function formatUptime(val) {
    if (val === null || val === undefined) return '-';
    return `${val.toFixed(2)}%`;
  }

  function formatPercent(val) {
    if (val === null || val === undefined) return '-';
    return `${Number(val).toFixed(1)}%`;
  }

  function formatLatency(val) {
    if (val === null || val === undefined) return '-';
    return `${Math.round(val)} ms`;
  }

  function formatDaysLeft(val) {
    if (val === null || val === undefined) return '-';
    return `${Math.round(val)}`;
  }

  function tagLabel(code) {
    if (typeof TagDirectory !== 'undefined' && TagDirectory.label) {
      const label = TagDirectory.label(code);
      if (label) return label;
    }
    return code;
  }

  function renderTagHint(select, hint) {
    if (!select || !hint) return;
    hint.innerHTML = '';
    const selected = Array.from(select.selectedOptions || []);
    if (!selected.length) return;
    selected.forEach(opt => {
      const tag = document.createElement('span');
      tag.className = 'tag';
      tag.textContent = tagLabel(opt.value || opt.textContent || '');
      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'tag-remove';
      remove.setAttribute('aria-label', t('common.delete'));
      remove.textContent = 'x';
      remove.addEventListener('click', (e) => {
        e.stopPropagation();
        opt.selected = false;
        select.dispatchEvent(new Event('change', { bubbles: true }));
      });
      tag.appendChild(remove);
      hint.appendChild(tag);
    });
  }

  function bindTagHint(select, hint) {
    if (!select || !hint) return;
    const update = () => renderTagHint(select, hint);
    if (select.dataset.tagHintBound === '1') {
      update();
      return;
    }
    select.dataset.tagHintBound = '1';
    select.addEventListener('change', update);
    update();
  }

  function showAlert(el, msg, success = false) {
    if (!el) return;
    el.textContent = msg;
    el.classList.toggle('success', success);
    el.hidden = false;
  }

  function hideAlert(el) {
    if (!el) return;
    el.hidden = true;
    el.classList.remove('success');
  }

  function sanitizeErrorMessage(msg) {
    if (!msg) return t('common.error');
    msg = String(msg).trim();
    if (msg.endsWith(':')) msg = msg.slice(0, -1).trim();
    if (!msg) return t('common.error');
    if (msg.startsWith('status_')) {
      const code = msg.replace('status_', '');
      return `HTTP ${code}`;
    }
    const translated = t(msg);
    return translated === msg ? msg : translated;
  }

  function selectedMonitor() {
    return state.monitors.find(m => m.id === state.selectedId);
  }

  return {
    state,
    t,
    hasPermission,
    init,
    loadCurrentUser,
    setMonitorDeepLink,
    waitMonitorCheckedAfter,
    formatDate,
    formatDateShort,
    formatUptime,
    formatPercent,
    formatLatency,
    formatDaysLeft,
    tagLabel,
    bindTagHint,
    showAlert,
    hideAlert,
    sanitizeErrorMessage,
    selectedMonitor,
    resolveMonitorDeepLink,
  };
})();

if (typeof window !== 'undefined') {
  window.MonitoringPage = MonitoringPage;
}
