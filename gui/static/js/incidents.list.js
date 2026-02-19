(() => {
  const state = IncidentsPage.state;
  const { t, escapeHtml } = IncidentsPage;

  function renderList() {
    const container = document.getElementById('incidents-list');
    if (!container) return;
    container.innerHTML = `
      <div class="card table-card">
        <div class="card-header">
          <div>
            <h3>${t('incidents.listTitle')}</h3>
            <p>${t('incidents.listSubtitle')}</p>
          </div>
          <div class="actions">
            <button class="btn primary" id="incident-create-btn">${t('incidents.createButton')}</button>
          </div>
        </div>
        <div class="card-body">
          <div class="incident-filters" id="incident-filters">
            <select id="incident-filter-status">
              <option value="">${t('incidents.filters.statusAll')}</option>
              <option value="draft">${t('incidents.status.draft')}</option>
              <option value="open">${t('incidents.status.open')}</option>
              <option value="in_progress">${t('incidents.status.in_progress')}</option>
              <option value="contained">${t('incidents.status.contained')}</option>
              <option value="resolved">${t('incidents.status.resolved')}</option>
              <option value="waiting">${t('incidents.status.waiting')}</option>
              <option value="waiting_info">${t('incidents.status.waiting_info')}</option>
              <option value="approval">${t('incidents.status.approval')}</option>
              <option value="closed">${t('incidents.status.closed')}</option>
            </select>
            <select id="incident-filter-severity">
              <option value="">${t('incidents.filters.severityAll')}</option>
              <option value="critical">${t('incidents.severity.critical')}</option>
              <option value="high">${t('incidents.severity.high')}</option>
              <option value="medium">${t('incidents.severity.medium')}</option>
              <option value="low">${t('incidents.severity.low')}</option>
            </select>
            <select id="incident-filter-scope">
              <option value="all">${t('incidents.filters.scopeAll')}</option>
              <option value="mine">${t('incidents.filters.scopeMine')}</option>
            </select>
            <select id="incident-filter-period">
              <option value="all">${t('incidents.filters.periodAll')}</option>
              <option value="24h">${t('incidents.filters.period24h')}</option>
              <option value="7d">${t('incidents.filters.period7d')}</option>
              <option value="30d">${t('incidents.filters.period30d')}</option>
            </select>
          </div>
          <div class="table-responsive">
            <table class="data-table" id="incidents-table">
              <thead>
                <tr>
                  <th>${t('incidents.table.id')}</th>
                  <th>${t('incidents.table.title')}</th>
                  <th>${t('incidents.table.severity')}</th>
                  <th>${t('incidents.table.status')}</th>
                  <th>${t('incidents.table.owner')}</th>
                  <th>${t('incidents.table.createdAt')}</th>
                  <th>${t('incidents.table.updatedAt')}</th>
                  <th>${t('incidents.table.actions')}</th>
                </tr>
              </thead>
              <tbody></tbody>
            </table>
          </div>
        </div>
      </div>`;
    const createBtn = document.getElementById('incident-create-btn');
    if (createBtn) createBtn.addEventListener('click', () => IncidentsPage.openCreateTab());
    bindFilters();
    syncFilterControls();
    renderTableRows();
  }

  function bindFilters() {
    const status = document.getElementById('incident-filter-status');
    const severity = document.getElementById('incident-filter-severity');
    const scope = document.getElementById('incident-filter-scope');
    const period = document.getElementById('incident-filter-period');
    if (status) status.onchange = () => { state.filters.status = status.value; renderTableRows(); };
    if (severity) severity.onchange = () => { state.filters.severity = severity.value; renderTableRows(); };
    if (scope) scope.onchange = () => { state.filters.scope = scope.value; renderTableRows(); };
    if (period) period.onchange = () => { state.filters.period = period.value; renderTableRows(); };
  }

  function syncFilterControls() {
    const status = document.getElementById('incident-filter-status');
    const severity = document.getElementById('incident-filter-severity');
    const scope = document.getElementById('incident-filter-scope');
    const period = document.getElementById('incident-filter-period');
    if (status) status.value = state.filters.status || '';
    if (severity) severity.value = state.filters.severity || '';
    if (scope) scope.value = state.filters.scope || 'all';
    if (period) period.value = state.filters.period || 'all';
  }

  function applyListFilters(filters = {}, reset = false) {
    const base = reset
      ? { status: '', severity: '', scope: 'all', period: 'all' }
      : { ...state.filters };
    state.filters = { ...base, ...filters };
    syncFilterControls();
    renderTableRows();
  }

  function openListWithFilters(filters = {}, reset = false) {
    applyListFilters(filters, reset);
    if (IncidentsPage.switchTab) IncidentsPage.switchTab('list');
    const filtersEl = document.getElementById('incident-filters');
    if (filtersEl) {
      filtersEl.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  }

  function renderTableRows() {
    const tbody = document.querySelector('#incidents-table tbody');
    if (!tbody) return;
    tbody.innerHTML = '';
    const rows = applyFilters(state.incidents.slice()).sort(sortByUpdated);
    if (!rows.length) {
      const tr = document.createElement('tr');
      tr.innerHTML = `<td colspan="8">${escapeHtml(t('incidents.listEmpty'))}</td>`;
      tbody.appendChild(tr);
      return;
    }
    const canDelete = !!(IncidentsPage.hasPermission && IncidentsPage.hasPermission('incidents.delete'));
    rows.forEach(incident => {
      const tr = document.createElement('tr');
      tr.className = 'incident-row';
      tr.dataset.id = incident.id;
      const statusDisplay = IncidentsPage.getIncidentStatusDisplay
        ? IncidentsPage.getIncidentStatusDisplay(incident)
        : { status: incident.status, label: t(`incidents.status.${incident.status}`) };
      tr.innerHTML = `
        <td>${escapeHtml(incidentLabel(incident))}</td>
        <td>${escapeHtml(incident.title)}</td>
        <td><span class="badge severity-badge severity-${incident.severity}">${escapeHtml(t(`incidents.severity.${incident.severity}`))}</span></td>
        <td><span class="badge status-badge status-${statusDisplay.status}">${escapeHtml(statusDisplay.label)}</span></td>
        <td>${escapeHtml(incident.owner_name || incident.owner || '')}</td>
        <td>${escapeHtml(IncidentsPage.formatDate(incident.created_at))}</td>
        <td>${escapeHtml(IncidentsPage.formatDate(incident.updated_at))}</td>
        <td>${canDelete ? `<button class="btn ghost btn-sm incident-delete-btn" title="${escapeHtml(t('incidents.action.delete'))}" aria-label="${escapeHtml(t('incidents.action.delete'))}">x</button>` : ''}</td>`;
      tr.addEventListener('click', () => IncidentsPage.openIncidentTab(incident.id));
      tr.addEventListener('contextmenu', (e) => {
        e.preventDefault();
        showContextMenu(e.clientX, e.clientY, incident.id);
      });
      const deleteBtn = tr.querySelector('.incident-delete-btn');
      if (deleteBtn) {
        deleteBtn.addEventListener('click', async (e) => {
          e.preventDefault();
          e.stopPropagation();
          const confirmed = await IncidentsPage.confirmAction({
            title: t('common.confirm'),
            message: t('incidents.listDeleteConfirm'),
            confirmText: t('common.delete'),
            cancelText: t('common.cancel'),
          });
          if (!confirmed) return;
          try {
            await Api.del(`/api/incidents/${incident.id}`);
            await IncidentsPage.loadIncidents();
            renderTableRows();
          } catch (err) {
            IncidentsPage.showError(err, 'common.error');
          }
        });
      }
      tbody.appendChild(tr);
    });
  }

  function bindContextMenu() {
    hideContextMenu();
    document.addEventListener('click', hideContextMenu);
    window.addEventListener('resize', hideContextMenu);
    document.addEventListener('scroll', hideContextMenu, true);
  }

  function showContextMenu(x, y, incidentId) {
    const menu = document.getElementById('incidents-context-menu');
    if (!menu) return;
    menu.innerHTML = '';
    const btn = document.createElement('button');
    btn.textContent = t('incidents.context.openTab');
    btn.addEventListener('click', () => {
      hideContextMenu();
      IncidentsPage.openIncidentTab(incidentId);
    });
    menu.appendChild(btn);
    if (IncidentsPage.hasPermission && IncidentsPage.hasPermission('reports.create')) {
      const reportBtn = document.createElement('button');
      reportBtn.textContent = t('incidents.context.createReport');
      reportBtn.addEventListener('click', async () => {
        hideContextMenu();
        try {
          const res = await Api.post('/api/reports/from-incident', { incident_id: incidentId });
          if (res && res.report_id && IncidentsPage.openReportInReports) {
            IncidentsPage.openReportInReports(res.report_id);
          }
        } catch (err) {
          IncidentsPage.showError(err, 'incidents.report.createFailed');
        }
      });
      menu.appendChild(reportBtn);
    }
    menu.hidden = false;
    menu.style.display = 'flex';
    requestAnimationFrame(() => {
      const rect = menu.getBoundingClientRect();
      const left = Math.min(x, window.innerWidth - rect.width - 8);
      const top = Math.min(y, window.innerHeight - rect.height - 8);
      menu.style.left = `${left}px`;
      menu.style.top = `${top}px`;
    });
  }

  function hideContextMenu() {
    const menu = document.getElementById('incidents-context-menu');
    if (menu) {
      menu.hidden = true;
      menu.style.display = 'none';
    }
  }

  function applyFilters(items) {
    let res = items;
    if (state.filters.status) {
      res = res.filter(i => i.status === state.filters.status);
    }
    if (state.filters.severity) {
      res = res.filter(i => i.severity === state.filters.severity);
    }
    if (state.filters.scope === 'mine' && state.currentUser) {
      res = res.filter(i => i.owner_user_id === state.currentUser.id || i.assignee_user_id === state.currentUser.id);
    }
    if (state.filters.period && state.filters.period !== 'all') {
      const now = Date.now();
      const delta = periodToMs(state.filters.period);
      if (delta > 0) {
        res = res.filter(i => {
          const ts = new Date(i.created_at || i.updated_at || '').getTime();
          return ts && now - ts <= delta;
        });
      }
    }
    return res;
  }

  function periodToMs(period) {
    switch (period) {
      case '24h':
        return 24 * 60 * 60 * 1000;
      case '7d':
        return 7 * 24 * 60 * 60 * 1000;
      case '30d':
        return 30 * 24 * 60 * 60 * 1000;
      default:
        return 0;
    }
  }

  function sortByUpdated(a, b) {
    return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();
  }

  function incidentLabel(incident) {
    if (!incident) return '';
    if (incident.reg_no) return incident.reg_no;
    if (incident.id) return `#${incident.id}`;
    return '';
  }

  IncidentsPage.renderList = renderList;
  IncidentsPage.bindFilters = bindFilters;
  IncidentsPage.renderTableRows = renderTableRows;
  IncidentsPage.applyFilters = applyFilters;
  IncidentsPage.periodToMs = periodToMs;
  IncidentsPage.sortByUpdated = sortByUpdated;
  IncidentsPage.incidentLabel = incidentLabel;
  IncidentsPage.bindContextMenu = bindContextMenu;
  IncidentsPage.showContextMenu = showContextMenu;
  IncidentsPage.hideContextMenu = hideContextMenu;
  IncidentsPage.applyListFilters = applyListFilters;
  IncidentsPage.openListWithFilters = openListWithFilters;
  IncidentsPage.syncFilterControls = syncFilterControls;
})();
