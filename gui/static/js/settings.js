const Preferences = (() => {
  const key = 'berkut_prefs';
  const defaults = {
    language: 'ru',
    autoLogout: false,
    incidentAutoSaveEnabled: false,
    incidentAutoSavePeriod: 5,
    timeZone: 'Europe/Moscow',
  };

  function load() {
    try {
      const raw = localStorage.getItem(key);
      return { ...defaults, ...(raw ? JSON.parse(raw) : {}) };
    } catch (err) {
      console.error('prefs load', err);
      return { ...defaults };
    }
  }

  function save(next) {
    const prefs = { ...load(), ...next };
    localStorage.setItem(key, JSON.stringify(prefs));
    if (prefs.language) {
      localStorage.setItem('berkut_lang', prefs.language);
    }
    return prefs;
  }

  return { load, save };
})();

const AppTime = (() => {
  const DEFAULT_TZ = 'Europe/Moscow';
  const PAD = (num) => `${num}`.padStart(2, '0');

  function getTimeZone() {
    if (typeof Preferences !== 'undefined' && Preferences.load) {
      return Preferences.load().timeZone || DEFAULT_TZ;
    }
    return DEFAULT_TZ;
  }

  function currentLang() {
    if (typeof BerkutI18n !== 'undefined' && BerkutI18n.currentLang) {
      return BerkutI18n.currentLang();
    }
    if (typeof Preferences !== 'undefined' && Preferences.load) {
      return Preferences.load().language || 'ru';
    }
    return 'ru';
  }

  function formatDateTime(value) {
    const d = toDate(value);
    if (!d) return '-';
    const parts = formatParts(d, true);
    if (currentLang() === 'en') {
      return `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}`;
    }
    return `${parts.day}.${parts.month}.${parts.year} ${parts.hour}:${parts.minute}`;
  }

  function formatDate(value) {
    const d = toDate(value);
    if (!d) return '-';
    const parts = formatParts(d, false);
    if (currentLang() === 'en') {
      return `${parts.year}-${parts.month}-${parts.day}`;
    }
    return `${parts.day}.${parts.month}.${parts.year}`;
  }

  function formatTime(value) {
    const d = toDate(value);
    if (!d) return '';
    const parts = formatParts(d, true);
    return `${parts.hour}:${parts.minute}`;
  }

  function timeZones() {
    if (typeof Intl !== 'undefined' && Intl.supportedValuesOf) {
      try {
        return Intl.supportedValuesOf('timeZone');
      } catch (_) {
        // ignore
      }
    }
    return [DEFAULT_TZ, 'UTC'];
  }

  function toDate(value) {
    if (!value) return null;
    const d = new Date(value);
    if (Number.isNaN(d.getTime())) return null;
    return d;
  }

  function parseDateInput(value) {
    const raw = (value || '').trim();
    if (!raw) return null;
    let m = /^(\d{2})\.(\d{2})\.(\d{4})$/.exec(raw);
    let day;
    let month;
    let year;
    if (m) {
      day = parseInt(m[1], 10);
      month = parseInt(m[2], 10);
      year = parseInt(m[3], 10);
    } else {
      m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(raw);
      if (!m) return null;
      year = parseInt(m[1], 10);
      month = parseInt(m[2], 10);
      day = parseInt(m[3], 10);
    }
    if ([day, month, year].some(Number.isNaN)) return null;
    if (day < 1 || day > 31 || month < 1 || month > 12) return null;
    const date = new Date(Date.UTC(year, month - 1, day, 0, 0, 0));
    if (Number.isNaN(date.getTime())) return null;
    return { day, month, year, date };
  }

  function parseTimeInput(value) {
    const raw = (value || '').trim();
    if (!raw) return { hour: 0, minute: 0, second: 0 };
    const m = /^(\d{1,2}):(\d{2})(?::(\d{2}))?$/.exec(raw);
    if (!m) return null;
    const hour = parseInt(m[1], 10);
    const minute = parseInt(m[2], 10);
    const second = parseInt(m[3] || '0', 10);
    if ([hour, minute, second].some(Number.isNaN)) return null;
    if (hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 59) return null;
    return { hour, minute, second };
  }

  function parseDateTimeInput(value) {
    const raw = (value || '').trim();
    if (!raw) return null;
    let datePart = raw;
    let timePart = '';
    if (raw.includes('T')) {
      [datePart, timePart] = raw.split('T');
    } else if (raw.includes(' ')) {
      [datePart, timePart] = raw.split(' ');
    }
    const parsedDate = parseDateInput(datePart);
    if (!parsedDate) return null;
    const parsedTime = timePart ? parseTimeInput(timePart) : { hour: 0, minute: 0, second: 0 };
    if (timePart && !parsedTime) return null;
    const dt = new Date(parsedDate.year, parsedDate.month - 1, parsedDate.day, parsedTime.hour, parsedTime.minute, parsedTime.second);
    if (Number.isNaN(dt.getTime())) return null;
    return dt;
  }

  function toISODate(value) {
    const parsed = parseDateInput(value);
    if (!parsed) return '';
    return `${parsed.year}-${PAD(parsed.month)}-${PAD(parsed.day)}`;
  }

  function toISODateTime(dateValue, timeValue) {
    let datePart = (dateValue || '').trim();
    let timePart = (timeValue || '').trim();
    if (!timeValue) {
      if (datePart.includes('T')) {
        [datePart, timePart] = datePart.split('T');
      } else if (datePart.includes(' ')) {
        [datePart, timePart] = datePart.split(' ');
      }
    }
    const parsedDate = parseDateInput(datePart);
    if (!parsedDate) return '';
    const parsedTime = timePart ? parseTimeInput(timePart) : { hour: 0, minute: 0, second: 0 };
    if (timePart && !parsedTime) return '';
    const dt = new Date(parsedDate.year, parsedDate.month - 1, parsedDate.day, parsedTime.hour, parsedTime.minute, parsedTime.second);
    if (Number.isNaN(dt.getTime())) return '';
    return dt.toISOString();
  }

  function splitDateTime(value) {
    const d = toDate(value);
    if (!d) return { date: '', time: '' };
    const parts = formatParts(d, true);
    return {
      date: currentLang() === 'en' ? `${parts.year}-${parts.month}-${parts.day}` : `${parts.day}.${parts.month}.${parts.year}`,
      time: `${parts.hour}:${parts.minute}`
    };
  }

  function formatParts(date, withTime) {
    const tz = getTimeZone();
    const locale = currentLang() === 'en' ? 'en-US' : 'ru-RU';
    const options = withTime
      ? { timeZone: tz, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }
      : { timeZone: tz, year: 'numeric', month: '2-digit', day: '2-digit' };
    const fmt = new Intl.DateTimeFormat(locale, options);
    const parts = fmt.formatToParts(date);
    const out = {};
    parts.forEach(p => {
      if (p.type !== 'literal') out[p.type] = p.value;
    });
    return {
      day: out.day || '00',
      month: out.month || '00',
      year: out.year || '0000',
      hour: out.hour || '00',
      minute: out.minute || '00',
    };
  }

  return {
    formatDateTime,
    formatDate,
    formatTime,
    getTimeZone,
    timeZones,
    parseDateInput,
    parseTimeInput,
    parseDateTimeInput,
    toISODate,
    toISODateTime,
    splitDateTime
  };
})();

