(() => {
  const state = ReportsPage.state;

  function bindList() {
    const searchBtn = document.getElementById('reports-search-btn');
    const searchInput = document.getElementById('reports-search');
    if (searchBtn) {
      searchBtn.onclick = () => {
        state.filters.search = searchInput.value.trim();
        ReportsPage.loadReports();
      };
    }
    if (searchInput) {
      searchInput.onkeypress = (e) => {
        if (e.key === 'Enter') {
          state.filters.search = searchInput.value.trim();
          ReportsPage.loadReports();
        }
      };
    }
    const status = document.getElementById('reports-filter-status');
    if (status) status.onchange = () => {
      state.filters.status = status.value;
      ReportsPage.loadReports();
    };
    const cls = document.getElementById('reports-filter-classification');
    if (cls) cls.onchange = () => {
      state.filters.classification = cls.value;
      ReportsPage.loadReports();
    };
    const tags = document.getElementById('reports-filter-tags');
    if (tags) tags.onchange = () => {
      state.filters.tags = Array.from(tags.selectedOptions).map(o => o.value);
      ReportsPage.loadReports();
    };
    const mine = document.getElementById('reports-filter-mine');
    if (mine) mine.onchange = () => {
      state.filters.mine = mine.checked;
      ReportsPage.loadReports();
    };
    const periodFrom = document.getElementById('reports-filter-period-from');
    if (periodFrom) periodFrom.onchange = () => {
      state.filters.periodFrom = periodFrom.value;
      ReportsPage.loadReports();
    };
    const periodTo = document.getElementById('reports-filter-period-to');
    if (periodTo) periodTo.onchange = () => {
      state.filters.periodTo = periodTo.value;
      ReportsPage.loadReports();
    };
    const refreshBtn = document.getElementById('reports-refresh-btn');
    if (refreshBtn) refreshBtn.onclick = () => ReportsPage.loadReports();
    const newBtn = document.getElementById('reports-new-btn');
    if (newBtn) newBtn.onclick = () => {
      if (ReportsPage.openCreateModal) {
        ReportsPage.openCreateModal();
      }
    };
    const auditBtn = document.getElementById('reports-audit-package-btn');
    if (auditBtn) {
      auditBtn.onclick = () => exportAuditPackage();
    }
    if (ReportsPage.bindContextMenu) ReportsPage.bindContextMenu();
    const table = document.getElementById('reports-table');
    if (table) {
      table.addEventListener('click', (e) => {
        const row = e.target.closest('tr[data-report-id]');
        if (!row) return;
        if (e.target.closest('button,a,input,select,textarea,label')) return;
        const id = parseInt(row.dataset.reportId, 10);
        if (!id) return;
        if (ReportsPage.openViewer) {
          ReportsPage.openViewer(id);
        } else if (ReportsPage.openEditor) {
          ReportsPage.openEditor(id);
        }
      });
    }
  }

  async function loadReports() {
    const params = new URLSearchParams();
    if (state.filters.search) params.set('q', state.filters.search);
    if (state.filters.status) params.set('status', state.filters.status);
    if (state.filters.classification) params.set('classification', state.filters.classification);
    if (state.filters.tags && state.filters.tags.length) params.set('tag', state.filters.tags.join(','));
    if (state.filters.mine) params.set('mine', '1');
    const from = ReportsPage.toISODateInput(state.filters.periodFrom);
    const to = ReportsPage.toISODateInput(state.filters.periodTo);
    if (from) params.set('date_from', from);
    if (to) params.set('date_to', to);
    try {
      const res = await Api.get(`/api/reports?${params.toString()}`);
      state.reports = res.items || [];
      state.converters = res.converters || null;
      renderReports();
    } catch (err) {
      console.warn('load reports', err);
    }
  }

  function renderReports() {
    const table = document.querySelector('#reports-table tbody');
    if (!table) return;
    table.innerHTML = '';
    if (!state.reports.length) {
      const row = document.createElement('tr');
      row.className = 'placeholder';
      row.innerHTML = `<td colspan="7">${BerkutI18n.t('reports.empty') || '-'}</td>`;
      table.appendChild(row);
    }
    state.reports.forEach(item => {
      const doc = item.document || {};
      const meta = item.meta || {};
      const row = document.createElement('tr');
      row.dataset.reportId = `${doc.id || ''}`;
      row.innerHTML = `
        <td>${escapeHtml(doc.title || '')}</td>
        <td>${escapeHtml(doc.reg_number || '')}</td>
        <td>${escapeHtml(ReportsPage.formatPeriod(meta))}</td>
        <td><span class="badge status-${escapeHtml(meta.report_status || meta.status || 'draft')}">${escapeHtml(statusLabel(meta.report_status || meta.status))}</span></td>
        <td>${escapeHtml(DocUI.levelName(doc.classification_level))}</td>
        <td>${escapeHtml(ownerLabel(doc.created_by))}</td>
        <td>${escapeHtml(ReportsPage.formatDate(doc.updated_at || doc.created_at))}</td>
      `;
      table.appendChild(row);
    });
    const count = document.getElementById('reports-count');
    if (count) count.textContent = `${state.reports.length}`;
  }

  function statusLabel(status) {
    return BerkutI18n.t(`reports.status.${status}`) || status || '-';
  }

  function ownerLabel(id) {
    if (!id) return '-';
    if (state.currentUser && state.currentUser.id === id) {
      return BerkutI18n.t('reports.owner.me') || UserDirectory.name(id);
    }
    return UserDirectory ? UserDirectory.name(id) : `#${id}`;
  }

  function escapeHtml(str) {
    return (str || '').toString().replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }

  async function updateStatus(id, status) {
    if (!id) return;
    try {
      await Api.put(`/api/reports/${id}`, { status });
      await ReportsPage.loadReports();
    } catch (err) {
      console.warn('update status', err);
    }
  }

  function exportReport(id) {
    if (!id) return;
    const fmt = prompt(BerkutI18n.t('reports.exportPrompt'), 'pdf') || 'pdf';
    window.open(`/api/reports/${id}/export?format=${encodeURIComponent(fmt)}`, '_blank');
  }

  async function deleteReport(id) {
    if (!id) return;
    const confirmed = window.confirm(BerkutI18n.t('reports.deleteConfirm'));
    if (!confirmed) return;
    try {
      await Api.del(`/api/reports/${id}`);
      await ReportsPage.loadReports();
    } catch (err) {
      console.warn('delete report', err);
    }
  }

  function exportAuditPackage() {
    const from = ReportsPage.toISODateInput(state.filters.periodFrom);
    const to = ReportsPage.toISODateInput(state.filters.periodTo);
    const fmt = (window.prompt(BerkutI18n.t('reports.auditPackage.formatPrompt') || 'Format (md/pdf/docx/json)', 'md') || 'md').trim().toLowerCase();
    const params = new URLSearchParams();
    if (from) params.set('period_from', from);
    if (to) params.set('period_to', to);
    params.set('format', fmt || 'md');
    params.set('limit', '300');
    window.open(`/api/reports/audit-package?${params.toString()}`, '_blank');
  }

  ReportsPage.bindList = bindList;
  ReportsPage.loadReports = loadReports;
  ReportsPage.renderReports = renderReports;
  ReportsPage.updateStatus = updateStatus;
  ReportsPage.exportReport = exportReport;
  ReportsPage.deleteReport = deleteReport;
  ReportsPage.exportAuditPackage = exportAuditPackage;
  ReportsPage.bindContextMenu = bindContextMenu;

  function bindContextMenu() {
    hideContextMenu();
    document.addEventListener('click', hideContextMenu);
    window.addEventListener('resize', hideContextMenu);
    document.addEventListener('scroll', hideContextMenu, true);
    const table = document.getElementById('reports-table');
    if (!table) return;
    table.addEventListener('contextmenu', (e) => {
      const row = e.target.closest('tr[data-report-id]');
      if (!row) return;
      const id = parseInt(row.dataset.reportId, 10);
      if (!id) return;
      e.preventDefault();
      e.stopPropagation();
      showContextMenu(e.clientX, e.clientY, id);
    });
  }

  function buildContextActions(docId) {
    const actions = [];
    actions.push({
      label: BerkutI18n.t('docs.menu.open') || BerkutI18n.t('reports.action.open'),
      handler: () => (ReportsPage.openViewer ? ReportsPage.openViewer(docId) : ReportsPage.openEditor(docId))
    });
    if (ReportsPage.hasPermission('reports.edit')) {
      actions.push({
        label: BerkutI18n.t('docs.menu.edit'),
        handler: () => ReportsPage.openEditor(docId)
      });
    }
    if (ReportsPage.hasPermission('reports.export')) {
      actions.push({
        label: BerkutI18n.t('docs.menu.export') || BerkutI18n.t('reports.action.export'),
        handler: () => ReportsPage.exportReport(docId)
      });
    }
    if (ReportsPage.hasPermission('docs.approval.start') && typeof DocsPage !== 'undefined' && DocsPage.openApprovalModal) {
      actions.push({
        label: BerkutI18n.t('docs.menu.approval'),
        handler: () => DocsPage.openApprovalModal(docId)
      });
    }
    if (ReportsPage.hasPermission('reports.delete')) {
      actions.push({
        label: BerkutI18n.t('common.delete'),
        danger: true,
        handler: () => ReportsPage.deleteReport(docId)
      });
    }
    return actions;
  }

  function showContextMenu(x, y, docId) {
    const menu = document.getElementById('reports-context-menu');
    if (!menu) return;
    const actions = buildContextActions(docId);
    menu.innerHTML = '';
    actions.forEach(action => {
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = action.danger ? 'danger' : '';
      btn.textContent = action.label || '';
      btn.onclick = () => {
        hideContextMenu();
        action.handler();
      };
      menu.appendChild(btn);
    });
    menu.hidden = false;
    const rect = menu.getBoundingClientRect();
    const left = Math.min(x, window.innerWidth - rect.width - 10);
    const top = Math.min(y, window.innerHeight - rect.height - 10);
    menu.style.left = `${Math.max(10, left)}px`;
    menu.style.top = `${Math.max(10, top)}px`;
  }

  function hideContextMenu() {
    const menu = document.getElementById('reports-context-menu');
    if (!menu) return;
    menu.hidden = true;
  }
})();
