const LogsPage = (() => {
  const state = {
    items: [],
    savedViews: [],
    filters: {
      section: '',
      action: '',
      user: '',
      from: '',
      to: '',
    },
  };

  const els = {};

  async function init() {
    const page = document.getElementById('logs-page');
    if (!page) return;
    els.refresh = document.getElementById('logs-refresh');
    els.tbody = document.querySelector('#logs-table tbody');
    els.section = document.getElementById('logs-filter-section');
    els.action = document.getElementById('logs-filter-action');
    els.user = document.getElementById('logs-filter-user');
    els.from = document.getElementById('logs-filter-from');
    els.to = document.getElementById('logs-filter-to');
    els.reset = document.getElementById('logs-filter-reset');
    els.saveView = document.getElementById('logs-save-view');
    els.savedViews = document.getElementById('logs-saved-views');
    els.exportBtn = document.getElementById('logs-export');

    applyDateInputLocale();

    if (els.refresh) els.refresh.onclick = () => load();
    if (els.saveView) els.saveView.onclick = () => saveCurrentView();
    if (els.exportBtn) els.exportBtn.onclick = () => exportCurrentView();
    if (els.savedViews) {
      els.savedViews.addEventListener('change', applySavedViewFromSelect);
    }

    const onFilterChange = () => {
      syncFilters();
      applyFilters();
    };

    [els.section, els.action, els.user, els.from, els.to].forEach(el => {
      if (!el) return;
      el.addEventListener('input', onFilterChange);
      el.addEventListener('change', onFilterChange);
    });

    if (els.reset) {
      els.reset.addEventListener('click', () => {
        resetFilters();
        applyFilters();
      });
    }

    await load();
    loadSavedViews();
  }

  function applyDateInputLocale() {
    const page = document.getElementById('logs-page');
    if (!page) return;
    const lang = (typeof BerkutI18n !== 'undefined' && BerkutI18n.currentLang) ? BerkutI18n.currentLang() : 'ru';
    page.querySelectorAll('input[type="date"], input[type="datetime-local"]').forEach(input => {
      input.lang = lang === 'en' ? 'en' : 'ru';
    });
  }

  async function load() {
    if (els.tbody) els.tbody.innerHTML = '';
    let items = [];
    try {
      const params = buildQueryFromFilters();
      const res = await Api.get(`/api/logs?${params.toString()}`);
      items = res.items || [];
    } catch (err) {
      console.error('logs load', err);
    }
    state.items = items;
    renderSectionOptions(items);
    syncFilters();
    applyFilters();
  }

  function syncFilters() {
    state.filters.section = els.section?.value || '';
    state.filters.action = (els.action?.value || '').trim();
    state.filters.user = (els.user?.value || '').trim();
    state.filters.from = els.from?.value || '';
    state.filters.to = els.to?.value || '';
  }

  function resetFilters() {
    if (els.section) els.section.value = '';
    if (els.action) els.action.value = '';
    if (els.user) els.user.value = '';
    if (els.from) els.from.value = '';
    if (els.to) els.to.value = '';
    syncFilters();
    if (els.savedViews) els.savedViews.value = '';
  }

  function buildQueryFromFilters() {
    const params = new URLSearchParams();
    if (state.filters.section) params.set('section', state.filters.section);
    if (state.filters.action) params.set('action', state.filters.action);
    if (state.filters.user) params.set('user', state.filters.user);
    if (state.filters.from) params.set('since', state.filters.from);
    if (state.filters.to) params.set('to', state.filters.to);
    if (state.filters.action) params.set('q', state.filters.action);
    params.set('limit', '2000');
    return params;
  }

  function savedViewsKey() {
    const user = typeof App !== 'undefined' && App.state?.user?.username ? App.state.user.username : 'default';
    return `logs.saved_views.${user}`;
  }

  function loadSavedViews() {
    try {
      const raw = localStorage.getItem(savedViewsKey());
      state.savedViews = raw ? JSON.parse(raw) : [];
    } catch (_) {
      state.savedViews = [];
    }
    renderSavedViews();
  }

  function renderSavedViews() {
    if (!els.savedViews) return;
    const select = els.savedViews;
    const current = select.value;
    select.innerHTML = '';
    const placeholder = document.createElement('option');
    placeholder.value = '';
    placeholder.textContent = BerkutI18n.t('logs.views.placeholder') || 'Saved views';
    select.appendChild(placeholder);
    state.savedViews.forEach((item, idx) => {
      const opt = document.createElement('option');
      opt.value = `${idx}`;
      opt.textContent = item.name || `View ${idx + 1}`;
      select.appendChild(opt);
    });
    if (current) select.value = current;
  }

  function saveCurrentView() {
    const name = (window.prompt(BerkutI18n.t('logs.views.prompt') || 'View name', '') || '').trim();
    if (!name) return;
    const payload = {
      name,
      filters: { ...state.filters },
      created_at: new Date().toISOString(),
    };
    const filtered = (state.savedViews || []).filter((item) => (item.name || '').toLowerCase() !== name.toLowerCase());
    filtered.unshift(payload);
    state.savedViews = filtered.slice(0, 20);
    localStorage.setItem(savedViewsKey(), JSON.stringify(state.savedViews));
    renderSavedViews();
    if (els.savedViews) els.savedViews.value = '0';
  }

  function applySavedViewFromSelect() {
    if (!els.savedViews) return;
    const idx = parseInt(els.savedViews.value, 10);
    if (!Number.isFinite(idx) || idx < 0 || !state.savedViews[idx]) return;
    const view = state.savedViews[idx];
    const filters = view.filters || {};
    if (els.section) els.section.value = filters.section || '';
    if (els.action) els.action.value = filters.action || '';
    if (els.user) els.user.value = filters.user || '';
    if (els.from) els.from.value = filters.from || '';
    if (els.to) els.to.value = filters.to || '';
    syncFilters();
    load();
  }

  function exportCurrentView() {
    const params = buildQueryFromFilters();
    window.open(`/api/logs/export?${params.toString()}`, '_blank');
  }

  function applyFilters() {
    if (!els.tbody) return;
    const filtered = state.items.filter(item => {
      if (!item) return false;
      if (state.filters.section) {
        const section = categoryForAction(item.action);
        if (section !== state.filters.section) return false;
      }
      if (state.filters.user) {
        const user = (item.username || '').toLowerCase();
        if (!user.includes(state.filters.user.toLowerCase())) return false;
      }
      if (state.filters.action) {
        const query = state.filters.action.toLowerCase();
        const actionLabel = prettyAction(item.action).toLowerCase();
        const raw = (item.action || '').toLowerCase();
        const details = (item.details || '').toLowerCase();
        if (!actionLabel.includes(query) && !raw.includes(query) && !details.includes(query)) return false;
      }
      const createdAt = toDate(item.created_at);
      if (state.filters.from) {
        const from = toDate(state.filters.from);
        if (from && createdAt && createdAt < from) return false;
      }
      if (state.filters.to) {
        const to = toDate(state.filters.to);
        if (to && createdAt && createdAt > to) return false;
      }
      return true;
    });

    renderRows(filtered);
  }

  function renderRows(items) {
    if (!els.tbody) return;
    els.tbody.innerHTML = '';
    if (!items.length) {
      const tr = document.createElement('tr');
      tr.innerHTML = `<td colspan="4">${BerkutI18n.t('logs.empty') || '-'}</td>`;
      els.tbody.appendChild(tr);
      return;
    }
    items.forEach(i => {
      const tr = document.createElement('tr');
      tr.innerHTML = `
        <td>${formatDate(i.created_at)}</td>
        <td>${escapeHtml(i.username)}</td>
        <td>${escapeHtml(prettyAction(i.action))}</td>
        <td>${escapeHtml(prettyDetails(i))}</td>
      `;
      els.tbody.appendChild(tr);
    });
  }

  function renderSectionOptions(items) {
    if (!els.section) return;
    const current = els.section.value;
    const sections = new Set();
    items.forEach(item => sections.add(categoryForAction(item.action)));
    const ordered = SECTION_ORDER.filter(key => sections.has(key));
    els.section.innerHTML = '';

    const allOpt = document.createElement('option');
    allOpt.value = '';
    allOpt.textContent = BerkutI18n.t('common.all') || 'All';
    els.section.appendChild(allOpt);

    ordered.forEach(key => {
      const opt = document.createElement('option');
      opt.value = key;
      opt.textContent = sectionLabel(key);
      els.section.appendChild(opt);
    });

    if (current) {
      const exists = Array.from(els.section.options).some(o => o.value === current);
      if (exists) els.section.value = current;
    }
  }

  function sectionLabel(key) {
    const labelKey = `logs.section.${key}`;
    const label = BerkutI18n?.t ? BerkutI18n.t(labelKey) : key;
    return label || key;
  }

  function toDate(value) {
    if (!value) return null;
    if (typeof AppTime !== 'undefined' && AppTime.parseDateTimeInput) {
      return AppTime.parseDateTimeInput(value);
    }
    const dt = new Date(value);
    return Number.isNaN(dt.getTime()) ? null : dt;
  }

  function escapeHtml(str) {
    return (str || '').toString().replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }

  function formatDate(d) {
    if (!d) return '';
    try {
      if (typeof AppTime !== 'undefined' && AppTime.formatDateTime) {
        return AppTime.formatDateTime(d);
      }
      const dt = new Date(d);
      const pad = (n) => (n < 10 ? `0${n}` : `${n}`);
      return `${pad(dt.getDate())}.${pad(dt.getMonth() + 1)}.${dt.getFullYear()} ${pad(dt.getHours())}:${pad(dt.getMinutes())}`;
    } catch (_) {
      return d;
    }
  }

  function prettyAction(action) {
    const lang = BerkutI18n?.currentLang?.() || 'ru';
    const map = ACTION_LABELS[lang] || ACTION_LABELS.ru;
    if (map[action]) return map[action];
    const translated = BerkutI18n?.t ? BerkutI18n.t(action) : '';
    if (translated && translated !== action) return translated;
    return action;
  }

  function prettyDetails(item) {
    if (!item) return '';
    const act = item.action || '';
    const details = (item.details || '').trim();
    const lang = BerkutI18n?.currentLang?.() || 'ru';
    const labels = DETAIL_LABELS[lang] || DETAIL_LABELS.ru;

    const formatDocDetails = (code) => {
      if (!code) return '';
      const parts = String(code).split('|').map((part) => part.trim()).filter(Boolean);
      const docCode = parts[0] || '';
      const base = (docCode.startsWith('PUB') || docCode.startsWith('CONF') || docCode.startsWith('SEC'))
        ? `${labels.document} ${docCode}`
        : `${labels.document} #${docCode}`;
      if (parts.length <= 1) return base;
      const detailMap = labels?.doc || {};
      const extra = parts.slice(1).map((part) => detailMap[part] || part);
      return `${base} | ${extra.join(' | ')}`;
    };

    if (act.startsWith('doc.')) return formatDocDetails(details);
    if (act === 'settings.updates.check') {
      return formatUpdateCheckDetails(details, labels);
    }
    if (act === 'settings.hardening.read') {
      const m = /^score=(.+)$/i.exec(details);
      if (m && m[1]) {
        const caption = labels?.settings?.score || 'Score';
        return `${caption}: ${m[1]}`;
      }
      return details;
    }
    if (act.startsWith('approval')) {
      if (!details) return '';
      return isNaN(details) ? `${labels.approval} ${details}` : `${labels.approval} #${details}`;
    }
    if (act.startsWith('backups.')) {
      return formatBackupDetails(details, labels);
    }
    if (act.startsWith('monitoring.')) {
      return formatMonitoringDetails(act, details, labels);
    }
    if (/^\d+$/.test(details) && NOISY_NUMERIC_DETAILS_ACTIONS.has(act)) {
      return '';
    }
    if (details && labels[details]) return labels[details];
    return details || '';
  }

  function formatUpdateCheckDetails(details, labels) {
    if (!details) return '';
    const dict = labels?.updates || {};
    const parts = details.split('|').map((chunk) => chunk.trim()).filter(Boolean);
    if (!parts.length) return details;
    const rendered = parts.map((part) => {
      const eq = part.indexOf('=');
      if (eq <= 0) return part;
      const key = part.slice(0, eq).trim();
      const value = part.slice(eq + 1).trim();
      if (!key) return part;
      const keyLabel = dict[key] || key;
      if (key === 'has_update') {
        const boolLabel = value === 'true' ? (dict.true || value) : (dict.false || value);
        return `${keyLabel}: ${boolLabel}`;
      }
      return `${keyLabel}: ${value}`;
    });
    return rendered.join(' | ');
  }

  function formatBackupDetails(details, labels) {
    if (!details) return '';
    const dict = labels?.backups || {};
    const resultLabels = {
      requested: dict.result_requested || 'requested',
      queued: dict.result_queued || 'queued',
      success: dict.result_success || 'success',
      failed: dict.result_failed || 'failed',
      denied: dict.result_denied || 'denied',
      not_found: dict.result_not_found || 'not_found',
      not_implemented: dict.result_not_implemented || 'not_implemented',
    };
    const codeToKey = {
      'backups.concurrent_operation': 'backups.error.concurrentOperation',
      'backups.not_found': 'backups.error.notFound',
      'backups.not_ready': 'backups.error.notReady',
      'backups.file_missing': 'backups.error.fileMissing',
      'backups.invalid_request': 'backups.error.invalidRequest',
      'backups.internal': 'backups.error.internal',
    };
    const segments = details.split('|').map((chunk) => chunk.trim()).filter(Boolean);
    const pairs = segments.flatMap((segment) => {
      const eq = segment.indexOf('=');
      const colon = segment.indexOf(':');
      if (eq > 0) return [[segment.slice(0, eq).trim(), segment.slice(eq + 1).trim()]];
      if (colon > 0) return [[segment.slice(0, colon).trim(), segment.slice(colon + 1).trim()]];
      if (segment.includes('=')) {
        return segment
          .split(/\s+/)
          .map((token) => token.trim())
          .filter(Boolean)
          .map((token) => {
            const tokenEq = token.indexOf('=');
            if (tokenEq <= 0) return [token, ''];
            return [token.slice(0, tokenEq).trim(), token.slice(tokenEq + 1).trim()];
          });
      }
      return [[segment, '']];
    });
    const rendered = pairs.map(([key, rawValue]) => {
      if (!key) return '';
      const keyLabel = dict[key] || key;
      if ((key === 'code' || key === 'reason_code') && rawValue) {
        const i18nKey = codeToKey[rawValue];
        if (i18nKey) {
          const translated = BerkutI18n.t(i18nKey);
          const value = translated && translated !== i18nKey ? translated : rawValue;
          return `${keyLabel}: ${value}`;
        }
      }
      if (key === 'event' && rawValue) {
        const eventText = BerkutI18n.t(rawValue);
        const value = eventText && eventText !== rawValue ? eventText : rawValue;
        return `${keyLabel}: ${value}`;
      }
      if (key === 'result' && rawValue) {
        const normalized = rawValue.toLowerCase();
        const value = resultLabels[normalized] || rawValue;
        return `${keyLabel}: ${value}`;
      }
      if (key === 'resource' && rawValue) {
        const normalized = rawValue.toLowerCase();
        const value = dict[`resource_${normalized}`] || rawValue;
        return `${keyLabel}: ${value}`;
      }
      if (!rawValue) return keyLabel;
      return `${keyLabel}: ${rawValue}`;
    });
    return rendered.join(' | ');
  }

  function formatMonitoringDetails(action, details, labels) {
    if (!details) return '';
    const dict = labels?.monitoring || {};
    if (action === 'monitoring.notification.channel.apply_all') {
      const parts = details.split('|').map((x) => x.trim()).filter(Boolean);
      if (parts.length >= 2) {
        const channel = `${dict.channel || 'Канал'}: ${parts[0]}`;
        const applied = `${dict.applied || 'Применено'}: ${parts[1]}`;
        return `${channel} | ${applied}`;
      }
    }
    if (action === 'monitoring.settings.update') {
      const pieces = details.split('|').map((chunk) => chunk.trim()).filter(Boolean);
      if (!pieces.length) return details;
      const rendered = pieces.map((piece) => {
        const eq = piece.indexOf('=');
        if (eq <= 0) return piece;
        const key = piece.slice(0, eq).trim();
        const value = piece.slice(eq + 1).trim();
        const keyLabel = dict[key] || key;
        return `${keyLabel}: ${value}`;
      });
      return rendered.join(' | ');
    }
    if (/^\d+$/.test(details)) {
      return `${dict.id || 'ID'}: ${details}`;
    }
    return details;
  }

  function categoryForAction(action) {
    if (!action) return 'other';
    if (action.startsWith('doc.') || action.startsWith('template.')) return 'docs';
    if (action.startsWith('folder.')) return 'folders';
    if (action.startsWith('approval.')) return 'approvals';
    if (action.startsWith('auth.')) return 'auth';
    if (action.startsWith('session.')) return 'sessions';
    if (action.startsWith('accounts.')) return 'accounts';
    if (action.startsWith('groups.')) return 'groups';
    if (action.startsWith('incidents.') || action.startsWith('incident.')) return 'incidents';
    if (action.startsWith('task.')) return 'tasks';
    if (action.startsWith('control.')) return 'controls';
    if (action.startsWith('monitoring.')) return 'monitoring';
    if (action.startsWith('reports.') || action.startsWith('report.')) return 'reports';
    if (action.startsWith('backups.')) return 'backups';
    if (action.startsWith('settings.')) return 'settings';
    if (['create_user', 'delete_user', 'copy_user', 'import_users'].includes(action)) return 'accounts';
    return 'other';
  }

  const SECTION_ORDER = [
    'docs',
    'folders',
    'approvals',
    'accounts',
    'groups',
    'auth',
    'sessions',
    'incidents',
    'tasks',
    'controls',
    'monitoring',
    'reports',
    'backups',
    'settings',
    'other',
  ];

  const ACTION_LABELS = {
    ru: {
      'login_success': 'Авторизация: успешный вход',
      'auth.login_success': 'Авторизация: успешный вход',
      'auth.logout': 'Авторизация: выход',
      'auth.login_failed': 'Авторизация: неудачный вход',
      'auth.login_blocked': 'Авторизация: вход заблокирован',
      'auth.lockout': 'Авторизация: блокировка',
      'auth.password_changed': 'Авторизация: смена пароля',
      'auth.password_reset': 'Авторизация: сброс пароля',
      'auth.password_reuse_denied': 'Авторизация: повтор пароля запрещен',
      'auth.lock_manual': 'Авторизация: блокировка вручную',
      'auth.unlock': 'Авторизация: разблокировка',
      'session.kill': 'Сессия: завершение',
      'session.kill_all': 'Сессии: завершение всех',
      'doc.list': 'Документы: просмотр списка',
      'doc.view': 'Документы: просмотр',
      'doc.create': 'Документы: создание',
      'doc.edit': 'Документы: редактирование',
      'doc.delete': 'Документы: удаление',
      'doc.versions.view': 'Документы: просмотр версий',
      'doc.restore': 'Документы: восстановление версии',
      'doc.import': 'Документы: импорт',
      'doc.export': 'Документы: экспорт',
      'doc.export.blocked_policy': 'Документы: экспорт заблокирован политикой',
      'doc.export.approval.used': 'Документы: согласование экспорта использовано',
      'doc.export.approval.granted': 'Документы: согласование экспорта выдано',
      'doc.acl.view': 'Документы: просмотр прав',
      'doc.acl.update': 'Документы: изменение прав',
      'doc.classification.change': 'Документы: смена грифа',
      'doc.security.copy_blocked': 'Документы: блокировка копирования',
      'doc.security.screenshot_attempt': 'Документы: попытка скриншота',
      'folder.list': 'Папки: просмотр списка',
      'folder.create': 'Папки: создание',
      'folder.update': 'Папки: обновление',
      'folder.delete': 'Папки: удаление',
      'template.list': 'Шаблоны: список',
      'template.save': 'Шаблоны: сохранение',
      'template.delete': 'Шаблоны: удаление',
      'approval.start': 'Согласование: отправка',
      'approval.view': 'Согласование: просмотр карточки',
      'approval.approve': 'Согласование: утверждение',
      'approval.reject': 'Согласование: отклонение',
      'approval.comment': 'Согласование: комментарий',
      'approval.comments.view': 'Согласование: просмотр комментариев',
      'approval.cleanup': 'Согласование: очистка',
      'accounts.import_start': 'Учетные записи: импорт запущен',
      'accounts.import_commit': 'Учетные записи: импорт завершен',
      'accounts.import_row_failed': 'Учетные записи: ошибка импорта',
      'accounts.user_created_via_import': 'Учетные записи: создан пользователь (импорт)',
      'accounts.self_lockout_blocked': 'Учетные записи: предотвращена блокировка пользователя',
      'accounts.last_superadmin_blocked': 'Учетные записи: защищен последний администратор',
      'accounts.roles_changed': 'Учетные записи: роли изменены',
      'accounts.groups_changed': 'Учетные записи: группы изменены',
      'accounts.clearance_changed': 'Учетные записи: допуск изменен',
      'accounts.status_changed': 'Учетные записи: статус изменен',
      'accounts.bulk.assign_role': 'Учетные записи: массовое назначение роли',
      'accounts.bulk.assign_group': 'Учетные записи: массовое назначение группы',
      'accounts.bulk.reset_password': 'Учетные записи: массовый сброс пароля',
      'accounts.bulk.lock': 'Учетные записи: массовая блокировка',
      'accounts.bulk.unlock': 'Учетные записи: массовая разблокировка',
      'accounts.bulk.enable': 'Учетные записи: массовое включение',
      'accounts.bulk.disable': 'Учетные записи: массовое отключение',
      'accounts.bulk_action': 'Учетные записи: массовое действие',
      'accounts.role_create': 'Учетные записи: роль создана',
      'accounts.role_create_from_template': 'Учетные записи: роль из шаблона',
      'accounts.role_update': 'Учетные записи: роль обновлена',
      'accounts.role_delete': 'Учетные записи: роль удалена',
      'accounts.import_forbidden': 'Учетные записи: импорт запрещен',
      'accounts.legacy_import_blocked': 'Учетные записи: старый импорт заблокирован',
      'create_user': 'Учетные записи: создание пользователя',
      'delete_user': 'Учетные записи: удаление пользователя',
      'copy_user': 'Учетные записи: копирование пользователя',
      'import_users': 'Учетные записи: импорт пользователей',
      'groups.create': 'Группы: создание',
      'groups.update': 'Группы: обновление',
      'groups.delete': 'Группы: удаление',
      'groups.roles_changed': 'Группы: роли изменены',
      'groups.clearance_changed': 'Группы: допуск изменен',
      'groups.menu_changed': 'Группы: доступ к меню изменен',
      'groups.member_add': 'Группы: добавление участника',
      'groups.member_remove': 'Группы: удаление участника',
      'dashboard.layout.save': 'Дашборд: сохранение раскладки',
      'incident.create': 'Инциденты: создание',
      'incident.view': 'Инциденты: просмотр',
      'incident.update': 'Инциденты: обновление',
      'incident.close': 'Инциденты: закрытие',
      'incident.delete': 'Инциденты: удаление',
      'incident.restore': 'Инциденты: восстановление',
      'incident.participants.update': 'Инциденты: участники обновлены',
      'incident.assignee.change': 'Инциденты: исполнитель изменен',
      'incident.owner.change': 'Инциденты: владелец изменен',
      'incident.status.change': 'Инциденты: статус изменен',
      'incident.severity.change': 'Инциденты: критичность изменена',
      'incident.classification.change': 'Инциденты: гриф изменен',
      'incidents.closed': 'Инциденты: закрытие',
      'incident.stage.add': 'Инциденты: этап добавлен',
      'incident.stage.rename': 'Инциденты: этап переименован',
      'incident.stage.reorder': 'Инциденты: этап перемещен',
      'incident.stage.delete': 'Инциденты: этап удален',
      'incidents.stage_completed': 'Инциденты: этап завершен',
      'incident.stage.content.update': 'Инциденты: содержимое этапа обновлено',
      'incident.acl.update': 'Инциденты: доступ обновлен',
      'incident.link.add': 'Инциденты: связь добавлена',
      'incident.link.remove': 'Инциденты: связь удалена',
      'incident.attachment.upload': 'Инциденты: файл вложен',
      'incident.attachment.download': 'Инциденты: файл скачан',
      'incident.attachment.delete': 'Инциденты: файл удален',
      'incident.artifact.upload': 'Инциденты: артефакт загружен',
      'incident.artifact.download': 'Инциденты: артефакт скачан',
      'incident.artifact.delete': 'Инциденты: артефакт удален',
      'incident.note.add': 'Инциденты: заметка добавлена',
      'incident.export': 'Инциденты: экспорт',
      'incident.report.create': 'Инциденты: отчет создан',
      'report.build': 'Отчеты: сборка',
      'report.list': 'Отчеты: список',
      'report.view': 'Отчеты: просмотр',
      'report.edit': 'Отчеты: редактирование',
      'report.create': 'Отчеты: создание',
      'report.snapshot.create': 'Отчеты: создание снапшота',
      'report.snapshot.view': 'Отчеты: просмотр снапшота',
      'report.snapshots.view': 'Отчеты: список снапшотов',
      'report.sections.update': 'Отчеты: секции обновлены',
      'report.sections.view': 'Отчеты: просмотр секций',
      'report.create_from_incident': 'Отчеты: из инцидента',
      'report.settings.view': 'Отчеты: настройки',
      'report.settings.update': 'Отчеты: настройки обновлены',
      'report.template.list': 'Отчеты: шаблоны',
      'report.template.create': 'Отчеты: шаблон создан',
      'report.template.update': 'Отчеты: шаблон обновлен',
      'report.template.delete': 'Отчеты: шаблон удален',
      'report.template.use': 'Отчеты: применен шаблон',
      'report.update_meta': 'Отчеты: параметры обновлены',
      'report.delete': 'Отчеты: удаление',
      'report.export': 'Отчеты: экспорт',
      'report.export.with_charts': 'Отчеты: экспорт с графиками',
      'report.charts.list': 'Отчеты: графики',
      'report.charts.update': 'Отчеты: обновление графиков',
      'report.charts.render': 'Отчеты: построение графиков',
      'task.space.create': 'Задачи: создание пространства',
      'task.space.update': 'Задачи: обновление пространства',
      'task.space.delete': 'Задачи: удаление пространства',
      'task.board.create': 'Задачи: создание доски',
      'task.board.update': 'Задачи: обновление доски',
      'task.board.move': 'Задачи: перемещение доски',
      'task.board.delete': 'Задачи: удаление доски',
      'task.column.create': 'Задачи: создание колонки',
      'task.column.update': 'Задачи: обновление колонки',
      'task.column.delete': 'Задачи: удаление колонки',
      'task.column.move': 'Задачи: перемещение колонки',
      'task.subcolumn.create': 'Задачи: создание подколонки',
      'task.subcolumn.update': 'Задачи: обновление подколонки',
      'task.subcolumn.delete': 'Задачи: удаление подколонки',
      'task.subcolumn.move': 'Задачи: перемещение подколонки',
      'task.create': 'Задачи: создание задачи',
      'task.update': 'Задачи: обновление задачи',
      'task.assign': 'Задачи: назначение исполнителей',
      'task.move': 'Задачи: перемещение задачи',
      'task.relocate': 'Задачи: перенос задачи',
      'task.clone': 'Задачи: клонирование задачи',
      'task.delete': 'Задачи: удаление задачи',
      'task.close': 'Задачи: закрытие задачи',
      'task.archive': 'Задачи: архивирование задачи',
      'task.comment.add': 'Задачи: комментарий',
      'task.comment.update': 'Задачи: комментарий обновлен',
      'task.comment.delete': 'Задачи: комментарий удален',
      'task.comment.file.delete': 'Задачи: файл комментария удален',
      'task.link.add': 'Задачи: связь добавлена',
      'task.link.remove': 'Задачи: связь удалена',
      'task.link.pair.add': 'Задачи: связи карточек добавлены',
      'task.link.pair.remove': 'Задачи: связи карточек удалены',
      'task.template.create': 'Задачи: шаблон создан',
      'task.template.update': 'Задачи: шаблон обновлен',
      'task.template.delete': 'Задачи: шаблон удален',
      'task.recurring.create': 'Задачи: правило создано',
      'task.recurring.update': 'Задачи: правило обновлено',
      'task.recurring.toggle': 'Задачи: правило включение/выключение',
      'task.recurring.run_now': 'Задачи: запуск правила',
      'task.recurring.task_create': 'Задачи: задача по расписанию',
      'task.file.add': 'Задачи: файл добавлен',
      'task.file.delete': 'Задачи: файл удален',
      'task.field.clear': 'Задачи: поле очищено',
      'control.create': 'Контроли: создание',
      'control.update': 'Контроли: обновление',
      'control.delete': 'Контроли: удаление',
      'control.type.create': 'Контроли: тип создан',
      'control.type.delete': 'Контроли: тип удален',
      'control.comment.add': 'Контроли: комментарий',
      'control.comment.update': 'Контроли: комментарий обновлен',
      'control.comment.delete': 'Контроли: комментарий удален',
      'control.comment.file.delete': 'Контроли: файл комментария удален',
      'control.check.create': 'Контроли: проверка создана',
      'control.check.delete': 'Контроли: проверка удалена',
      'control.violation.create': 'Контроли: нарушение создано',
      'control.violation.delete': 'Контроли: нарушение удалено',
      'control.framework.create': 'Контроли: матрица создана',
      'control.framework.item.create': 'Контроли: требование создано',
      'control.framework.map.create': 'Контроли: требование связано',
      'control.link.add': 'Контроли: связь добавлена',
      'control.link.remove': 'Контроли: связь удалена',
      'link.violates.add': 'Контроли: нарушение связано',
      'link.violates.remove': 'Контроли: связь нарушения удалена',
      'violation.auto_create': 'Контроли: нарушение создано автоматически',
      'violation.auto_disable': 'Контроли: нарушение отключено автоматически',
      'task.block.create_text': 'Задачи: блокировка (текст)',
      'task.block.create_task': 'Задачи: блокировка (задача)',
      'task.block.resolve_manual': 'Задачи: разблокировка вручную',
      'task.block.resolve_auto': 'Задачи: разблокировка автоматически',
      'task.move.denied_blocked_final': 'Задачи: перенос запрещен (заблокирована)',
      'task.close.denied_blocked': 'Задачи: закрытие запрещено (заблокирована)',
      'monitoring.monitor.create': 'Мониторинг: создание монитора',
      'monitoring.monitor.update': 'Мониторинг: обновление монитора',
      'monitoring.monitor.delete': 'Мониторинг: удаление монитора',
      'monitoring.monitor.pause': 'Мониторинг: пауза',
      'monitoring.monitor.resume': 'Мониторинг: возобновление',
      'monitoring.monitor.clone': 'Мониторинг: копирование',
      'monitoring.monitor.check_now': 'Мониторинг: ручная проверка',
      'monitoring.settings.update': 'Мониторинг: обновление настроек',
      'monitoring.maintenance.create': 'Мониторинг: создание окна обслуживания',
      'monitoring.maintenance.update': 'Мониторинг: обновление окна обслуживания',
      'monitoring.maintenance.stop': 'Мониторинг: остановка окна обслуживания',
      'monitoring.maintenance.delete': 'Мониторинг: удаление окна обслуживания',
      'monitoring.certs.settings.update': 'Мониторинг: настройки сертификатов',
      'monitoring.sla.update': 'Мониторинг: обновление SLA',
      'monitoring.sla.policy.update': 'Мониторинг: обновление SLA-политики',
      'monitoring.notification.create': 'Мониторинг: уведомление создано',
      'monitoring.notification.update': 'Мониторинг: уведомление обновлено',
      'monitoring.notification.delete': 'Мониторинг: уведомление удалено',
      'monitoring.notification.test': 'Мониторинг: тест уведомления',
      'monitoring.notification.channel.create': 'Мониторинг: создание канала уведомлений',
      'monitoring.notification.channel.update': 'Мониторинг: обновление канала уведомлений',
      'monitoring.notification.channel.delete': 'Мониторинг: удаление канала уведомлений',
      'monitoring.notification.channel.test': 'Мониторинг: тест канала уведомлений',
      'monitoring.notification.channel.apply_all': 'Мониторинг: канал применен ко всем мониторам',
      'monitoring.notification.bindings.update': 'Мониторинг: привязки уведомлений обновлены',
      'monitoring.monitor.push': 'Мониторинг: push-событие',
      'monitoring.monitor.events.delete': 'Мониторинг: очистка событий монитора',
      'monitoring.monitor.metrics.delete': 'Мониторинг: очистка метрик монитора',
      'monitoring.certs.notify_test': 'Мониторинг: тест сертификатов',
      'monitoring.certs.notify_test.failed': 'Мониторинг: тест сертификатов завершился ошибкой',
      'monitoring.incident.auto_create': 'Мониторинг: авто-создание инцидента',
      'monitoring.incident.auto_close': 'Мониторинг: авто-закрытие инцидента',
      'backups.list': 'Бэкапы: список',
      'backups.read': 'Бэкапы: просмотр',
      'backups.create.requested': 'Бэкапы: создание запрошено',
      'backups.create.success': 'Бэкапы: создание завершено',
      'backups.create.failed': 'Бэкапы: создание завершилось ошибкой',
      'backups.import.requested': 'Бэкапы: импорт запрошен',
      'backups.import.success': 'Бэкапы: импорт завершен',
      'backups.import.failed': 'Бэкапы: импорт завершился ошибкой',
      'backups.download.requested': 'Бэкапы: скачивание запрошено',
      'backups.download.success': 'Бэкапы: скачивание завершено',
      'backups.download.failed': 'Бэкапы: скачивание завершилось ошибкой',
      'backups.delete': 'Бэкапы: удаление',
      'backups.restore.requested': 'Бэкапы: восстановление запрошено',
      'backups.restore.start': 'Бэкапы: восстановление запущено',
      'backups.restore.dry_run.requested': 'Бэкапы: dry-run запрошен',
      'backups.restore.read': 'Бэкапы: статус восстановления',
      'backups.restore.success': 'Бэкапы: восстановление завершено',
      'backups.restore.failed': 'Бэкапы: восстановление завершилось ошибкой',
      'backups.maintenance.enter': 'Бэкапы: вход в режим обслуживания',
      'backups.maintenance.exit': 'Бэкапы: выход из режима обслуживания',
      'backups.plan.read': 'Бэкапы: просмотр расписания',
      'backups.plan.update': 'Бэкапы: обновление расписания',
      'backups.plan.enable': 'Бэкапы: расписание включено',
      'backups.plan.disable': 'Бэкапы: расписание отключено',
      'backups.auto.started': 'Бэкапы: автозапуск начат',
      'backups.auto.success': 'Бэкапы: автозапуск успешен',
      'backups.auto.failed': 'Бэкапы: автозапуск с ошибкой',
      'backups.retention.deleted': 'Бэкапы: удаление по ретенции',
      'settings.updates.check': 'Настройки: проверка обновлений',
      'settings.updates.toggle': 'Настройки: автопроверка обновлений',
      'settings.hardening.read': 'Настройки: hardening-проверка',
    },
    en: {
      'login_success': 'Authentication: login success',
      'auth.login_success': 'Authentication: login success',
      'auth.logout': 'Authentication: logout',
      'auth.login_failed': 'Authentication: login failed',
      'auth.login_blocked': 'Authentication: login blocked',
      'auth.lockout': 'Authentication: lockout',
      'auth.password_changed': 'Authentication: password changed',
      'auth.password_reset': 'Authentication: password reset',
      'auth.password_reuse_denied': 'Authentication: password reuse denied',
      'auth.lock_manual': 'Authentication: manual lock',
      'auth.unlock': 'Authentication: unlock',
      'session.kill': 'Session: terminated',
      'session.kill_all': 'Sessions: terminated all',
      'doc.list': 'Documents: list view',
      'doc.view': 'Documents: view',
      'doc.create': 'Documents: create',
      'doc.edit': 'Documents: edit',
      'doc.delete': 'Documents: delete',
      'doc.versions.view': 'Documents: versions view',
      'doc.restore': 'Documents: restore version',
      'doc.import': 'Documents: import',
      'doc.export': 'Documents: export',
      'doc.export.blocked_policy': 'Documents: export blocked by policy',
      'doc.export.approval.used': 'Documents: export approval consumed',
      'doc.export.approval.granted': 'Documents: export approval granted',
      'doc.acl.view': 'Documents: permissions view',
      'doc.acl.update': 'Documents: permissions update',
      'doc.classification.change': 'Documents: classification change',
      'doc.security.copy_blocked': 'Documents: copy blocked',
      'doc.security.screenshot_attempt': 'Documents: screenshot attempt',
      'folder.list': 'Folders: list view',
      'folder.create': 'Folders: create',
      'folder.update': 'Folders: update',
      'folder.delete': 'Folders: delete',
      'template.list': 'Templates: list',
      'template.save': 'Templates: save',
      'template.delete': 'Templates: delete',
      'approval.start': 'Approvals: start',
      'approval.view': 'Approvals: view card',
      'approval.approve': 'Approvals: approve',
      'approval.reject': 'Approvals: reject',
      'approval.comment': 'Approvals: comment',
      'approval.comments.view': 'Approvals: view comments',
      'approval.cleanup': 'Approvals: cleanup',
      'accounts.import_start': 'Accounts: import started',
      'accounts.import_commit': 'Accounts: import completed',
      'accounts.import_row_failed': 'Accounts: import row failed',
      'accounts.user_created_via_import': 'Accounts: user created (import)',
      'accounts.self_lockout_blocked': 'Accounts: self lockout prevented',
      'accounts.last_superadmin_blocked': 'Accounts: last admin protected',
      'accounts.roles_changed': 'Accounts: roles changed',
      'accounts.groups_changed': 'Accounts: groups changed',
      'accounts.clearance_changed': 'Accounts: clearance changed',
      'accounts.status_changed': 'Accounts: status changed',
      'accounts.bulk.assign_role': 'Accounts: bulk role assignment',
      'accounts.bulk.assign_group': 'Accounts: bulk group assignment',
      'accounts.bulk.reset_password': 'Accounts: bulk password reset',
      'accounts.bulk.lock': 'Accounts: bulk lock',
      'accounts.bulk.unlock': 'Accounts: bulk unlock',
      'accounts.bulk.enable': 'Accounts: bulk enable',
      'accounts.bulk.disable': 'Accounts: bulk disable',
      'accounts.bulk_action': 'Accounts: bulk action',
      'accounts.role_create': 'Accounts: role created',
      'accounts.role_create_from_template': 'Accounts: role created from template',
      'accounts.role_update': 'Accounts: role updated',
      'accounts.role_delete': 'Accounts: role deleted',
      'accounts.import_forbidden': 'Accounts: import forbidden',
      'accounts.legacy_import_blocked': 'Accounts: legacy import blocked',
      'create_user': 'Accounts: create user',
      'delete_user': 'Accounts: delete user',
      'copy_user': 'Accounts: copy user',
      'import_users': 'Accounts: import users',
      'groups.create': 'Groups: create',
      'groups.update': 'Groups: update',
      'groups.delete': 'Groups: delete',
      'groups.roles_changed': 'Groups: roles changed',
      'groups.clearance_changed': 'Groups: clearance changed',
      'groups.menu_changed': 'Groups: menu access changed',
      'groups.member_add': 'Groups: add member',
      'groups.member_remove': 'Groups: remove member',
      'dashboard.layout.save': 'Dashboard: layout saved',
      'incident.create': 'Incidents: create',
      'incident.view': 'Incidents: view',
      'incident.update': 'Incidents: update',
      'incident.close': 'Incidents: close',
      'incident.delete': 'Incidents: delete',
      'incident.restore': 'Incidents: restore',
      'incident.participants.update': 'Incidents: participants updated',
      'incident.assignee.change': 'Incidents: assignee changed',
      'incident.owner.change': 'Incidents: owner changed',
      'incident.status.change': 'Incidents: status changed',
      'incident.severity.change': 'Incidents: severity changed',
      'incident.classification.change': 'Incidents: classification changed',
      'incidents.closed': 'Incidents: closed',
      'incident.stage.add': 'Incidents: stage added',
      'incident.stage.rename': 'Incidents: stage renamed',
      'incident.stage.reorder': 'Incidents: stage reordered',
      'incident.stage.delete': 'Incidents: stage deleted',
      'incidents.stage_completed': 'Incidents: stage completed',
      'incident.stage.content.update': 'Incidents: stage content updated',
      'incident.acl.update': 'Incidents: permissions updated',
      'incident.link.add': 'Incidents: link added',
      'incident.link.remove': 'Incidents: link removed',
      'incident.attachment.upload': 'Incidents: attachment uploaded',
      'incident.attachment.download': 'Incidents: attachment downloaded',
      'incident.attachment.delete': 'Incidents: attachment deleted',
      'incident.artifact.upload': 'Incidents: artifact uploaded',
      'incident.artifact.download': 'Incidents: artifact downloaded',
      'incident.artifact.delete': 'Incidents: artifact deleted',
      'incident.note.add': 'Incidents: note added',
      'incident.export': 'Incidents: export',
      'incident.report.create': 'Incidents: report created',
      'report.build': 'Reports: build',
      'report.list': 'Reports: list view',
      'report.view': 'Reports: view',
      'report.edit': 'Reports: edit',
      'report.create': 'Reports: create',
      'report.snapshot.create': 'Reports: snapshot created',
      'report.snapshot.view': 'Reports: snapshot viewed',
      'report.snapshots.view': 'Reports: snapshots list',
      'report.sections.update': 'Reports: sections updated',
      'report.sections.view': 'Reports: sections viewed',
      'report.create_from_incident': 'Reports: from incident',
      'report.settings.view': 'Reports: settings view',
      'report.settings.update': 'Reports: settings updated',
      'report.template.list': 'Reports: templates list',
      'report.template.create': 'Reports: template created',
      'report.template.update': 'Reports: template updated',
      'report.template.delete': 'Reports: template deleted',
      'report.template.use': 'Reports: template applied',
      'report.update_meta': 'Reports: meta updated',
      'report.delete': 'Reports: delete',
      'report.export': 'Reports: export',
      'report.export.with_charts': 'Reports: export with charts',
      'report.charts.list': 'Reports: charts list',
      'report.charts.update': 'Reports: charts updated',
      'report.charts.render': 'Reports: charts rendered',
      'task.space.create': 'Tasks: space created',
      'task.space.update': 'Tasks: space updated',
      'task.space.delete': 'Tasks: space deleted',
      'task.board.create': 'Tasks: board created',
      'task.board.update': 'Tasks: board updated',
      'task.board.move': 'Tasks: board moved',
      'task.board.delete': 'Tasks: board deleted',
      'task.column.create': 'Tasks: column created',
      'task.column.update': 'Tasks: column updated',
      'task.column.delete': 'Tasks: column deleted',
      'task.column.move': 'Tasks: column moved',
      'task.subcolumn.create': 'Tasks: subcolumn created',
      'task.subcolumn.update': 'Tasks: subcolumn updated',
      'task.subcolumn.delete': 'Tasks: subcolumn deleted',
      'task.subcolumn.move': 'Tasks: subcolumn moved',
      'task.create': 'Tasks: task created',
      'task.update': 'Tasks: task updated',
      'task.assign': 'Tasks: assignees updated',
      'task.move': 'Tasks: task moved',
      'task.relocate': 'Tasks: task relocated',
      'task.clone': 'Tasks: task cloned',
      'task.delete': 'Tasks: task deleted',
      'task.close': 'Tasks: task closed',
      'task.archive': 'Tasks: task archived',
      'task.comment.add': 'Tasks: comment added',
      'task.comment.update': 'Tasks: comment updated',
      'task.comment.delete': 'Tasks: comment deleted',
      'task.comment.file.delete': 'Tasks: comment file deleted',
      'task.link.add': 'Tasks: link added',
      'task.link.remove': 'Tasks: link removed',
      'task.link.pair.add': 'Tasks: card relations added',
      'task.link.pair.remove': 'Tasks: card relations removed',
      'task.template.create': 'Tasks: template created',
      'task.template.update': 'Tasks: template updated',
      'task.template.delete': 'Tasks: template deleted',
      'task.recurring.create': 'Tasks: recurring rule created',
      'task.recurring.update': 'Tasks: recurring rule updated',
      'task.recurring.toggle': 'Tasks: recurring rule toggled',
      'task.recurring.run_now': 'Tasks: recurring run now',
      'task.recurring.task_create': 'Tasks: task from schedule',
      'task.file.add': 'Tasks: file added',
      'task.file.delete': 'Tasks: file deleted',
      'task.field.clear': 'Tasks: field cleared',
      'control.create': 'Controls: created',
      'control.update': 'Controls: updated',
      'control.delete': 'Controls: deleted',
      'control.type.create': 'Controls: type created',
      'control.type.delete': 'Controls: type deleted',
      'control.comment.add': 'Controls: comment added',
      'control.comment.update': 'Controls: comment updated',
      'control.comment.delete': 'Controls: comment deleted',
      'control.comment.file.delete': 'Controls: comment file deleted',
      'control.check.create': 'Controls: check created',
      'control.check.delete': 'Controls: check deleted',
      'control.violation.create': 'Controls: violation created',
      'control.violation.delete': 'Controls: violation deleted',
      'control.framework.create': 'Controls: framework created',
      'control.framework.item.create': 'Controls: requirement created',
      'control.framework.map.create': 'Controls: requirement mapped',
      'control.link.add': 'Controls: link added',
      'control.link.remove': 'Controls: link removed',
      'link.violates.add': 'Controls: violation linked',
      'link.violates.remove': 'Controls: violation link removed',
      'violation.auto_create': 'Controls: violation auto-created',
      'violation.auto_disable': 'Controls: violation auto-disabled',
      'task.block.create_text': 'Tasks: block (text)',
      'task.block.create_task': 'Tasks: block (task)',
      'task.block.resolve_manual': 'Tasks: unblock manually',
      'task.block.resolve_auto': 'Tasks: unblock automatically',
      'task.move.denied_blocked_final': 'Tasks: move denied (blocked)',
      'task.close.denied_blocked': 'Tasks: close denied (blocked)',
      'monitoring.monitor.create': 'Monitoring: monitor created',
      'monitoring.monitor.update': 'Monitoring: monitor updated',
      'monitoring.monitor.delete': 'Monitoring: monitor deleted',
      'monitoring.monitor.pause': 'Monitoring: paused',
      'monitoring.monitor.resume': 'Monitoring: resumed',
      'monitoring.monitor.clone': 'Monitoring: cloned',
      'monitoring.monitor.check_now': 'Monitoring: manual check',
      'monitoring.settings.update': 'Monitoring: settings updated',
      'monitoring.maintenance.create': 'Monitoring: maintenance window created',
      'monitoring.maintenance.update': 'Monitoring: maintenance window updated',
      'monitoring.maintenance.stop': 'Monitoring: maintenance window stopped',
      'monitoring.maintenance.delete': 'Monitoring: maintenance window deleted',
      'monitoring.certs.settings.update': 'Monitoring: certificate settings updated',
      'monitoring.sla.update': 'Monitoring: SLA updated',
      'monitoring.sla.policy.update': 'Monitoring: SLA policy updated',
      'monitoring.notification.create': 'Monitoring: notification created',
      'monitoring.notification.update': 'Monitoring: notification updated',
      'monitoring.notification.delete': 'Monitoring: notification deleted',
      'monitoring.notification.test': 'Monitoring: notification test',
      'monitoring.notification.channel.create': 'Monitoring: notification channel created',
      'monitoring.notification.channel.update': 'Monitoring: notification channel updated',
      'monitoring.notification.channel.delete': 'Monitoring: notification channel deleted',
      'monitoring.notification.channel.test': 'Monitoring: notification channel test',
      'monitoring.notification.channel.apply_all': 'Monitoring: channel applied to all monitors',
      'monitoring.notification.bindings.update': 'Monitoring: notification bindings updated',
      'monitoring.monitor.push': 'Monitoring: push event',
      'monitoring.monitor.events.delete': 'Monitoring: monitor events cleared',
      'monitoring.monitor.metrics.delete': 'Monitoring: monitor metrics cleared',
      'monitoring.certs.notify_test': 'Monitoring: certificates test',
      'monitoring.certs.notify_test.failed': 'Monitoring: certificates test failed',
      'monitoring.incident.auto_create': 'Monitoring: incident auto-created',
      'monitoring.incident.auto_close': 'Monitoring: incident auto-closed',
      'backups.list': 'Backups: list',
      'backups.read': 'Backups: read',
      'backups.create.requested': 'Backups: create requested',
      'backups.create.success': 'Backups: create succeeded',
      'backups.create.failed': 'Backups: create failed',
      'backups.import.requested': 'Backups: import requested',
      'backups.import.success': 'Backups: import succeeded',
      'backups.import.failed': 'Backups: import failed',
      'backups.download.requested': 'Backups: download requested',
      'backups.download.success': 'Backups: download succeeded',
      'backups.download.failed': 'Backups: download failed',
      'backups.delete': 'Backups: delete',
      'backups.restore.requested': 'Backups: restore requested',
      'backups.restore.start': 'Backups: restore started',
      'backups.restore.dry_run.requested': 'Backups: dry-run requested',
      'backups.restore.read': 'Backups: restore status read',
      'backups.restore.success': 'Backups: restore succeeded',
      'backups.restore.failed': 'Backups: restore failed',
      'backups.maintenance.enter': 'Backups: enter maintenance mode',
      'backups.maintenance.exit': 'Backups: exit maintenance mode',
      'backups.plan.read': 'Backups: plan read',
      'backups.plan.update': 'Backups: plan updated',
      'backups.plan.enable': 'Backups: plan enabled',
      'backups.plan.disable': 'Backups: plan disabled',
      'backups.auto.started': 'Backups: auto backup started',
      'backups.auto.success': 'Backups: auto backup succeeded',
      'backups.auto.failed': 'Backups: auto backup failed',
      'backups.retention.deleted': 'Backups: retention deleted',
      'settings.updates.check': 'Settings: update check',
      'settings.updates.toggle': 'Settings: update checks toggled',
      'settings.hardening.read': 'Settings: hardening read',
    },
  };

  const DETAIL_LABELS = {
    ru: {
      document: 'Документ',
      approval: 'Согласование',
      monitoring: {
        id: 'ID',
        channel: 'Канал',
        applied: 'Применено',
        retention: 'Хранение (дни)',
        max_concurrent: 'Параллельные проверки',
        default_timeout: 'Таймаут по умолчанию',
        default_interval: 'Интервал по умолчанию',
        default_retries: 'Повторы по умолчанию',
        default_retry_interval: 'Интервал повторов',
        default_sla: 'SLA по умолчанию',
        engine: 'Движок включен',
        allow_private: 'Разрешены приватные сети',
        tls_refresh: 'Обновление TLS (часы)',
        tls_expiring: 'Порог TLS (дни)',
        notify_suppress: 'Подавление уведомлений (мин)',
        notify_repeat: 'Повтор down-уведомлений (мин)',
        notify_maintenance: 'Уведомлять об обслуживании',
        auto_incident_close_on_up: 'Автозакрытие инцидента при UP',
      },
      doc: {
        approval_required: 'требуется согласование экспорта',
      },
      backups: {
        result: 'Результат',
        code: 'Код',
        reason_code: 'Причина',
        event: 'Событие',
        backup_id: 'Бэкап',
        restore_id: 'Восстановление',
        resource: 'Ресурс',
        id: 'ID',
        filename: 'Файл',
        size: 'Размер',
        size_bytes: 'Размер',
        dry_run: 'Сухой прогон',
        resource_integrity: 'Целостность',
        result_requested: 'Запрошено',
        result_queued: 'В очереди',
        result_success: 'Успешно',
        result_failed: 'Ошибка',
        result_denied: 'Запрещено',
        result_not_found: 'Не найдено',
        result_not_implemented: 'Не реализовано',
      },
      'invalid password': 'неверный пароль',
      'user missing or inactive': 'пользователь не найден или отключен',
      permanent: 'постоянная блокировка',
      updates: {
        current: 'Текущая версия',
        latest: 'Последняя версия',
        has_update: 'Доступно обновление',
        source: 'Источник',
        true: 'да',
        false: 'нет',
      },
      settings: {
        score: 'Оценка',
      },
    },
    en: {
      document: 'Document',
      approval: 'Approval',
      monitoring: {
        id: 'ID',
        channel: 'Channel',
        applied: 'Applied',
        retention: 'Retention (days)',
        max_concurrent: 'Concurrent checks',
        default_timeout: 'Default timeout',
        default_interval: 'Default interval',
        default_retries: 'Default retries',
        default_retry_interval: 'Retry interval',
        default_sla: 'Default SLA',
        engine: 'Engine enabled',
        allow_private: 'Private networks allowed',
        tls_refresh: 'TLS refresh (hours)',
        tls_expiring: 'TLS expiring threshold (days)',
        notify_suppress: 'Suppress notifications (min)',
        notify_repeat: 'Repeat down notifications (min)',
        notify_maintenance: 'Notify maintenance',
        auto_incident_close_on_up: 'Auto-close incident on UP',
      },
      doc: {
        approval_required: 'export approval required',
      },
      backups: {
        result: 'Result',
        code: 'Code',
        reason_code: 'Reason',
        event: 'Event',
        backup_id: 'Backup',
        restore_id: 'Restore',
        resource: 'Resource',
        id: 'ID',
        filename: 'File',
        size: 'Size',
        size_bytes: 'Size',
        dry_run: 'Dry run',
        resource_integrity: 'Integrity',
        result_requested: 'Requested',
        result_queued: 'Queued',
        result_success: 'Success',
        result_failed: 'Failed',
        result_denied: 'Denied',
        result_not_found: 'Not found',
        result_not_implemented: 'Not implemented',
      },
      'invalid password': 'invalid password',
      'user missing or inactive': 'user missing or inactive',
      permanent: 'permanent lockout',
      updates: {
        current: 'Current version',
        latest: 'Latest version',
        has_update: 'Update available',
        source: 'Source',
        true: 'yes',
        false: 'no',
      },
      settings: {
        score: 'Score',
      },
    },
  };

  const NOISY_NUMERIC_DETAILS_ACTIONS = new Set([
    'report.list',
    'report.template.list',
    'report.settings.view',
    'doc.list',
    'folder.list',
  ]);

  return { init, prettyAction };
})();

if (typeof window !== 'undefined') {
  window.LogsPage = LogsPage;
}