if (typeof window !== 'undefined') {
  window.AppTime = AppTime;
}

const SettingsPage = (() => {
  let activeTab = 'settings-general';
  let currentUser = null;
  let fullAccess = false;
  let permissions = [];
  const TAB_PERMISSIONS = {
    'settings-general': 'settings.general',
    'settings-advanced': 'settings.advanced',
    'settings-cleanup': 'settings.advanced',
    'settings-https': 'settings.advanced',
    'settings-hardening': 'settings.advanced',
    'settings-tags': 'settings.tags',
    'settings-classifications': 'settings.tags',
    'settings-incidents': 'settings.incident_options',
    'settings-controls': 'settings.controls',
    'settings-sources': 'settings.detection_sources',
  };
  const CLEANUP_TARGETS = {
    monitoring: { keys: [], prefixes: ['monitoring.'], remoteCleanup: cleanupMonitoringRemote },
    controls: {
      keys: ['controls.customOptions'],
      prefixes: [],
      remoteCleanup: cleanupControlsRemote,
      onAfter: () => {
        if (window.ControlsPage?.saveCustomOptions) {
          window.ControlsPage.saveCustomOptions({ domains: [] });
        }
      },
    },
    tasks: { keys: [], prefixes: ['tasks.'], remoteCleanup: cleanupTasksRemote },
    incidents: {
      keys: ['incidents.customOptions'],
      prefixes: [],
      remoteCleanup: cleanupIncidentsRemote,
      onAfter: () => {
        if (window.IncidentsPage?.saveCustomOptions) {
          window.IncidentsPage.saveCustomOptions({ incidentTypes: [], detectionSources: [] });
        }
      },
    },
    reports: { keys: [], prefixes: ['reports.'], remoteCleanup: cleanupReportsRemote },
    docs: {
      keys: ['berkut.tags'],
      prefixes: [],
      remoteCleanup: cleanupDocsRemote,
    },
  };

  function init(onChange) {
    const prefs = Preferences.load();
    const alertBox = document.getElementById('settings-alert');
    const langSelect = document.getElementById('settings-language');
    const autoLogoutToggle = document.getElementById('settings-auto-logout');
    const autoSaveToggle = document.getElementById('settings-autosave-enabled');
    const autoSavePeriod = document.getElementById('settings-autosave-period');
    const timeZoneSelect = document.getElementById('settings-timezone');

    if (langSelect) langSelect.value = prefs.language || 'ru';
    if (autoLogoutToggle) autoLogoutToggle.checked = !!prefs.autoLogout;
    if (autoSaveToggle) autoSaveToggle.checked = !!prefs.incidentAutoSaveEnabled;
    if (autoSavePeriod) {
      const storedPeriod = parseInt(prefs.incidentAutoSavePeriod, 10);
      autoSavePeriod.value = Number.isFinite(storedPeriod) && storedPeriod > 0 ? `${storedPeriod}` : '5';
    }
    if (timeZoneSelect && typeof AppTime !== 'undefined' && AppTime.timeZones) {
      const zones = AppTime.timeZones();
      timeZoneSelect.innerHTML = '';
      zones.forEach(zone => {
        const opt = document.createElement('option');
        opt.value = zone;
        opt.textContent = zone;
        timeZoneSelect.appendChild(opt);
      });
      timeZoneSelect.value = prefs.timeZone || AppTime.getTimeZone();
    }

    bindTabs();
    bindPasswordChange(alertBox);
    bindPreferences(alertBox, onChange);
    bindTimeZoneSettings(alertBox, onChange);
    bindRuntimeSettings(alertBox);
    bindHTTPSSettings(alertBox);
    bindHardeningSettings(alertBox);
    bindApprovalsCleanup(alertBox);
    bindMonitoringCleanup(alertBox);
    bindTabsCleanup(alertBox);
    bindTagSettings();
    bindClassificationSettings();
    bindIncidentSettings();
    bindControlsSettings(alertBox);
    (async () => {
      const ctx = await loadCurrentUser();
      applyAccessControls(ctx, alertBox);
      renderPasswordMeta(ctx);
      const target = tabFromPath();
      const initialTab = (target && canViewTab(target)) ? target : firstAllowedTab() || activeTab;
      switchTab(initialTab);
    })();
  }

  function bindTabs() {
    const tabs = document.querySelectorAll('#settings-tabs .tab-btn');
    tabs.forEach(btn => {
      btn.addEventListener('click', () => {
        if (btn.disabled || btn.hidden) return;
        const target = btn.dataset.tab;
        if (target && canViewTab(target)) switchTab(target);
      });
    });
  }

  function switchTab(tabId) {
    if (!canViewTab(tabId)) {
      const fallback = firstAllowedTab();
      if (!fallback) return;
      tabId = fallback;
    }
    activeTab = tabId || activeTab;
    document.querySelectorAll('#settings-tabs .tab-btn').forEach(btn => {
      btn.classList.toggle('active', btn.dataset.tab === activeTab);
    });
    document.querySelectorAll('.settings-panel').forEach(panel => {
      panel.hidden = panel.dataset.tab !== activeTab;
    });
    updateTabHash(activeTab);
  }

  function bindPasswordChange(alertBox) {
    const pwdForm = document.getElementById('password-change-form');
    if (!pwdForm) return;
    pwdForm.onsubmit = async (e) => {
      e.preventDefault();
      if (alertBox) {
        alertBox.hidden = true;
        alertBox.classList.remove('success');
      }
      const data = Object.fromEntries(new FormData(pwdForm).entries());
      if ((data.password || '') !== (data.password_confirm || '')) {
        if (alertBox) {
          alertBox.textContent = BerkutI18n.t('accounts.passwordMismatch');
          alertBox.hidden = false;
        }
        return;
      }
      try {
        await Api.post('/api/auth/change-password', {
          current_password: data.current_password,
          password: data.password
        });
        if (alertBox) {
          alertBox.textContent = BerkutI18n.t('accounts.passwordChangeDone') || BerkutI18n.t('common.saved');
          alertBox.classList.add('success');
          alertBox.hidden = false;
        }
      } catch (err) {
        if (alertBox) {
          alertBox.textContent = err.message || BerkutI18n.t('common.error');
          alertBox.hidden = false;
        }
      }
    };
  }

  function bindPreferences(alertBox, onChange) {
    const form = document.getElementById('settings-form');
    const langSelect = document.getElementById('settings-language');
    const autoLogoutToggle = document.getElementById('settings-auto-logout');
    const autoSaveToggle = document.getElementById('settings-autosave-enabled');
    const autoSavePeriod = document.getElementById('settings-autosave-period');
    if (!form || !langSelect || !autoLogoutToggle) return;
    const syncAutoSave = () => {
      if (autoSavePeriod && autoSaveToggle) {
        autoSavePeriod.disabled = !autoSaveToggle.checked;
      }
    };
    syncAutoSave();
    if (autoSaveToggle) {
      autoSaveToggle.addEventListener('change', syncAutoSave);
    }
    form.onsubmit = async (e) => {
      e.preventDefault();
      if (alertBox) {
        alertBox.hidden = true;
        alertBox.classList.remove('success');
      }
      const nextPrefs = Preferences.save({
        language: langSelect.value,
        autoLogout: autoLogoutToggle.checked,
        incidentAutoSaveEnabled: autoSaveToggle ? autoSaveToggle.checked : false,
        incidentAutoSavePeriod: autoSavePeriod ? parseInt(autoSavePeriod.value, 10) || 0 : 0
      });
      if (onChange) {
        await onChange(nextPrefs);
      }
      if (alertBox) {
        alertBox.textContent = BerkutI18n.t('settings.saved');
        alertBox.classList.add('success');
        alertBox.hidden = false;
      }
    };
  }

  function bindTimeZoneSettings(alertBox, onChange) {
    const form = document.getElementById('settings-timezone-form');
    const timeZoneSelect = document.getElementById('settings-timezone');
    if (!form || !timeZoneSelect) return;
    form.onsubmit = async (e) => {
      e.preventDefault();
      if (alertBox) {
        alertBox.hidden = true;
        alertBox.classList.remove('success');
      }
      const nextPrefs = Preferences.save({
        timeZone: timeZoneSelect.value || (AppTime?.getTimeZone ? AppTime.getTimeZone() : 'Europe/Moscow')
      });
      if (onChange) {
        await onChange(nextPrefs);
      }
      if (alertBox) {
        alertBox.textContent = BerkutI18n.t('settings.saved');
        alertBox.classList.add('success');
        alertBox.hidden = false;
      }
    };
  }

  function bindRuntimeSettings(alertBox) {
    const modeEl = document.getElementById('settings-deployment-mode');
    const updatesEl = document.getElementById('settings-updates-enabled');
    const saveBtn = document.getElementById('settings-runtime-save');
    const checkBtn = document.getElementById('settings-updates-check');
    const statusEl = document.getElementById('settings-updates-status');
    const homeWarning = document.getElementById('settings-home-warning');
    const httpsHomeWarning = document.getElementById('settings-https-home-warning');
    const httpsModeEl = document.getElementById('settings-https-mode');
    const aboutVersion = document.getElementById('settings-app-version');
    if (!modeEl || !updatesEl || !saveBtn) return;

    const applyMode = () => {
      const home = (modeEl.value || '') === 'home';
      if (homeWarning) homeWarning.hidden = !home;
      if (httpsHomeWarning) httpsHomeWarning.hidden = !home;
      if (httpsModeEl) {
        const tlsOption = httpsModeEl.querySelector('option[value="builtin_tls"]');
        if (tlsOption) tlsOption.disabled = home;
        if (home && httpsModeEl.value === 'builtin_tls') {
          httpsModeEl.value = 'disabled';
        }
      }
    };

    const formatUpdateStatus = (payload) => {
      const result = payload?.update || payload?.result || payload;
      const checkedAtRaw = result?.checked_at || result?.checkedAt;
      if (!result || !checkedAtRaw) {
        return BerkutI18n.t('settings.updates.notChecked');
      }
      const latest = result.latest_version || result.latestVersion || '-';
      const checkedAt = AppTime?.formatDateTime ? AppTime.formatDateTime(checkedAtRaw) : checkedAtRaw;
      if (result.has_update || result.hasUpdate) {
        return `${BerkutI18n.t('settings.updates.available')}: ${latest} (${checkedAt})`;
      }
      return `${BerkutI18n.t('settings.updates.upToDate')}: ${latest} (${checkedAt})`;
    };

    const loadRuntime = async () => {
      try {
        const data = await Api.get('/api/settings/runtime');
        modeEl.value = data?.deployment_mode || 'enterprise';
        updatesEl.checked = !!data?.update_checks_enabled;
        if (statusEl) statusEl.textContent = formatUpdateStatus(data || {});
        if (aboutVersion) {
          aboutVersion.textContent = `${BerkutI18n.t('settings.version')}: ${data?.app_version || '-'}`;
        }
        applyMode();
      } catch (err) {
        if ((err.message || '').trim() === 'forbidden') {
          if (aboutVersion) {
            try {
              const meta = await Api.get('/api/app/meta');
              aboutVersion.textContent = `${BerkutI18n.t('settings.version')}: ${meta?.app_version || '-'}`;
            } catch (_) {}
          }
          return;
        }
        showSettingsAlert(alertBox, err.message || BerkutI18n.t('common.error'));
      }
    };

    modeEl.addEventListener('change', applyMode);
    saveBtn.addEventListener('click', async () => {
      try {
        const payload = {
          deployment_mode: modeEl.value || 'enterprise',
          update_checks_enabled: !!updatesEl.checked,
        };
        const data = await Api.put('/api/settings/runtime', payload);
        modeEl.value = data?.deployment_mode || payload.deployment_mode;
        updatesEl.checked = !!data?.update_checks_enabled;
        if (statusEl) statusEl.textContent = formatUpdateStatus(data || {});
        applyMode();
        showSettingsAlert(alertBox, BerkutI18n.t('settings.saved'), true);
      } catch (err) {
        showSettingsAlert(alertBox, err.message || BerkutI18n.t('common.error'));
      }
    });

    if (checkBtn) {
      checkBtn.addEventListener('click', async () => {
        try {
          if (updatesEl.checked) {
            await Api.put('/api/settings/runtime', {
              deployment_mode: modeEl.value || 'enterprise',
              update_checks_enabled: true,
            });
          }
          const data = await Api.post('/api/settings/updates/check', {});
          if (statusEl) statusEl.textContent = formatUpdateStatus(data || {});
          showSettingsAlert(alertBox, BerkutI18n.t('settings.updates.checked'), true);
        } catch (err) {
          showSettingsAlert(alertBox, err.message || BerkutI18n.t('common.error'));
        }
      });
    }
    loadRuntime();
  }

  function bindHTTPSSettings(alertBox) {
    const form = document.getElementById('settings-https-form');
    const modeEl = document.getElementById('settings-https-mode');
    const portEl = document.getElementById('settings-https-port');
    const proxyHintEl = document.getElementById('settings-https-proxy-hint');
    const trustedEl = document.getElementById('settings-https-trusted-proxies');
    const certEl = document.getElementById('settings-https-cert');
    const keyEl = document.getElementById('settings-https-key');
    const certField = document.getElementById('settings-https-cert-field');
    const keyField = document.getElementById('settings-https-key-field');
    const saveBtn = document.getElementById('settings-https-save');
    const card = document.getElementById('settings-https-card');
    if (!form || !modeEl || !portEl || !trustedEl || !saveBtn || !card) return;

    const toggleModeFields = () => {
      const mode = (modeEl.value || '').trim();
      const builtIn = mode === 'builtin_tls';
      const proxyMode = mode === 'external_proxy';
      if (certField) certField.hidden = !builtIn;
      if (keyField) keyField.hidden = !builtIn;
      const proxyField = proxyHintEl ? proxyHintEl.closest('.form-field') : null;
      if (proxyField) proxyField.hidden = !proxyMode;
    };

    const parseTrustedProxies = (raw) => {
      const parts = String(raw || '')
        .split(/\r?\n|,/)
        .map((item) => item.trim())
        .filter(Boolean);
      return Array.from(new Set(parts));
    };

    const fillForm = (data) => {
      modeEl.value = data.mode || 'disabled';
      portEl.value = Number.isFinite(Number(data.listen_port)) ? `${data.listen_port}` : '8080';
      trustedEl.value = Array.isArray(data.trusted_proxies) ? data.trusted_proxies.join('\n') : '';
      if (certEl) certEl.value = data.builtin_cert_path || '';
      if (keyEl) keyEl.value = data.builtin_key_path || '';
      if (proxyHintEl) proxyHintEl.value = data.external_proxy_hint || 'nginx';
      toggleModeFields();
    };

    const loadHTTPS = async () => {
      try {
        const data = await Api.get('/api/settings/https');
        fillForm(data || {});
      } catch (err) {
        showSettingsAlert(alertBox, err.message || BerkutI18n.t('common.error'));
      }
    };

    modeEl.addEventListener('change', toggleModeFields);
    saveBtn.addEventListener('click', async () => {
      const payload = {
        mode: (modeEl.value || 'disabled').trim(),
        listen_port: parseInt(portEl.value, 10) || 0,
        trusted_proxies: parseTrustedProxies(trustedEl.value),
        external_proxy_hint: (proxyHintEl?.value || '').trim(),
        builtin_cert_path: (certEl?.value || '').trim(),
        builtin_key_path: (keyEl?.value || '').trim(),
      };
      try {
        const data = await Api.put('/api/settings/https', payload);
        fillForm(data || payload);
        showSettingsAlert(alertBox, BerkutI18n.t('settings.saved'), true);
      } catch (err) {
        showSettingsAlert(alertBox, err.message || BerkutI18n.t('common.error'));
      }
    });

    loadHTTPS();
  }

  function bindApprovalsCleanup(alertBox) {
    const cleanupSection = document.getElementById('approvals-cleanup-section');
    const cleanupBtn = document.getElementById('approvals-cleanup-btn');
    const cleanupScope = document.getElementById('approvals-cleanup-scope');
    if (!cleanupSection || !cleanupBtn || !cleanupScope) return;
    cleanupBtn.onclick = async () => {
      if (alertBox) {
        alertBox.hidden = true;
        alertBox.classList.remove('success');
      }
      const scope = cleanupScope.value;
      const includeActive = scope === 'all';
      const confirmed = await confirmSettingsCleanup(BerkutI18n.t('settings.approvalsCleanupConfirm'));
      if (!confirmed) return;
      try {
        await Api.post('/api/approvals/cleanup', includeActive ? { all: true } : {});
        if (alertBox) {
          alertBox.textContent = BerkutI18n.t('settings.approvalsCleanupDone');
          alertBox.classList.add('success');
          alertBox.hidden = false;
        }
      } catch (err) {
        if (alertBox) {
          alertBox.textContent = err.message || 'error';
          alertBox.hidden = false;
        }
      }
    };
  }

  function bindHardeningSettings(alertBox) {
    const refreshBtn = document.getElementById('settings-hardening-refresh');
    const scoreEl = document.getElementById('settings-hardening-score');
    const statusEl = document.getElementById('settings-hardening-status');
    const tbody = document.querySelector('#settings-hardening-table tbody');
    if (!refreshBtn || !scoreEl || !statusEl || !tbody) return;

    const statusLabel = (status) => {
      const key = `settings.hardening.status.${status || 'warning'}`;
      const text = BerkutI18n.t(key);
      return text && text !== key ? text : (status || '-');
    };

    const render = (data) => {
      const score = Number(data?.score || 0);
      const max = Number(data?.max_score || 0);
      scoreEl.textContent = `${score}/${max}`;
      statusEl.textContent = statusLabel(data?.status);
      tbody.innerHTML = '';
      const checks = Array.isArray(data?.checks) ? data.checks : [];
      if (!checks.length) {
        const row = document.createElement('tr');
        row.innerHTML = `<td colspan="4">${BerkutI18n.t('settings.options.empty')}</td>`;
        tbody.appendChild(row);
        return;
      }
      checks.forEach((item) => {
        const row = document.createElement('tr');
        const title = BerkutI18n.t(item.title_i18n_key || '') || item.id || '-';
        const msg = BerkutI18n.t(item.message_i18n_key || '') || '-';
        row.innerHTML = `
          <td>${title}</td>
          <td>${statusLabel(item.status)}</td>
          <td>${item.score || 0}/${item.max_score || 0}</td>
          <td>${msg}</td>
        `;
        tbody.appendChild(row);
      });
    };

    const load = async () => {
      try {
        const data = await Api.get('/api/settings/hardening');
        render(data || {});
      } catch (err) {
        showSettingsAlert(alertBox, err.message || BerkutI18n.t('common.error'));
      }
    };

    refreshBtn.addEventListener('click', load);
    load();
  }

  function bindMonitoringCleanup(alertBox) {
    const section = document.getElementById('monitoring-cleanup-section');
    const cleanupBtn = document.getElementById('monitoring-cleanup-btn');
    const scopeEl = document.getElementById('monitoring-cleanup-scope');
    const periodRow = document.getElementById('monitoring-cleanup-period-row');
    const fromEl = document.getElementById('monitoring-cleanup-from');
    const toEl = document.getElementById('monitoring-cleanup-to');
    if (!section || !cleanupBtn || !scopeEl) return;

    const togglePeriod = () => {
      if (periodRow) periodRow.hidden = scopeEl.value !== 'period';
    };
    scopeEl.addEventListener('change', togglePeriod);
    togglePeriod();

    cleanupBtn.onclick = async () => {
      if (alertBox) {
        alertBox.hidden = true;
        alertBox.classList.remove('success');
      }
      const scope = (scopeEl.value || '').trim() || 'open';
      const confirmed = await confirmSettingsCleanup(BerkutI18n.t('settings.monitoringCleanupConfirm'));
      if (!confirmed) return;
      const payload = {
        source: 'monitoring',
        scope,
      };
      if (scope === 'period') {
        payload.from = (fromEl?.value || '').trim();
        payload.to = (toEl?.value || '').trim();
      }
      try {
        const res = await Api.post('/api/incidents/cleanup', payload);
        const removed = Number(res?.removed || 0);
        if (alertBox) {
          alertBox.textContent = BerkutI18n.t('settings.monitoringCleanupDone').replace('{count}', `${removed}`);
          alertBox.classList.add('success');
          alertBox.hidden = false;
        }
      } catch (err) {
        if (alertBox) {
          const raw = (err?.message || '').trim();
          const translated = raw ? BerkutI18n.t(raw) : '';
          alertBox.textContent = translated && translated !== raw ? translated : (raw || BerkutI18n.t('common.error'));
          alertBox.classList.remove('success');
          alertBox.hidden = false;
        }
      }
    };
  }

  function bindTabsCleanup(alertBox) {
    const cleanupSection = document.getElementById('tabs-cleanup-section');
    const cleanupBtn = document.getElementById('tabs-cleanup-btn');
    const selectAllBtn = document.getElementById('tabs-cleanup-select-all');
    const clearAllBtn = document.getElementById('tabs-cleanup-clear-all');
    const optionsRoot = document.getElementById('tabs-cleanup-options');
    if (!cleanupSection || !cleanupBtn || !optionsRoot) return;

    const checkboxes = () => Array.from(optionsRoot.querySelectorAll('input[type="checkbox"]'));
    const selectedTargets = () => checkboxes().filter((cb) => cb.checked).map((cb) => cb.value);

    if (selectAllBtn) {
      selectAllBtn.onclick = () => {
        checkboxes().forEach((cb) => { cb.checked = true; });
      };
    }
    if (clearAllBtn) {
      clearAllBtn.onclick = () => {
        checkboxes().forEach((cb) => { cb.checked = false; });
      };
    }

    cleanupBtn.onclick = async () => {
      if (alertBox) {
        alertBox.hidden = true;
        alertBox.classList.remove('success');
      }
      const targets = selectedTargets();
      if (!targets.length) {
        if (alertBox) {
          alertBox.textContent = BerkutI18n.t('settings.tabsCleanupPickAtLeastOne');
          alertBox.hidden = false;
        }
        return;
      }
      const confirmed = await confirmSettingsCleanup(BerkutI18n.t('settings.tabsCleanupConfirm'));
      if (!confirmed) return;
      try {
        let removed = 0;
        for (const target of targets) {
          removed += await cleanupTabStorage(target);
        }
        if (alertBox) {
          alertBox.textContent = BerkutI18n.t('settings.tabsCleanupDone').replace('{count}', `${removed}`);
          alertBox.classList.add('success');
          alertBox.hidden = false;
        }
      } catch (err) {
        if (alertBox) {
          const raw = (err?.message || '').trim();
          const translated = raw ? BerkutI18n.t(raw) : '';
          alertBox.textContent = translated && translated !== raw ? translated : (raw || BerkutI18n.t('common.error'));
          alertBox.classList.remove('success');
          alertBox.hidden = false;
        }
      }
    };
  }

  async function cleanupTabStorage(tab) {
    const cfg = CLEANUP_TARGETS[tab] || { keys: [], prefixes: [] };
    const remove = new Set(cfg.keys || []);
    const prefixes = cfg.prefixes || [];
    const keysToRemove = [];
    for (let i = 0; i < localStorage.length; i += 1) {
      const key = localStorage.key(i);
      if (!key) continue;
      if (remove.has(key)) {
        keysToRemove.push(key);
        continue;
      }
      if (prefixes.some(prefix => key.startsWith(prefix))) {
        keysToRemove.push(key);
      }
    }
    keysToRemove.forEach((key) => localStorage.removeItem(key));
    let remoteRemoved = 0;
    if (typeof cfg.remoteCleanup === 'function') {
      remoteRemoved = await cfg.remoteCleanup();
    }
    if (typeof cfg.onAfter === 'function') {
      cfg.onAfter();
    }
    return keysToRemove.length + remoteRemoved;
  }

  async function cleanupDocsRemote() {
    let removed = 0;
    for (let i = 0; i < 100; i += 1) {
      const list = extractListItems(await Api.get('/api/docs?limit=200&offset=0'));
      if (!list.length) break;
      let removedThisPass = 0;
      for (const doc of list) {
        if (!doc?.id) continue;
        try {
          await Api.del(`/api/docs/${doc.id}`);
          removed += 1;
          removedThisPass += 1;
        } catch (err) {
          const msg = (err?.message || '').trim();
          if (msg === 'not found' || msg === 'forbidden') continue;
          throw err;
        }
      }
      if (removedThisPass === 0) break;
    }
    return removed;
  }

  async function cleanupIncidentsRemote() {
    const res = await Api.post('/api/incidents/cleanup', { scope: 'all' });
    return Number(res?.removed || 0);
  }

  async function cleanupMonitoringRemote() {
    let removed = 0;
    removed += await cleanupCollection('/api/monitoring/maintenance', (item) => `/api/monitoring/maintenance/${item.id}`);
    removed += await cleanupCollection('/api/monitoring/notifications', (item) => `/api/monitoring/notifications/${item.id}`);
    removed += await cleanupCollection('/api/monitoring/monitors', (item) => `/api/monitoring/monitors/${item.id}`);
    return removed;
  }

  async function cleanupReportsRemote() {
    let removed = 0;
    removed += await cleanupCollection('/api/reports/templates', (item) => `/api/reports/templates/${item.id}`);
    removed += await cleanupCollection('/api/reports', (item) => `/api/reports/${item.id}`);
    return removed;
  }

  async function cleanupTasksRemote() {
    let removed = 0;
    removed += await cleanupCollection('/api/tasks?include_archived=1&limit=500', (item) => `/api/tasks/${item.id}`);
    removed += await cleanupCollection('/api/tasks/templates?include_inactive=1', (item) => `/api/tasks/templates/${item.id}`);
    removed += await cleanupCollection('/api/tasks/boards?include_inactive=1', (item) => `/api/tasks/boards/${item.id}`);
    removed += await cleanupCollection('/api/tasks/spaces?include_inactive=1', (item) => `/api/tasks/spaces/${item.id}`);
    return removed;
  }

  async function cleanupControlsRemote() {
    let removed = 0;
    removed += await cleanupCollection('/api/checks', (item) => `/api/checks/${item.id}`);
    removed += await cleanupCollection('/api/violations', (item) => `/api/violations/${item.id}`);
    removed += await cleanupCollection('/api/controls', (item) => `/api/controls/${item.id}`);
    removed += await cleanupCollection('/api/controls/types', (item) => `/api/controls/types/${item.id}`, {
      itemFilter: (item) => !item?.is_builtin,
    });
    return removed;
  }

  async function cleanupCollection(listUrl, deleteUrlByItem, options = {}) {
    let removed = 0;
    for (let i = 0; i < 100; i += 1) {
      let list = extractListItems(await Api.get(listUrl));
      if (typeof options.itemFilter === 'function') {
        list = list.filter((item) => options.itemFilter(item));
      }
      if (!list.length) break;
      let removedThisPass = 0;
      for (const item of list) {
        const id = item?.id;
        if (!id) continue;
        try {
          await Api.del(deleteUrlByItem(item));
          removed += 1;
          removedThisPass += 1;
        } catch (err) {
          if (isCleanupIgnorableError(err)) continue;
          throw err;
        }
      }
      if (removedThisPass === 0) break;
    }
    return removed;
  }

  function isCleanupIgnorableError(err) {
    const msg = (err?.message || '').trim().toLowerCase();
    return (
      msg === 'forbidden' ||
      msg === 'not found' ||
      msg === 'common.notfound' ||
      msg === 'controls.types.builtin' ||
      msg === 'controls.types.inuse' ||
      msg === 'monitoring.error.passivemonitor' ||
      msg === 'monitoring.error.busy'
    );
  }

  function extractListItems(payload) {
    if (Array.isArray(payload)) return payload;
    if (payload && Array.isArray(payload.items)) return payload.items;
    return [];
  }

  function confirmSettingsCleanup(message) {
    const modal = document.getElementById('settings-cleanup-confirm-modal');
    if (!modal) {
      return Promise.resolve(window.confirm(message || BerkutI18n.t('common.confirm')));
    }
    const titleEl = document.getElementById('settings-cleanup-confirm-title');
    const msgEl = document.getElementById('settings-cleanup-confirm-message');
    const yesBtn = document.getElementById('settings-cleanup-confirm-yes');
    const noBtn = document.getElementById('settings-cleanup-confirm-no');
    const closeBtn = document.getElementById('settings-cleanup-confirm-close');
    if (titleEl) titleEl.textContent = BerkutI18n.t('common.confirm');
    if (msgEl) msgEl.textContent = message || '';
    modal.hidden = false;
    return new Promise((resolve) => {
      const cleanup = (result) => {
        modal.hidden = true;
        if (yesBtn) yesBtn.onclick = null;
        if (noBtn) noBtn.onclick = null;
        if (closeBtn) closeBtn.onclick = null;
        resolve(result);
      };
      if (yesBtn) yesBtn.onclick = () => cleanup(true);
      if (noBtn) noBtn.onclick = () => cleanup(false);
      if (closeBtn) closeBtn.onclick = () => cleanup(false);
      if (yesBtn) yesBtn.focus();
    });
  }

  function bindTagSettings() {
    if (typeof TagDirectory === 'undefined') return;
    const input = document.getElementById('settings-tag-input');
    const addBtn = document.getElementById('settings-tag-add');
    const list = document.getElementById('settings-tags-list');
    if (!input || !addBtn || !list) return;

    const render = () => {
      list.innerHTML = '';
      const tags = TagDirectory.all();
      if (!tags.length) {
        const empty = document.createElement('div');
        empty.className = 'muted';
        empty.textContent = BerkutI18n.t('settings.tags.empty');
        list.appendChild(empty);
        return;
      }
      tags.forEach(tag => {
        const badge = tag.builtIn ? BerkutI18n.t('settings.tags.standard') : BerkutI18n.t('settings.tags.custom');
        list.appendChild(buildPill(TagDirectory.label(tag.code), !tag.builtIn, badge, () => {
          TagDirectory.remove(tag.code);
        }));
      });
    };

    const add = () => {
      const val = (input.value || '').trim();
      if (!val) return;
      TagDirectory.add(val);
      input.value = '';
    };

    addBtn.addEventListener('click', (e) => {
      e.preventDefault();
      add();
    });
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        add();
      }
    });
    document.addEventListener('tags:changed', render);
    render();
  }

  function bindClassificationSettings() {
    if (typeof ClassificationDirectory === 'undefined') return;
    const input = document.getElementById('settings-classification-input');
    const addBtn = document.getElementById('settings-classification-add');
    const list = document.getElementById('settings-classifications-list');
    if (!input || !addBtn || !list) return;

    const render = () => {
      list.innerHTML = '';
      const levels = ClassificationDirectory.all();
      if (!levels.length) {
        const empty = document.createElement('div');
        empty.className = 'muted';
        empty.textContent = BerkutI18n.t('settings.classifications.empty');
        list.appendChild(empty);
        return;
      }
      levels.forEach((level, idx) => {
        const row = document.createElement('div');
        row.className = 'classification-row';

        const main = document.createElement('div');
        main.className = 'classification-main';

        const rank = document.createElement('span');
        rank.className = 'classification-rank';
        rank.textContent = `${idx + 1}`;

        const label = document.createElement('span');
        label.className = 'classification-label';
        label.textContent = level.label || level.code;

        main.appendChild(rank);
        main.appendChild(label);
        row.appendChild(main);

        const actions = document.createElement('div');
        actions.className = 'classification-actions';
        if (level.builtIn) {
          const badge = document.createElement('span');
          badge.className = 'pill pill-muted';
          badge.textContent = BerkutI18n.t('settings.tags.standard');
          actions.appendChild(badge);
        } else {
          const upBtn = document.createElement('button');
          upBtn.type = 'button';
          upBtn.className = 'btn ghost btn-sm';
          upBtn.textContent = BerkutI18n.t('settings.classifications.moveUp');
          upBtn.disabled = !levels[idx - 1];
          upBtn.addEventListener('click', () => ClassificationDirectory.move(level.code, 'up'));

          const downBtn = document.createElement('button');
          downBtn.type = 'button';
          downBtn.className = 'btn ghost btn-sm';
          downBtn.textContent = BerkutI18n.t('settings.classifications.moveDown');
          downBtn.disabled = !levels[idx + 1];
          downBtn.addEventListener('click', () => ClassificationDirectory.move(level.code, 'down'));

          const delBtn = document.createElement('button');
          delBtn.type = 'button';
          delBtn.className = 'btn danger btn-sm';
          delBtn.textContent = BerkutI18n.t('common.delete');
          delBtn.addEventListener('click', () => ClassificationDirectory.remove(level.code));

          actions.appendChild(upBtn);
          actions.appendChild(downBtn);
          actions.appendChild(delBtn);
        }
        row.appendChild(actions);
        list.appendChild(row);
      });
    };

    const add = () => {
      const val = (input.value || '').trim();
      if (!val) return;
      const result = ClassificationDirectory.add(val);
      if (!result.ok) {
        if (result.reason === 'limit') {
          showSettingsAlert(document.getElementById('settings-alert'), BerkutI18n.t('settings.classifications.limit'));
        } else if (result.reason === 'duplicate') {
          showSettingsAlert(document.getElementById('settings-alert'), BerkutI18n.t('settings.classifications.duplicate'));
        }
        return;
      }
      input.value = '';
      render();
    };

    addBtn.addEventListener('click', (e) => {
      e.preventDefault();
      add();
    });
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        add();
      }
    });
    document.addEventListener('classifications:changed', render);
    render();
  }

  function bindIncidentSettings() {
    if (typeof IncidentsPage === 'undefined') return;
    if (IncidentsPage.loadCustomOptions) {
      IncidentsPage.loadCustomOptions();
    }
    bindOptionControls({
      inputId: 'incident-type-input',
      buttonId: 'incident-type-add',
      listId: 'incident-type-list',
      getter: () => IncidentsPage.getIncidentTypes ? IncidentsPage.getIncidentTypes() : [],
      customList: () => IncidentsPage.state?.customIncidentTypes || [],
      onSave: (next) => IncidentsPage.saveCustomOptions ? IncidentsPage.saveCustomOptions({ incidentTypes: next }) : null
    });
    bindOptionControls({
      inputId: 'incident-source-input',
      buttonId: 'incident-source-add',
      listId: 'incident-source-list',
      getter: () => IncidentsPage.getDetectionSources ? IncidentsPage.getDetectionSources() : [],
      customList: () => IncidentsPage.state?.customDetectionSources || [],
      onSave: (next) => IncidentsPage.saveCustomOptions ? IncidentsPage.saveCustomOptions({ detectionSources: next }) : null
    });
  }

  function bindControlsSettings(alertBox) {
    if (typeof ControlsPage === 'undefined') return;
    if (ControlsPage.loadCustomOptions) {
      ControlsPage.loadCustomOptions();
    }
    bindOptionControls({
      inputId: 'controls-domain-input',
      buttonId: 'controls-domain-add',
      listId: 'controls-domain-list',
      getter: () => ControlsPage.getDomains ? ControlsPage.getDomains() : [],
      customList: () => ControlsPage.state?.customDomains || [],
      onSave: (next) => ControlsPage.saveCustomOptions ? ControlsPage.saveCustomOptions({ domains: next }) : null
    });
    bindControlTypeSettings(alertBox);
  }

  function bindControlTypeSettings(alertBox) {
    const input = document.getElementById('controls-type-input');
    const btn = document.getElementById('controls-type-add');
    const list = document.getElementById('controls-type-list');
    if (!input || !btn || !list) return;
    let types = [];

    const labelForType = (item) => {
      if (!item) return '-';
      const key = `controls.type.${String(item.name || '').toLowerCase()}`;
      const localized = BerkutI18n.t(key);
      return localized !== key ? localized : item.name;
    };

    const render = () => {
      list.innerHTML = '';
      if (!types.length) {
        const empty = document.createElement('div');
        empty.className = 'muted';
        empty.textContent = BerkutI18n.t('settings.options.empty');
        list.appendChild(empty);
        return;
      }
      types.forEach(item => {
        const removable = !item.is_builtin;
        const badge = removable ? BerkutI18n.t('settings.tags.custom') : BerkutI18n.t('incidents.settings.default');
        list.appendChild(buildPill(labelForType(item), removable, badge, () => removeType(item)));
      });
    };

    const load = async () => {
      try {
        const res = await Api.get('/api/controls/types');
        types = res.items || [];
      } catch (err) {
        types = [];
        showSettingsAlert(alertBox, err.message || BerkutI18n.t('common.error'));
      }
      render();
    };

    const addType = async () => {
      const val = (input.value || '').trim();
      if (!val) return;
      try {
        await Api.post('/api/controls/types', { name: val });
        input.value = '';
        await load();
        document.dispatchEvent(new CustomEvent('controls:typesUpdated'));
      } catch (err) {
        showSettingsAlert(alertBox, err.message || BerkutI18n.t('common.error'));
      }
    };

    const removeType = async (item) => {
      if (!item || !item.id) return;
      const ok = window.confirm(BerkutI18n.t('settings.controls.typesDeleteConfirm'));
      if (!ok) return;
      try {
        await Api.del(`/api/controls/types/${item.id}`);
        await load();
        document.dispatchEvent(new CustomEvent('controls:typesUpdated'));
      } catch (err) {
        showSettingsAlert(alertBox, err.message || BerkutI18n.t('common.error'));
      }
    };

    btn.addEventListener('click', (e) => {
      e.preventDefault();
      addType();
    });
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        addType();
      }
    });
    load();
  }

  function showSettingsAlert(alertBox, message, success) {
    if (!alertBox) return;
    alertBox.textContent = message;
    alertBox.hidden = false;
    if (success) {
      alertBox.classList.add('success');
    } else {
      alertBox.classList.remove('success');
    }
  }

  function bindOptionControls(cfg) {
    const input = document.getElementById(cfg.inputId);
    const btn = document.getElementById(cfg.buttonId);
    const list = document.getElementById(cfg.listId);
    if (!input || !btn || !list || !cfg.getter || !cfg.onSave || !cfg.customList) return;

    const render = () => {
      list.innerHTML = '';
      const all = cfg.getter() || [];
      const customs = (cfg.customList() || []).map(v => (v || '').toLowerCase());
      if (!all.length) {
        const empty = document.createElement('div');
        empty.className = 'muted';
        empty.textContent = BerkutI18n.t('settings.options.empty');
        list.appendChild(empty);
        return;
      }
      all.forEach(item => {
        const isCustom = customs.includes((item || '').toLowerCase());
        const badge = isCustom ? BerkutI18n.t('settings.tags.custom') : BerkutI18n.t('incidents.settings.default');
        list.appendChild(buildPill(item, isCustom, badge, () => {
          const next = (cfg.customList() || []).filter(v => v.toLowerCase() !== (item || '').toLowerCase());
          cfg.onSave(next);
          render();
        }));
      });
    };

    const add = () => {
      const val = (input.value || '').trim();
      if (!val) return;
      const current = cfg.customList() || [];
      const next = Array.from(new Set([...current, val]));
      cfg.onSave(next);
      input.value = '';
      render();
    };

    btn.addEventListener('click', (e) => {
      e.preventDefault();
      add();
    });
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        add();
      }
    });
    render();
  }

  function buildPill(label, removable, badgeText, onRemove) {
    const pill = document.createElement('span');
    pill.className = `pill ${removable ? 'pill-removable' : 'pill-muted'}`;
    pill.textContent = label;
    if (badgeText) {
      const badge = document.createElement('span');
      badge.className = 'pill-badge';
      badge.textContent = badgeText;
      pill.appendChild(badge);
    }
    if (removable) {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'pill-remove';
      btn.setAttribute('aria-label', BerkutI18n.t('common.delete'));
      btn.textContent = '\u00d7';
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        if (onRemove) onRemove();
      });
      pill.appendChild(btn);
    }
    return pill;
  }

  async function loadCurrentUser() {
    try {
      const res = await Api.get('/api/auth/me');
      const me = res.user;
      const roles = me?.roles || [];
      permissions = Array.isArray(me?.permissions) ? me.permissions.slice() : [];
      fullAccess = roles.includes('superadmin') || roles.includes('admin');
      currentUser = me;
      const isSuperadmin = roles.includes('superadmin');
      const canManageSettings = hasPerm('settings.general') || fullAccess;
      return { user: me, isSuperadmin, canManageSettings };
    } catch (err) {
      return { user: null, isSuperadmin: false, canManageSettings: false };
    }
  }

  function applyAccessControls(ctx, alertBox) {
    const { isSuperadmin } = ctx;
    const cleanupSection = document.getElementById('approvals-cleanup-section');
    if (cleanupSection) {
      cleanupSection.hidden = !isSuperadmin || !hasPerm('settings.advanced');
    }
    const tabsCleanupSection = document.getElementById('tabs-cleanup-section');
    if (tabsCleanupSection) {
      tabsCleanupSection.hidden = !isSuperadmin || !hasPerm('settings.advanced');
    }
    const monitoringCleanupSection = document.getElementById('monitoring-cleanup-section');
    if (monitoringCleanupSection) {
      monitoringCleanupSection.hidden = !isSuperadmin || !hasPerm('settings.advanced');
    }
    const tabs = Array.from(document.querySelectorAll('.settings-tabs .tab-btn'));
    const panels = Array.from(document.querySelectorAll('.settings-panel'));
    tabs.forEach(btn => {
      const tab = btn.dataset.tab;
      const allowed = canViewTab(tab);
      btn.hidden = !allowed;
      btn.disabled = !allowed;
      const panel = panels.find(p => p.dataset.tab === tab);
      if (panel) panel.hidden = panel.hidden || !allowed;
    });
    if (!firstAllowedTab()) {
      if (alertBox) {
        alertBox.textContent = BerkutI18n.t('settings.restrictedNotice');
        alertBox.classList.remove('success');
        alertBox.hidden = false;
      }
    }
  }

  function renderPasswordMeta(ctx) {
    const pwdLast = document.getElementById('password-last-changed');
    if (!pwdLast || !ctx || !ctx.user || !ctx.user.password_changed_at) return;
    const formatted = (typeof AppTime !== 'undefined' && AppTime.formatDateTime)
      ? AppTime.formatDateTime(ctx.user.password_changed_at)
      : (() => {
        const dt = new Date(ctx.user.password_changed_at);
        const pad = (num) => `${num}`.padStart(2, '0');
        return `${pad(dt.getDate())}.${pad(dt.getMonth() + 1)}.${dt.getFullYear()} ${pad(dt.getHours())}:${pad(dt.getMinutes())}`;
      })();
    pwdLast.textContent = `${BerkutI18n.t('accounts.passwordLastChanged')}: ${formatted}`;
  }

  function hasPerm(perm) {
    if (!perm) return true;
    if (fullAccess) return true;
    return permissions.includes(perm);
  }

  function canViewTab(tabId) {
    if (!tabId) return false;
    const perm = TAB_PERMISSIONS[tabId];
    return hasPerm(perm);
  }

  function firstAllowedTab() {
    const btn = Array.from(document.querySelectorAll('.settings-tabs .tab-btn')).find(b => canViewTab(b.dataset.tab) && !b.hidden);
    return btn ? btn.dataset.tab : null;
  }

  function tabFromPath() {
    const parts = window.location.pathname.split('/').filter(Boolean);
    if (parts[0] !== 'settings') return null;
    if (!parts[1]) return null;
    return `settings-${parts[1]}`;
  }

  function updateTabHash(tabId) {
    const slug = (tabId || '').replace('settings-', '');
    const next = slug ? `/settings/${slug}` : '/settings';
    if (window.location.pathname !== next) {
      window.history.replaceState({}, '', next);
    }
  }

  return { init };
})();
