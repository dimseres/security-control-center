(async () => {
  let prefs = Preferences.load();
  let inactivityTimer;
  let autoLogoutHandler;
  let pingTimer;
  const MENU_ORDER = ['dashboard', 'tasks', 'controls', 'monitoring', 'docs', 'approvals', 'incidents', 'reports', 'accounts', 'settings', 'backups', 'logs'];
  const lang = prefs.language || localStorage.getItem('berkut_lang') || 'ru';
  await BerkutI18n.load(lang);
  BerkutI18n.apply();

  let me;
  let pendingDocsTab = null;
  let appMetaTimer;
  try {
    me = await Api.get('/api/auth/me');
  } catch (err) {
    window.location.href = '/login';
    return;
  }
  const user = me.user;
  if (!user.password_set || user.require_password_change) {
    window.location.href = '/password-change';
    return;
  }
  document.getElementById('user-info').textContent = user.username;
  await loadAppMeta();

  document.getElementById('logout-btn').addEventListener('click', async () => {
    if (typeof IncidentsPage !== 'undefined' && IncidentsPage.clearState) {
      IncidentsPage.clearState();
    }
    await Api.post('/api/auth/logout');
    window.location.href = '/login';
  });

  configureAutoLogout(prefs.autoLogout);
  startSessionPing();
  setupModalDismiss();

  const menuResp = await Api.get('/api/app/menu');
  migrateLegacyHash(menuResp.menu);
  let currentPage = pickInitialPage(menuResp.menu);
  renderMenu(menuResp.menu, currentPage);
  await navigateTo(currentPage, false);
  if (window.location.pathname === '/' || window.location.pathname === '/app') {
    window.history.replaceState({}, '', `/${currentPage}`);
  }

  window.addEventListener('popstate', async () => {
    const target = pathFromLocation(menuResp.menu) || currentPage;
    if (target !== currentPage) {
      const ok = await navigateTo(target, false);
      if (!ok) return;
    } else {
      setActiveLink(target);
      if (target === 'docs' && typeof DocsPage !== 'undefined' && DocsPage.switchTab) {
        if (pendingDocsTab) {
          const nextTab = pendingDocsTab;
          pendingDocsTab = null;
          DocsPage.switchTab(nextTab);
        }
      }
      if (target === 'monitoring' && typeof MonitoringPage !== 'undefined' && MonitoringPage.syncRouteTab) {
        MonitoringPage.syncRouteTab();
      }
    }
  });

  const switcher = document.getElementById('language-switcher');
  switcher.value = lang;
  switcher.addEventListener('change', async (e) => {
    const nextPrefs = Preferences.save({ ...prefs, language: e.target.value });
    await handlePreferencesChange(nextPrefs, menuResp.menu, currentPage);
  });

  function pickInitialPage(items) {
    const url = new URL(window.location.href);
    if (url.searchParams.get('incident')) {
      const hasIncidents = (items || []).some(i => i.path === 'incidents');
      if (hasIncidents) return 'incidents';
    }
    const fromPath = pathFromLocation(items);
    if (fromPath) return fromPath;
    const ordered = sortMenuItems(items);
    return ordered.length ? ordered[0].path : 'dashboard';
  }

  function pathFromLocation(items) {
    const path = window.location.pathname.replace(/\/+$/, '');
    const parts = path.split('/').filter(Boolean);
    const base = parts[0] || '';
    if (!base || base === 'app') return null;
    if (base === 'approvals') return 'approvals';
    if (base === 'docs') return 'docs';
    if (base === 'tasks') return 'tasks';
    if (base === 'incidents') return 'incidents';
    if (base === 'settings') return 'settings';
    if (base === 'accounts') return 'accounts';
    if (base === 'dashboard') return 'dashboard';
    if (base === 'controls') return 'controls';
    if (base === 'monitoring') return 'monitoring';
    if (base === 'backups') return 'backups';
    if (base === 'reports') return 'reports';
    if (base === 'findings') return 'findings';
    if (base === 'logs') return 'logs';
    return items.find(i => i.path === base)?.path || null;
  }

  function migrateLegacyHash(items) {
    const rawHash = window.location.hash.replace('#', '');
    if (!rawHash) return;
    let next = '';
    if (rawHash.startsWith('tasks/task/')) {
      const [, , id] = rawHash.split('/');
      next = id ? `/tasks/task/${id}` : '/tasks';
    } else if (rawHash.startsWith('tasks/space/')) {
      const [, , id] = rawHash.split('/');
      next = id ? `/tasks/space/${id}` : '/tasks';
    } else if (rawHash === 'tasks') {
      next = '/tasks';
    } else if (rawHash === 'docs') {
      next = '/docs';
    } else if (rawHash === 'approvals') {
      next = '/approvals';
    } else if (rawHash === 'docs/approvals') {
      next = '/docs/approvals';
    } else if (rawHash.startsWith('incident=')) {
      const id = rawHash.split('incident=')[1];
      next = id ? `/incidents/${id}` : '/incidents';
    } else if (rawHash.startsWith('settings/')) {
      const [, tab] = rawHash.split('/');
      next = tab ? `/settings/${tab}` : '/settings';
    } else if (rawHash.startsWith('backups/')) {
      const [, tab] = rawHash.split('/');
      next = tab ? `/backups/${tab}` : '/backups';
    } else if (rawHash === 'backups') {
      next = '/backups';
    } else if (rawHash) {
      const base = rawHash.split('/')[0];
      const known = items.find(i => i.path === base);
      if (known) next = `/${base}`;
    }
    if (next) {
      window.history.replaceState({}, '', next);
      window.location.hash = '';
    }
  }

  function sortMenuItems(items) {
    const originalOrder = new Map();
    (items || []).forEach((item, idx) => originalOrder.set(item, idx));
    return (items || []).slice().sort((a, b) => {
      const aIndex = MENU_ORDER.indexOf(a.path);
      const bIndex = MENU_ORDER.indexOf(b.path);
      const aScore = aIndex === -1 ? Number.MAX_SAFE_INTEGER : aIndex;
      const bScore = bIndex === -1 ? Number.MAX_SAFE_INTEGER : bIndex;
      if (aScore !== bScore) return aScore - bScore;
      return (originalOrder.get(a) ?? 0) - (originalOrder.get(b) ?? 0);
    });
  }

  async function navigateTo(path, updateHash = true) {
    if (currentPage === 'dashboard' && path !== 'dashboard') {
      if (typeof DashboardPage !== 'undefined' && DashboardPage.confirmNavigation) {
        const ok = await DashboardPage.confirmNavigation();
        if (!ok) {
          if (!updateHash) {
            window.history.replaceState({}, '', `/${currentPage}`);
          }
          return false;
        }
      }
    }
    if (currentPage === 'incidents' && path !== 'incidents') {
      clearIncidentQuery();
      if (typeof IncidentsPage !== 'undefined' && IncidentsPage.clearState) {
        IncidentsPage.clearState();
      }
    }
    currentPage = path;
    if (updateHash) {
      const nextPath = `/${path}`;
      if (window.location.pathname !== nextPath) {
        window.history.pushState({}, '', nextPath);
      }
    }
    setActiveLink(path);
    await loadPage(path);
    return true;
  }

  function renderMenu(items, activePath) {
    const nav = document.getElementById('menu');
    nav.innerHTML = '';
    sortMenuItems(items).forEach(item => {
      const link = document.createElement('a');
      link.className = 'sidebar-link';
      link.href = `/${item.path}`;
      link.textContent = BerkutI18n.t(`nav.${item.name}`) || item.name;
      link.dataset.path = item.path;
      if (item.path === activePath) {
        link.classList.add('active');
      }
      link.addEventListener('click', async (e) => {
        e.preventDefault();
        await navigateTo(item.path);
      });
      nav.appendChild(link);
    });
  }

  function setActiveLink(path) {
    document.querySelectorAll('.sidebar-link').forEach(link => {
      link.classList.toggle('active', link.dataset.path === path);
    });
  }

  async function loadPage(path) {
    document.body.classList.remove('dashboard-mode');
    const res = await fetch(`/api/page/${path}`, { credentials: 'include' });
    const area = document.getElementById('content-area');
    if (!res.ok) {
      area.textContent = BerkutI18n.t('common.accessDenied');
      return;
    }
    const html = await res.text();
    area.innerHTML = html;
    const titleEl = document.getElementById('page-title');
    const descEl = document.getElementById('page-desc');
    if (path === 'docs') {
      if (titleEl) titleEl.textContent = '';
      if (descEl) descEl.textContent = '';
    } else {
      if (titleEl) titleEl.textContent = BerkutI18n.t(`nav.${path}`) || path;
      if (descEl) descEl.textContent = descriptionFor(path);
    }
    BerkutI18n.apply();
    if (autoLogoutHandler) {
      autoLogoutHandler();
    }
    if (path === 'accounts' && typeof AccountsPage !== 'undefined') {
      AccountsPage.init();
    }
    if (path === 'docs' && typeof DocsPage !== 'undefined') {
      if (pendingDocsTab) {
        window.__docsPendingTab = pendingDocsTab;
        pendingDocsTab = null;
      }
      DocsPage.init();
    }
    if (path === 'approvals' && typeof ApprovalsPage !== 'undefined') {
      ApprovalsPage.init();
    }
    if (path === 'settings' && typeof SettingsPage !== 'undefined') {
      SettingsPage.init(async (next) => {
        const saved = Preferences.save(next);
        await handlePreferencesChange(saved, menuResp.menu, path);
      });
    }
    if (path === 'incidents' && typeof IncidentsPage !== 'undefined') {
      IncidentsPage.init();
    }
    if (path === 'tasks' && typeof TasksPage !== 'undefined') {
      TasksPage.init();
    }
    if (path === 'logs' && typeof LogsPage !== 'undefined') {
      LogsPage.init();
    }
    if (path === 'dashboard' && typeof DashboardPage !== 'undefined') {
      DashboardPage.init();
    }
    if (path === 'controls' && typeof ControlsPage !== 'undefined') {
      ControlsPage.init();
    }
    if (path === 'monitoring' && typeof MonitoringPage !== 'undefined') {
      MonitoringPage.init();
    }
    if (path === 'reports' && typeof ReportsPage !== 'undefined') {
      ReportsPage.init();
    }
    if (path === 'backups' && typeof BackupsPage !== 'undefined') {
      BackupsPage.init();
    }
  }

  async function handlePreferencesChange(nextPrefs, menu, currentPath) {
    prefs = nextPrefs;
    await BerkutI18n.load(prefs.language || 'ru');
    BerkutI18n.apply();
    renderMenu(menu, currentPath);
    setActiveLink(currentPath);
    const switcher = document.getElementById('language-switcher');
    if (switcher) switcher.value = prefs.language || 'ru';
    configureAutoLogout(prefs.autoLogout);
    await loadPage(currentPath);
  }

  function configureAutoLogout(enabled) {
    const events = ['click', 'keydown', 'mousemove', 'scroll'];
    events.forEach(evt => {
      if (autoLogoutHandler) {
        document.removeEventListener(evt, autoLogoutHandler);
      }
    });
    clearTimeout(inactivityTimer);
    if (!enabled) {
      autoLogoutHandler = null;
      return;
    }
    autoLogoutHandler = () => {
      clearTimeout(inactivityTimer);
      inactivityTimer = setTimeout(async () => {
        try {
          await Api.post('/api/auth/logout');
        } catch (err) {
          console.error('auto logout failed', err);
        } finally {
          window.location.href = '/login';
        }
      }, 15 * 60 * 1000);
    };
    events.forEach(evt => document.addEventListener(evt, autoLogoutHandler));
    autoLogoutHandler();
  }

  function startSessionPing() {
    clearInterval(pingTimer);
    const intervalMs = 45000;
    const ping = async () => {
      try {
        await Api.post('/api/app/ping');
      } catch (err) {
        console.debug('ping failed', err);
      }
    };
    pingTimer = setInterval(ping, intervalMs);
    ping();
  }

  async function loadAppMeta() {
    try {
      const meta = await Api.get('/api/app/meta');
      renderAppMeta(meta || {});
      if (meta?.update_checks_enabled) {
        if (!appMetaTimer) {
          appMetaTimer = setInterval(() => {
            loadAppMeta().catch(() => {});
          }, 30 * 60 * 1000);
        }
      } else if (appMetaTimer) {
        clearInterval(appMetaTimer);
        appMetaTimer = null;
      }
    } catch (_) {
      // ignore meta failures for normal navigation
    }
  }

  function renderAppMeta(meta) {
    const versionEl = document.getElementById('app-version');
    const badgeEl = document.getElementById('app-update-badge');
    if (versionEl) {
      const mode = meta?.is_home_mode ? 'HOME' : 'Enterprise';
      versionEl.textContent = `${BerkutI18n.t('settings.version')}: ${meta?.app_version || '-'} | ${mode}`;
    }
    if (!badgeEl) return;
    const update = meta?.update;
    if (update?.has_update) {
      badgeEl.hidden = false;
      badgeEl.textContent = `${BerkutI18n.t('settings.updates.available')}: ${update.latest_version || '-'}`;
    } else {
      badgeEl.hidden = true;
      badgeEl.textContent = '';
    }
  }

  function setupModalDismiss() {
    const closeModalWithHook = (modal) => {
      if (!modal) return;
      if (modal.id === 'task-modal' && typeof TasksPage !== 'undefined' && typeof TasksPage.closeTaskModal === 'function') {
        TasksPage.closeTaskModal();
        return;
      }
      modal.hidden = true;
    };
    document.addEventListener('keydown', (e) => {
      if (e.key !== 'Escape') return;
      const openModals = Array.from(document.querySelectorAll('.modal')).filter(m => !m.hidden);
      if (!openModals.length) return;
      closeModalWithHook(openModals[openModals.length - 1]);
    });
    document.addEventListener('click', (e) => {
      const backdrop = e.target.closest('.modal-backdrop');
      if (!backdrop) return;
      const modal = backdrop.closest('.modal');
      closeModalWithHook(modal);
    });
  }

  function clearIncidentQuery() {
    const url = new URL(window.location.href);
    if (!url.searchParams.has('incident')) return;
    url.searchParams.delete('incident');
    window.history.replaceState({}, '', url.toString());
  }

  function descriptionFor(path) {
    switch (path) {
      case 'dashboard':
        return BerkutI18n.t('dashboard.subtitle');
      case 'accounts':
        return BerkutI18n.t('accounts.subtitle');
      case 'docs':
        return BerkutI18n.t('docs.subtitle');
      case 'approvals':
        return BerkutI18n.t('approvals.subtitle');
      case 'incidents':
        return BerkutI18n.t('incidents.subtitle');
      case 'tasks':
        return BerkutI18n.t('tasks.subtitle');
      case 'controls':
        return BerkutI18n.t('controls.subtitle');
      case 'monitoring':
        return BerkutI18n.t('monitoring.subtitle');
      case 'logs':
        return BerkutI18n.t('logs.subtitle');
      case 'settings':
        return BerkutI18n.t('settings.subtitle');
      case 'reports':
        return BerkutI18n.t('reports.subtitle');
      case 'backups':
        return BerkutI18n.t('backups.subtitle');
      default:
        return BerkutI18n.t('placeholder.subtitle');
    }
  }
})();
