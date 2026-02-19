const IncidentsPage = (() => {
  const state = {
    incidents: [],
    tabs: [],
    activeTabId: 'home',
    incidentDetails: new Map(),
    pendingStageIncidentId: null,
    currentUser: null,
    dashboard: { metrics: { open: 0, in_progress: 0, closed: 0, critical: 0 }, mine: [], attention: [], recent: [] },
    filters: { status: '', severity: '', scope: 'all', period: 'all' },
    customIncidentTypes: [],
    customDetectionSources: []
  };
  function hasPermission(perm) {
    if (!perm) return true;
    const perms = Array.isArray(state.currentUser?.permissions) ? state.currentUser.permissions : [];
    if (!perms.length) return true;
    return perms.includes(perm);
  }
  let customLoaded = false;
  let autoSaveTimer = null;
  let autoSaveRunning = false;
  const PREF_KEY = 'berkut_prefs';
  const DEFAULT_INCIDENT_TYPES = [
    'Вредоносное ПО',
    'Вирус',
    'Троян',
    'Ransomware',
    'Spyware',
    'Backdoor',
    'Botnet',
    'Cryptominer',
    'Учетные записи и доступ',
    'Компрометация учетной записи',
    'Подбор пароля',
    'Эскалация привилегий',
    'Использование украденных учетных данных',
    'Нарушение MFA',
    'Регламенты и политика',
    'Нарушение регламентов',
    'Нарушение политики ИБ',
    'Нарушение требований ФЗ',
    'Нарушение требований GDPR',
    'Нарушение требований ISO',
    'Ненадлежащее хранение данных',
    'Ошибка конфигурации',
    'Внутренние угрозы',
    'Инсайдерская угроза',
    'Утечка по вине сотрудника',
    'Злоупотребление правами',
    'Нарушение служебных обязанностей',
    'Сеть и инфраструктура',
    'Сканирование сети',
    'DDoS',
    'Атака на сервис',
    'Подозрительный сетевой трафик',
    'Подключение неавторизованного устройства',
    'ИТ / Доступность',
    'Отказ сервиса',
    'Нарушение доступности',
    'Потеря данных',
    'Ошибка обновления',
    'Компрометация сервера',
    'Общие',
    'Инцидент ИБ (общее)',
    'Потенциальный инцидент',
    'Ложное срабатывание',
    'Другое',
    'Утечка данных',
    'Фишинг',
    'Несанкционированный доступ',
    'Аномалия',
    'Подозрительная активность'
  ];
  const DEFAULT_DETECTION_SOURCES = [
    'SIEM',
    'DLP',
    'EDR',
    'XDR',
    'IDS',
    'IPS',
    'WAF',
    'SOAR',
    'Пользователь',
    'Сотрудник ИБ',
    'SOC Аналитик',
    'Администратор',
    'Руководитель',
    'Аудит',
    'Внутренний контроль',
    'Проверка соответствия',
    'Пентест',
    'Red Team',
    'Blue Team',
    'Purple Team',
    'Внешнее уведомление',
    'Контрагент',
    'Регулятор',
    'CERT',
    'CSIRT',
    'Правоохранительные органы',
    'Мониторинг',
    'Система мониторинга',
    'Логи приложений',
    'Анализ журналов',
    'Корреляция событий',
    'Неизвестно',
    'Другое',
    'SOC / Analyst'
  ];
  const CUSTOM_OPTIONS_KEY = 'incidents.customOptions';

  async function init() {
    const page = document.getElementById('incidents-page');
    if (!page) return;
    loadCustomOptions();
    state.tabs = [
      { id: 'home', type: 'home', titleKey: 'incidents.tabs.home', closable: false },
      { id: 'list', type: 'list', titleKey: 'incidents.tabs.incidents', closable: false },
    ];
    const startupIncidentId = extractIncidentIdFromLocation();
    if (IncidentsPage.renderTabBar) IncidentsPage.renderTabBar();
    if (IncidentsPage.renderHome) IncidentsPage.renderHome();
    if (IncidentsPage.renderList) IncidentsPage.renderList();
    if (IncidentsPage.bindStageModal) IncidentsPage.bindStageModal();
    if (IncidentsPage.bindContextMenu) IncidentsPage.bindContextMenu();
    if (IncidentsPage.switchTab) IncidentsPage.switchTab('home');
    if (IncidentsPage.loadCurrentUser) await IncidentsPage.loadCurrentUser();
    if (IncidentsPage.ensureUserDirectory) {
      await IncidentsPage.ensureUserDirectory();
    }
    if (IncidentsPage.loadDashboard) await IncidentsPage.loadDashboard();
    if (IncidentsPage.loadIncidents) await IncidentsPage.loadIncidents();
    startAutoSave();
    window.addEventListener('beforeunload', (e) => {
      if (hasDirtyIncidents()) {
        e.preventDefault();
        e.returnValue = '';
      }
    });
    if (startupIncidentId && IncidentsPage.openIncidentTab) {
      await IncidentsPage.openIncidentTab(startupIncidentId, { fromLink: true });
    } else {
      await handleIncidentDeepLink();
    }
    await handlePendingIncidentOpen();
  }

  function t(key) {
    const value = BerkutI18n.t(key);
    return value === key ? key : value;
  }

  function showError(err, fallbackKey) {
    const raw = (err && err.message ? err.message : '').trim();
    const msg = raw && t(raw) !== raw ? t(raw) : t(fallbackKey || 'common.error');
    window.alert(msg || raw || 'Error');
  }

  function parseDate(value) {
    if (!value) return null;
    if (value instanceof Date) {
      return Number.isNaN(value.getTime()) ? null : value;
    }
    const d = new Date(value);
    if (Number.isNaN(d.getTime())) return null;
    return d;
  }

  function formatDate(value) {
    const d = parseDate(value);
    if (!d) return value || '';
    if (typeof AppTime !== 'undefined' && AppTime.formatDateTime) {
      return AppTime.formatDateTime(d);
    }
    const pad = (num) => `${num}`.padStart(2, '0');
    return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${d.getFullYear()} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }

  function getIncidentStatusDisplay(incident) {
    let status = (incident?.status || '').toLowerCase();
    let label = status ? t(`incidents.status.${status}`) : '';
    if (!label || label === `incidents.status.${status}`) label = status;
    if (status === 'closed') {
      const outcome = (incident?.meta?.closure_outcome || '').toLowerCase();
      if (outcome) {
        const outcomeLabel = t(`incidents.stage.blocks.decisions.outcome.${outcome}`);
        if (outcomeLabel && !outcomeLabel.startsWith('incidents.')) {
          label = outcomeLabel;
        } else {
          label = outcome;
        }
        status = outcome === 'closed' ? 'closed' : `closed_${outcome}`;
      }
    }
    return { status, label };
  }

  function toISODateTime(dateVal, timeVal) {
    if (typeof AppTime !== 'undefined' && AppTime.toISODateTime) {
      return AppTime.toISODateTime(dateVal, timeVal);
    }
    const date = (dateVal || '').trim();
    const time = (timeVal || '').trim();
    if (!date && !time) return '';
    const [yearStr, monthStr, dayStr] = date.split('-');
    const [hourStr = '0', minStr = '0', secStr = '0'] = time.split(':');
    const year = parseInt(yearStr, 10);
    const month = parseInt(monthStr, 10) - 1;
    const day = parseInt(dayStr, 10);
    const hour = parseInt(hourStr, 10);
    const minute = parseInt(minStr, 10);
    const second = parseInt(secStr, 10);
    const dt = new Date(Date.UTC(year, month, day, hour, minute, second));
    if (Number.isNaN(dt.getTime())) return '';
    return dt.toISOString();
  }

  function splitISODateTime(value) {
    if (typeof AppTime !== 'undefined' && AppTime.splitDateTime) {
      return AppTime.splitDateTime(value);
    }
    const d = parseDate(value);
    if (!d) return { date: '', time: '' };
    const pad = (num) => `${num}`.padStart(2, '0');
    return {
      date: `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${d.getFullYear()}`,
      time: `${pad(d.getHours())}:${pad(d.getMinutes())}`
    };
  }

  function escapeHtml(str) {
    return (str || '').toString().replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }

  function confirmAction(opts = {}) {
    const modal = document.getElementById('confirm-modal');
    const title = opts.title || t('common.confirm');
    const message = opts.message || '';
    const confirmText = opts.confirmText || t('common.confirm');
    const cancelText = opts.cancelText || t('common.cancel');
    if (!modal) {
      return Promise.resolve(window.confirm(message || title));
    }
    const titleEl = document.getElementById('confirm-modal-title');
    const msgEl = document.getElementById('confirm-modal-message');
    const yesBtn = document.getElementById('confirm-modal-yes');
    const noBtn = document.getElementById('confirm-modal-no');
    const closeBtn = document.getElementById('confirm-modal-close');
    if (titleEl) titleEl.textContent = title;
    if (msgEl) msgEl.textContent = message;
    if (yesBtn) yesBtn.textContent = confirmText;
    if (noBtn) noBtn.textContent = cancelText;
    modal.hidden = false;
    return new Promise(resolve => {
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

  function promptUnsavedChanges(opts = {}) {
    const modal = document.getElementById('confirm-modal');
    const title = opts.title || t('incidents.unsavedTitle');
    const message = opts.message || t('incidents.unsavedPrompt');
    const saveText = opts.saveText || t('incidents.unsavedSave');
    const discardText = opts.discardText || t('incidents.unsavedDiscard');
    if (!modal) {
      const confirmSave = window.confirm(message || title);
      return Promise.resolve(confirmSave ? 'save' : 'discard');
    }
    const titleEl = document.getElementById('confirm-modal-title');
    const msgEl = document.getElementById('confirm-modal-message');
    const yesBtn = document.getElementById('confirm-modal-yes');
    const noBtn = document.getElementById('confirm-modal-no');
    const closeBtn = document.getElementById('confirm-modal-close');
    if (titleEl) titleEl.textContent = title;
    if (msgEl) msgEl.textContent = message;
    if (yesBtn) yesBtn.textContent = saveText;
    if (noBtn) noBtn.textContent = discardText;
    modal.hidden = false;
    return new Promise(resolve => {
      const cleanup = (result) => {
        modal.hidden = true;
        if (yesBtn) yesBtn.onclick = null;
        if (noBtn) noBtn.onclick = null;
        if (closeBtn) closeBtn.onclick = null;
        resolve(result);
      };
      if (yesBtn) yesBtn.onclick = () => cleanup('save');
      if (noBtn) noBtn.onclick = () => cleanup('discard');
      if (closeBtn) closeBtn.onclick = () => cleanup('cancel');
      if (yesBtn) yesBtn.focus();
    });
  }

  function loadPreferences() {
    try {
      const raw = localStorage.getItem(PREF_KEY);
      if (!raw) return {};
      return JSON.parse(raw);
    } catch (_) {
      return {};
    }
  }

  function autoSaveConfig() {
    const prefs = loadPreferences();
    const minutes = parseInt(prefs.incidentAutoSavePeriod, 10);
    const intervalMs = Number.isFinite(minutes) && minutes > 0 ? minutes * 60000 : 0;
    return { enabled: !!prefs.incidentAutoSaveEnabled, intervalMs };
  }

  function stopAutoSave() {
    if (autoSaveTimer) {
      clearInterval(autoSaveTimer);
      autoSaveTimer = null;
    }
  }

  async function runAutoSave() {
    if (autoSaveRunning) return;
    if (!IncidentsPage.saveIncidentChanges || !IncidentsPage.isIncidentDirty) return;
    autoSaveRunning = true;
    try {
      for (const [incidentId, detail] of state.incidentDetails.entries()) {
        if (detail && IncidentsPage.isIncidentDirty(detail)) {
          await IncidentsPage.saveIncidentChanges(incidentId, { silent: true });
        }
      }
    } finally {
      autoSaveRunning = false;
    }
  }

  function startAutoSave() {
    stopAutoSave();
    const cfg = autoSaveConfig();
    if (!cfg.enabled || !cfg.intervalMs) return;
    autoSaveTimer = setInterval(runAutoSave, cfg.intervalMs);
  }

  function normalizeOptionValue(val) {
    return (val || '').toString().trim();
  }

  function mergeOptionLists(defaults, extras) {
    const seen = new Set();
    const out = [];
    [...(defaults || []), ...(extras || [])].forEach(item => {
      const clean = normalizeOptionValue(item);
      if (!clean) return;
      const key = clean.toLowerCase();
      if (seen.has(key)) return;
      seen.add(key);
      out.push(clean);
    });
    return out;
  }

  function loadCustomOptions() {
    customLoaded = true;
    try {
      const raw = localStorage.getItem(CUSTOM_OPTIONS_KEY);
      if (!raw) return;
      const parsed = JSON.parse(raw);
      state.customIncidentTypes = Array.isArray(parsed.incidentTypes) ? mergeOptionLists([], parsed.incidentTypes) : [];
      state.customDetectionSources = Array.isArray(parsed.detectionSources) ? mergeOptionLists([], parsed.detectionSources) : [];
    } catch (_) {
      state.customIncidentTypes = [];
      state.customDetectionSources = [];
    }
  }

  function persistCustomOptions() {
    try {
      localStorage.setItem(CUSTOM_OPTIONS_KEY, JSON.stringify({
        incidentTypes: state.customIncidentTypes,
        detectionSources: state.customDetectionSources
      }));
    } catch (_) {
      // ignore storage errors
    }
  }

  function saveCustomOptions({ incidentTypes, detectionSources }) {
    if (incidentTypes) {
      state.customIncidentTypes = mergeOptionLists([], incidentTypes);
    }
    if (detectionSources) {
      state.customDetectionSources = mergeOptionLists([], detectionSources);
    }
    persistCustomOptions();
    emitOptionChange();
    if (IncidentsPage.refreshOptionConsumers) {
      IncidentsPage.refreshOptionConsumers();
    }
  }

  function emitOptionChange() {
    try {
      if (typeof document !== 'undefined' && typeof CustomEvent !== 'undefined') {
        document.dispatchEvent(new CustomEvent('incidents:optionsChanged'));
      }
    } catch (_) {
      // ignore dispatch issues
    }
  }

  function getIncidentTypes() {
    ensureCustomOptionsLoaded();
    return mergeOptionLists(DEFAULT_INCIDENT_TYPES, state.customIncidentTypes);
  }

  function getDetectionSources() {
    ensureCustomOptionsLoaded();
    return mergeOptionLists(DEFAULT_DETECTION_SOURCES, state.customDetectionSources);
  }

  function ensureCustomOptionsLoaded() {
    if (!customLoaded) {
      loadCustomOptions();
    }
  }

  async function ensureUserDirectory() {
    if (typeof UserDirectory === 'undefined' || !UserDirectory.load) return;
    await UserDirectory.load();
  }

  function getSelectedValues(select) {
    if (!select) return [];
    return Array.from(select.options).filter(o => o.selected).map(o => o.value).filter(Boolean);
  }

  function setDefaultSelectValues(select) {
    if (!select) return;
    const values = getSelectedValues(select);
    select.dataset.defaultValues = JSON.stringify(values);
  }

  function populateSelectOptions(select, options) {
    if (!select) return;
    const current = select.value;
    select.innerHTML = '';
    (options || []).forEach(opt => {
      const option = document.createElement('option');
      option.value = opt;
      option.textContent = opt;
      select.appendChild(option);
    });
    if (current && Array.from(select.options).some(o => o.value === current)) {
      select.value = current;
    }
  }

  function refreshOptionConsumers() {
    document.querySelectorAll('.incident-type-select').forEach(sel => {
      populateSelectOptions(sel, getIncidentTypes());
    });
    document.querySelectorAll('.incident-source-select').forEach(sel => {
      populateSelectOptions(sel, getDetectionSources());
    });
  }

  function renderSelectedHint(select, hintEl) {
    if (!select || !hintEl) return;
    const names = Array.from(select.options)
      .filter(o => o.selected)
      .map(o => o.dataset.label || o.textContent);
    hintEl.innerHTML = '';
    if (!names.length) {
      hintEl.textContent = '-';
      return;
    }
    names.forEach(name => {
      const badge = document.createElement('span');
      badge.className = 'tag';
      badge.textContent = name;
      hintEl.appendChild(badge);
    });
  }

  function enforceSingleSelect(select) {
    if (!select) return;
    select.dataset.single = '1';
    select.addEventListener('mousedown', (e) => {
      const opt = e.target.closest('option');
      if (!opt) return;
      select.dataset.lastSelected = opt.value;
    });
    const ensureSingle = () => {
      const last = select.dataset.lastSelected;
      if (!last) return;
      let selected = false;
      Array.from(select.options).forEach(opt => {
        if (opt.value === last) {
          opt.selected = true;
          selected = true;
        } else if (opt.selected) {
          opt.selected = false;
        }
      });
      if (selected && typeof DocsPage !== 'undefined' && DocsPage.enhanceMultiSelects) {
        select.dispatchEvent(new Event('selectionrefresh', { bubbles: false }));
      }
    };
    select.addEventListener('change', ensureSingle);
    select.addEventListener('selectionrefresh', ensureSingle);
  }

  function populateUserSelect(select, selectedValues = []) {
    if (!select) return;
    select.innerHTML = '';
    if (typeof UserDirectory === 'undefined' || !UserDirectory.all) return;
    UserDirectory.all().forEach(u => {
      const opt = document.createElement('option');
      opt.value = u.username;
      opt.textContent = u.full_name || u.username;
      opt.dataset.label = opt.textContent;
      if (selectedValues.includes(u.username)) opt.selected = true;
      select.appendChild(opt);
    });
    if (!select.id) {
      select.id = `incident-user-${Math.random().toString(36).slice(2, 9)}`;
    }
    if (typeof DocsPage !== 'undefined' && DocsPage.enhanceMultiSelects) {
      DocsPage.enhanceMultiSelects([select.id]);
    }
  }

  function extractIncidentIdFromLocation() {
    const url = new URL(window.location.href);
    const parts = window.location.pathname.split('/').filter(Boolean);
    let rawId = '';
    if (parts[0] === 'incidents' && parts[1]) {
      rawId = parts[1];
    } else {
      const hash = window.location.hash.replace('#', '');
      if (hash.startsWith('incident=')) {
        rawId = hash.split('incident=')[1] || '';
      } else {
        rawId = url.searchParams.get('incident') || '';
      }
    }
    if (!rawId) return;
    const incidentId = parseInt(rawId, 10);
    if (!Number.isFinite(incidentId)) return;
    return incidentId;
  }

  async function handleIncidentDeepLink() {
    const incidentId = extractIncidentIdFromLocation();
    if (!incidentId) return;
    if (IncidentsPage.openIncidentTab) {
      await IncidentsPage.openIncidentTab(incidentId, { fromLink: true });
    }
  }

  async function handlePendingIncidentOpen() {
    if (!window.__pendingIncidentOpen) return;
    const incidentId = window.__pendingIncidentOpen;
    window.__pendingIncidentOpen = null;
    if (IncidentsPage.openIncidentTab) {
      await IncidentsPage.openIncidentTab(incidentId, { fromLink: true });
    }
  }

  function clearState() {
    stopAutoSave();
    state.incidents = [];
    state.tabs = [];
    state.activeTabId = 'home';
    state.incidentDetails = new Map();
    state.pendingStageIncidentId = null;
    state.dashboard = { metrics: { open: 0, in_progress: 0, closed: 0, critical: 0 }, mine: [], attention: [], recent: [] };
    state.filters = { status: '', severity: '', scope: 'all', period: 'all' };
    state.customIncidentTypes = [];
    state.customDetectionSources = [];
    customLoaded = false;
  }

  function hasDirtyIncidents() {
    if (!IncidentsPage.isIncidentDirty) return false;
    for (const detail of state.incidentDetails.values()) {
      if (IncidentsPage.isIncidentDirty(detail)) return true;
    }
    return false;
  }

  return {
    init,
    state,
    t,
    showError,
    confirmAction,
    promptUnsavedChanges,
    parseDate,
    formatDate,
    getIncidentStatusDisplay,
    toISODateTime,
    splitISODateTime,
    escapeHtml,
    getIncidentTypes,
    getDetectionSources,
    saveCustomOptions,
    refreshOptionConsumers,
    populateSelectOptions,
    ensureUserDirectory,
    populateUserSelect,
    renderSelectedHint,
    getSelectedValues,
    setDefaultSelectValues,
    enforceSingleSelect,
    loadCustomOptions,
    ensureCustomOptionsLoaded: ensureCustomOptionsLoaded,
    emitOptionChange: emitOptionChange,
    hasPermission,
    hasDirtyIncidents,
    clearState,
    startAutoSave,
    stopAutoSave
  };
})();

if (typeof window !== 'undefined') {
  window.IncidentsPage = IncidentsPage;
  window.addEventListener('DOMContentLoaded', () => {
    if (document.getElementById('incidents-page')) {
      IncidentsPage.init();
    }
  });
}
