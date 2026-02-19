(() => {
  const els = {};
  const ui = { menu: '', bulk: false, selected: new Set() };
  const HOST_TARGET_TYPES = new Set(['tcp', 'ping', 'dns', 'docker', 'steam', 'gamedig', 'mqtt', 'kafka_producer', 'mssql', 'mysql', 'mongodb', 'radius', 'redis', 'tailscale_ping']);
  function bindList() {
    const page = document.getElementById('monitoring-page');
    if (!page) return;
    Object.assign(els, {
      page,
      search: document.getElementById('monitor-search'),
      status: document.getElementById('monitor-filter-status'),
      active: document.getElementById('monitor-filter-active'),
      tags: document.getElementById('monitor-filter-tags'),
      tagsHint: document.querySelector('[data-tag-hint="monitor-filter-tags"]'),
      list: document.getElementById('monitor-list'),
      newBtn: document.getElementById('monitor-new-btn'),
      statusBtn: document.getElementById('monitor-filter-status-btn'),
      activeBtn: document.getElementById('monitor-filter-active-btn'),
      tagsBtn: document.getElementById('monitor-filter-tags-btn'),
      statusMenu: document.getElementById('monitor-filter-status-menu'),
      activeMenu: document.getElementById('monitor-filter-active-menu'),
      tagsMenu: document.getElementById('monitor-filter-tags-menu'),
      bulkToggle: document.getElementById('monitor-bulk-toggle'),
      bulkActions: document.getElementById('monitor-bulk-actions'),
      bulkCount: document.getElementById('monitor-bulk-count'),
      bulkAll: document.getElementById('monitor-bulk-all'),
      bulkPause: document.getElementById('monitor-bulk-pause'),
      bulkResume: document.getElementById('monitor-bulk-resume'),
    });
    els.search?.addEventListener('input', debounce(() => {
      MonitoringPage.state.filters.q = els.search.value.trim();
      MonitoringPage.loadMonitors?.();
    }, 250));
    if (els.newBtn) {
      els.newBtn.addEventListener('click', () => MonitoringPage.openMonitorModal?.());
      const canManage = MonitoringPage.hasPermission('monitoring.manage');
      els.newBtn.disabled = !canManage;
      els.newBtn.classList.toggle('disabled', !canManage);
    }
    if (els.bulkToggle) {
      const label = MonitoringPage.t('monitoring.bulk.select');
      els.bulkToggle.title = label;
      els.bulkToggle.setAttribute('aria-label', label);
    }
    els.bulkToggle?.addEventListener('click', () => setBulkMode(!ui.bulk));
    els.bulkAll?.addEventListener('change', () => toggleAllBulk(!!els.bulkAll.checked));
    els.bulkPause?.addEventListener('click', () => bulkSetPaused(true));
    els.bulkResume?.addEventListener('click', () => bulkSetPaused(false));
    bindMenus();
    populateTagOptions();
    MonitoringPage.bindTagHint?.(els.tags, els.tagsHint);
    renderMenuLabels();
    syncBulkUI();
  }
  function bindMenus() {
    [els.statusBtn, els.activeBtn, els.tagsBtn].forEach((btn) => {
      if (!btn) return;
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        const id = btn.dataset.menuToggle || '';
        toggleMenu(id);
      });
    });
    document.addEventListener('click', closeMenus);
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') closeMenus();
    });
    els.statusMenu?.addEventListener('click', (e) => handleSingleMenuClick(e, els.status, renderStatusMenu));
    els.activeMenu?.addEventListener('click', (e) => handleSingleMenuClick(e, els.active, renderActiveMenu));
    els.tagsMenu?.addEventListener('click', (e) => e.stopPropagation());
    els.tagsMenu?.addEventListener('change', () => {
      syncFilters();
      renderMenuLabels();
      MonitoringPage.bindTagHint?.(els.tags, els.tagsHint);
      MonitoringPage.loadMonitors?.();
    });
    renderStatusMenu();
    renderActiveMenu();
    renderTagsMenu();
  }
  function handleSingleMenuClick(e, select, rerenderMenu) {
    const opt = e.target.closest('button[data-value]');
    if (!opt || !select) return;
    select.value = opt.dataset.value || '';
    syncFilters();
    renderMenuLabels();
    rerenderMenu();
    closeMenus();
    MonitoringPage.loadMonitors?.();
  }
  function toggleMenu(id) {
    if (!id) return;
    if (ui.menu === id) return closeMenus();
    closeMenus();
    const node = document.getElementById(id);
    if (!node) return;
    node.hidden = false;
    ui.menu = id;
  }
  function closeMenus() {
    [els.statusMenu, els.activeMenu, els.tagsMenu].forEach((menu) => {
      if (menu) menu.hidden = true;
    });
    ui.menu = '';
  }
  async function loadMonitors() {
    if (!MonitoringPage.hasPermission('monitoring.view')) return;
    try {
      const res = await Api.get(`/api/monitoring/monitors${buildQuery()}`);
      const items = Array.isArray(res.items) ? res.items : [];
      MonitoringPage.state.monitors = items;
      pruneBulkSelection(items);
      applyCardsVisibility(items.length > 0);
      populateTagOptions(items);
      MonitoringPage.refreshEventsFilters?.();
      MonitoringPage.refreshCertsNotifyList?.();
      MonitoringPage.refreshMaintenanceOptions?.();
      renderList(items);
      if (!MonitoringPage.state.selectedId && items.length) MonitoringPage.state.selectedId = items[0].id;
      const selected = MonitoringPage.selectedMonitor();
      if (selected) MonitoringPage.loadDetail?.(selected.id);
      else if (!items.length) MonitoringPage.clearDetail?.();
    } catch (err) {
      console.error('monitor list', err);
    }
  }
  function applyCardsVisibility(hasMonitors) {
    const noMonitors = document.getElementById('monitor-no-monitors');
    const empty = document.getElementById('monitor-empty');
    const detail = document.getElementById('monitor-detail');
    const events = document.getElementById('monitor-events-center');
    if (noMonitors) noMonitors.hidden = hasMonitors;
    if (empty) empty.hidden = !hasMonitors;
    if (detail && !hasMonitors) detail.hidden = true;
    if (events) events.hidden = !hasMonitors || !MonitoringPage.hasPermission('monitoring.events.view');
  }
  function buildQuery() {
    const f = MonitoringPage.state.filters;
    const params = new URLSearchParams();
    if (f.q) params.set('q', f.q);
    if (f.status) params.set('status', f.status);
    if (f.active) params.set('active', f.active);
    if (Array.isArray(f.tags) && f.tags.length) params.set('tag', f.tags.join(','));
    const qs = params.toString();
    return qs ? `?${qs}` : '';
  }
  function syncFilters() {
    MonitoringPage.state.filters.status = els.status?.value || '';
    MonitoringPage.state.filters.active = els.active?.value || '';
    MonitoringPage.state.filters.tags = getSelectedOptions(els.tags);
  }
  function renderList(items) {
    if (!els.list) return;
    els.list.innerHTML = '';
    if (!items.length) {
      const empty = document.createElement('div');
      empty.className = 'muted';
      empty.textContent = MonitoringPage.t('monitoring.emptyList');
      els.list.appendChild(empty);
      return;
    }
    items.forEach((item) => els.list.appendChild(renderCard(item)));
    syncBulkUI();
  }
  function renderCard(item) {
    const card = document.createElement('div');
    card.className = 'monitor-item';
    if (!ui.bulk && item.id === MonitoringPage.state.selectedId) card.classList.add('active');
    if (ui.bulk && ui.selected.has(item.id)) card.classList.add('selected');
    const title = document.createElement('div');
    title.className = 'monitor-item-header';
    title.appendChild(ui.bulk ? createBulkCheck(item.id) : createDot(item.status));
    const name = document.createElement('span');
    name.className = 'monitor-item-name';
    name.textContent = item.name || `#${item.id}`;
    title.appendChild(name);
    appendTagChips(title, item.tags || []);
    const meta = document.createElement('div');
    meta.className = 'monitor-item-meta';
    meta.textContent = HOST_TARGET_TYPES.has((item.type || '').toLowerCase())
      ? (item.port ? `${item.host}:${item.port}` : (item.host || '-'))
      : (item.url || item.host || '-');
    if ((item.status || '').toLowerCase() === 'maintenance') {
      const badge = document.createElement('span');
      badge.className = 'status-badge maintenance';
      badge.textContent = MonitoringPage.t('monitoring.status.maintenance');
      meta.appendChild(badge);
    }
    card.appendChild(title);
    card.appendChild(meta);
    card.addEventListener('click', () => {
      if (ui.bulk) return toggleCardBulk(card, item.id);
      selectMonitor(item.id);
    });
    return card;
  }
  function createDot(status) {
    const dot = document.createElement('span');
    dot.className = `status-dot ${statusClass(status)}`;
    return dot;
  }
  function createBulkCheck(id) {
    const check = document.createElement('input');
    check.type = 'checkbox';
    check.className = 'monitor-item-bulk-check';
    check.checked = ui.selected.has(id);
    check.addEventListener('click', (e) => e.stopPropagation());
    check.addEventListener('change', () => toggleBulk(id));
    return check;
  }
  function appendTagChips(root, tags) {
    const values = Array.isArray(tags) ? tags.filter(Boolean) : [];
    if (!values.length) return;
    const wrap = document.createElement('div');
    wrap.className = 'monitor-item-tags';
    values.slice(0, 3).forEach((code) => {
      const chip = document.createElement('span');
      chip.className = 'monitor-item-tag';
      chip.textContent = MonitoringPage.tagLabel ? MonitoringPage.tagLabel(code) : code;
      chip.title = chip.textContent;
      wrap.appendChild(chip);
    });
    if (values.length > 3) {
      const more = document.createElement('span');
      more.className = 'monitor-item-tag monitor-item-tag-more';
      more.textContent = `+${values.length - 3}`;
      wrap.appendChild(more);
    }
    root.appendChild(wrap);
  }
  function selectMonitor(id) {
    MonitoringPage.state.selectedId = id;
    MonitoringPage.setMonitorDeepLink?.(id);
    renderList(MonitoringPage.state.monitors || []);
    MonitoringPage.loadDetail?.(id);
  }
  function setBulkMode(on) {
    ui.bulk = !!on;
    if (!ui.bulk) ui.selected.clear();
    renderList(MonitoringPage.state.monitors || []);
    syncBulkUI();
  }
  function toggleAllBulk(on) {
    const items = MonitoringPage.state.monitors || [];
    ui.selected.clear();
    if (on) items.forEach((m) => ui.selected.add(m.id));
    renderList(items);
    syncBulkUI();
  }
  function toggleCardBulk(card, id) {
    toggleBulk(id);
    const on = ui.selected.has(id);
    card.classList.toggle('selected', on);
    const check = card.querySelector('.monitor-item-bulk-check');
    if (check) check.checked = on;
  }
  function toggleBulk(id) {
    if (ui.selected.has(id)) ui.selected.delete(id);
    else ui.selected.add(id);
    syncBulkUI();
  }
  function pruneBulkSelection(items) {
    if (!ui.bulk) return;
    const ids = new Set(items.map((m) => m.id));
    Array.from(ui.selected).forEach((id) => {
      if (!ids.has(id)) ui.selected.delete(id);
    });
  }
  async function bulkSetPaused(paused) {
    if (!MonitoringPage.hasPermission('monitoring.manage')) return;
    const ids = Array.from(ui.selected);
    if (!ids.length) return;
    const action = paused ? 'pause' : 'resume';
    await Promise.allSettled(ids.map((id) => Api.post(`/api/monitoring/monitors/${id}/${action}`, {})));
    await MonitoringPage.loadMonitors?.();
    if (MonitoringPage.state.selectedId) await MonitoringPage.loadDetail?.(MonitoringPage.state.selectedId);
  }
  function syncBulkUI() {
    const count = ui.selected.size;
    const canManage = MonitoringPage.hasPermission('monitoring.manage');
    els.bulkToggle?.classList.toggle('active', ui.bulk);
    if (els.bulkActions) els.bulkActions.hidden = !ui.bulk;
    if (els.bulkCount) els.bulkCount.textContent = `${MonitoringPage.t('monitoring.bulk.selected')}: ${count}`;
    if (els.bulkAll) {
      const total = (MonitoringPage.state.monitors || []).length;
      els.bulkAll.indeterminate = count > 0 && count < total;
      els.bulkAll.checked = total > 0 && count === total;
    }
    [els.bulkPause, els.bulkResume].forEach((btn) => {
      if (!btn) return;
      btn.disabled = !canManage || !count;
      btn.classList.toggle('disabled', btn.disabled);
    });
  }
  function populateTagOptions(items = []) {
    if (!els.tags) return;
    const tags = new Set();
    if (typeof TagDirectory !== 'undefined' && TagDirectory.all) TagDirectory.all().forEach((t) => tags.add(t.code || t));
    items.forEach((m) => (m.tags || []).forEach((t) => tags.add(t)));
    const selected = new Set(getSelectedOptions(els.tags));
    els.tags.innerHTML = '';
    Array.from(tags).sort().forEach((tag) => {
      const opt = document.createElement('option');
      opt.value = tag;
      opt.textContent = (typeof TagDirectory !== 'undefined' && TagDirectory.label) ? (TagDirectory.label(tag) || tag) : tag;
      opt.selected = selected.has(tag);
      els.tags.appendChild(opt);
    });
    renderTagsMenu();
    renderMenuLabels();
    MonitoringPage.bindTagHint?.(els.tags, els.tagsHint);
  }
  function renderStatusMenu() {
    renderSingleSelectMenu(els.statusMenu, els.status);
  }
  function renderActiveMenu() {
    renderSingleSelectMenu(els.activeMenu, els.active);
  }
  function renderSingleSelectMenu(menu, select) {
    if (!menu || !select) return;
    menu.innerHTML = Array.from(select.options || []).map((opt) => {
      const active = select.value === (opt.value || '');
      const text = MonitoringPage.t(opt.dataset.i18n || '') || opt.textContent || '';
      return `<button type="button" class="${active ? 'active' : ''}" data-value="${escapeHtml(opt.value || '')}">${escapeHtml(text)}</button>`;
    }).join('');
  }
  function renderTagsMenu() {
    if (!els.tagsMenu || !els.tags) return;
    const selected = new Set(getSelectedOptions(els.tags));
    const rows = Array.from(els.tags.options || []).map((opt) => `
      <label><input type="checkbox" data-tag="${escapeHtml(opt.value || '')}" ${selected.has(opt.value) ? 'checked' : ''}>
      <span>${escapeHtml(opt.textContent || opt.value || '')}</span></label>
    `).join('');
    els.tagsMenu.innerHTML = rows || `<div class="muted">${escapeHtml(MonitoringPage.t('common.empty') || '-')}</div>`;
    els.tagsMenu.querySelectorAll('input[type="checkbox"][data-tag]').forEach((input) => {
      input.addEventListener('change', () => {
        const val = input.dataset.tag || '';
        Array.from(els.tags.options || []).forEach((opt) => { if (opt.value === val) opt.selected = !!input.checked; });
      });
    });
  }
  function renderMenuLabels() {
    renderSingleMenuLabel(els.statusBtn, els.status, 'monitoring.filter.status');
    renderSingleMenuLabel(els.activeBtn, els.active, 'monitoring.filter.active');
    if (els.tagsBtn && els.tags) {
      const cnt = getSelectedOptions(els.tags).length;
      const text = `${MonitoringPage.t('monitoring.filter.tags')}${cnt ? ` (${cnt})` : ''}`;
      els.tagsBtn.innerHTML = `<span>${escapeHtml(text)}</span>`;
    }
  }
  function renderSingleMenuLabel(btn, select, key) {
    if (!btn || !select) return;
    const opt = select.selectedOptions?.[0];
    const value = MonitoringPage.t(opt?.dataset?.i18n || '') || opt?.textContent || MonitoringPage.t('common.all');
    btn.innerHTML = `<span>${escapeHtml(MonitoringPage.t(key))}: ${escapeHtml(value)}</span>`;
  }
  function getSelectedOptions(select) {
    if (!select) return [];
    return Array.from(select.selectedOptions).map((o) => o.value);
  }
  function statusClass(status) {
    const v = (status || '').toLowerCase();
    if (v === 'up') return 'up';
    if (v === 'paused') return 'paused';
    if (v === 'maintenance') return 'maintenance';
    return 'down';
  }
  function escapeHtml(str) {
    return (str || '').toString().replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }
  function debounce(fn, delay) {
    let t;
    return (...args) => {
      clearTimeout(t);
      t = setTimeout(() => fn(...args), delay);
    };
  }
  if (typeof MonitoringPage !== 'undefined') {
    MonitoringPage.bindList = bindList;
    MonitoringPage.loadMonitors = loadMonitors;
    MonitoringPage.renderMonitorList = () => renderList(MonitoringPage.state.monitors || []);
  }
})();
