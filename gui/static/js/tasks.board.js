(() => {
  const state = TasksPage.state;
  const {
    t,
    hasPermission,
    showAlert,
    hideAlert,
    openModal,
    closeModal,
    setConnectionBanner,
    isNetworkError,
    resolveErrorMessage,
    showError,
    formatDateShort,
    toISODate,
    escapeHtml,
    confirmAction,
    applyTagStyle
  } = TasksPage;

  let spaceModalState = { mode: 'create', spaceId: null };
  let boardModalState = { mode: 'create', boardId: null, spaceId: null };
  let columnModalState = { mode: 'rename', columnId: null };
  let subcolumnModalState = { mode: 'add', subcolumnId: null, columnId: null };
  let taskMoveState = { taskId: null, mode: 'move' };
  let spaceDragId = null;
  const boardLayouts = {};
  const boardLayoutLoaded = new Set();
  const boardLayoutTimers = {};

  function init() {
    const page = document.getElementById('tasks-page');
    if (!page) return;
    if (page.dataset.bound === '1') return;
    page.dataset.bound = '1';
    bindUI();
    if (TasksPage.initSidebar) TasksPage.initSidebar();
    if (TasksPage.initTemplates) TasksPage.initTemplates();
    if (TasksPage.initTemplatesHome) TasksPage.initTemplatesHome();
    if (TasksPage.initTemplatePicker) TasksPage.initTemplatePicker();
    loadData();
  }

  async function loadData() {
    const pendingTaskId = parsePendingTaskFromPath();
    try {
      await TasksPage.loadCurrentUser();
      await TasksPage.ensureUserDirectory();
      await loadSpaces();
      await loadTags();
      if (TasksPage.ensureTemplates) {
        try {
          await TasksPage.ensureTemplates(false);
        } catch (_) {
          // ignore
        }
      }
      if (TasksPage.renderTemplatesHome) {
        TasksPage.renderTemplatesHome();
      }
      if (pendingTaskId) {
        await openTaskById(pendingTaskId);
      }
    } catch (err) {
      if (isNetworkError(err)) {
        setConnectionBanner(true);
      } else {
        showError(err, 'common.error');
      }
    }
  }

  function bindUI() {
    const spaceCreateBtn = document.getElementById('tasks-space-create-btn');
    const spaceCloseBtn = document.getElementById('tasks-space-close');
    const spaceCancelBtn = document.getElementById('tasks-space-cancel');
    const spaceSaveBtn = document.getElementById('tasks-space-save');
    const boardCloseBtn = document.getElementById('tasks-board-close');
    const boardCancelBtn = document.getElementById('tasks-board-cancel');
    const boardSaveBtn = document.getElementById('tasks-board-save');
    const columnCloseBtn = document.getElementById('tasks-column-close');
    const columnCancelBtn = document.getElementById('tasks-column-cancel');
    const columnSaveBtn = document.getElementById('tasks-column-save');
    const subcolumnCloseBtn = document.getElementById('tasks-subcolumn-close');
    const subcolumnCancelBtn = document.getElementById('tasks-subcolumn-cancel');
    const subcolumnSaveBtn = document.getElementById('tasks-subcolumn-save');
    const moveCloseBtn = document.getElementById('task-move-close');
    const moveCancelBtn = document.getElementById('task-move-cancel');
    const moveSaveBtn = document.getElementById('task-move-save');
    const moveSpaceSel = document.getElementById('task-move-space');
    const moveBoardSel = document.getElementById('task-move-board');
    const archiveRefreshBtn = document.getElementById('tasks-archive-refresh');

    if (spaceCreateBtn) {
      spaceCreateBtn.hidden = !hasPermission('tasks.manage');
      spaceCreateBtn.addEventListener('click', () => openSpaceModal('create'));
    }
    spaceCloseBtn?.addEventListener('click', () => closeModal('tasks-space-modal'));
    spaceCancelBtn?.addEventListener('click', () => closeModal('tasks-space-modal'));
    spaceSaveBtn?.addEventListener('click', saveSpace);

    boardCloseBtn?.addEventListener('click', () => closeModal('tasks-board-modal'));
    boardCancelBtn?.addEventListener('click', () => closeModal('tasks-board-modal'));
    boardSaveBtn?.addEventListener('click', saveBoard);

    columnCloseBtn?.addEventListener('click', () => closeModal('tasks-column-modal'));
    columnCancelBtn?.addEventListener('click', () => closeModal('tasks-column-modal'));
    columnSaveBtn?.addEventListener('click', saveColumn);

    subcolumnCloseBtn?.addEventListener('click', () => closeModal('tasks-subcolumn-modal'));
    subcolumnCancelBtn?.addEventListener('click', () => closeModal('tasks-subcolumn-modal'));
    subcolumnSaveBtn?.addEventListener('click', saveSubcolumn);

    moveCloseBtn?.addEventListener('click', () => closeModal('task-move-modal'));
    moveCancelBtn?.addEventListener('click', () => closeModal('task-move-modal'));
    moveSaveBtn?.addEventListener('click', saveTaskMove);
    moveSpaceSel?.addEventListener('change', () => populateMoveBoards());
    moveBoardSel?.addEventListener('change', () => populateMoveColumns());
    archiveRefreshBtn?.addEventListener('click', () => loadArchivedTasks(true));

    bindAccessControls();
    bindContextMenu();
    bindSpacePanelsContextMenu();
  }

  function bindAccessControls() {
    const radios = document.querySelectorAll('input[name="tasks-space-access"]');
    radios.forEach(r => r.addEventListener('change', syncAccessMode));
  }

  function syncAccessMode() {
    const mode = currentAccessMode();
    const usersField = document.getElementById('tasks-space-users-field');
    const deptField = document.getElementById('tasks-space-departments-field');
    if (usersField) usersField.hidden = mode !== 'users';
    if (deptField) deptField.hidden = mode !== 'departments';
  }

  function currentAccessMode() {
    const selected = document.querySelector('input[name="tasks-space-access"]:checked');
    return selected?.value || 'all';
  }

  function hideContextMenu() {
    const menu = document.getElementById('tasks-context-menu');
    if (!menu) return;
    menu.hidden = true;
    menu.innerHTML = '';
    menu.style.visibility = '';
  }

  function bindContextMenu() {
    const menu = document.getElementById('tasks-context-menu');
    if (!menu) return;
    document.addEventListener('click', (e) => {
      if (!e.target.closest('.tasks-menu-btn') && !e.target.closest('#tasks-context-menu') && !e.target.closest('.tasks-column-add-btn')) {
        hideContextMenu();
      }
    });
    document.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') hideContextMenu();
    });
  }

  function bindSpacePanelsContextMenu() {
    const host = document.getElementById('tasks-space-panels');
    if (!host || host.dataset.ctxBound === '1') return;
    host.dataset.ctxBound = '1';
    host.addEventListener('contextmenu', (e) => {
      if (!hasPermission('tasks.manage')) return;
      const activePanel = host.querySelector('.tasks-space-view:not([hidden])');
      if (!activePanel || !activePanel.contains(e.target)) return;
      if (e.target.closest('.tasks-board-frame') || e.target.closest('.tasks-column') || e.target.closest('.task-card')) {
        return;
      }
      const spaceId = parseInt(activePanel.dataset.spaceId || '0', 10);
      if (!spaceId) return;
      e.preventDefault();
      showContextMenu(e.clientX, e.clientY, [
        { label: t('tasks.actions.createBoard'), handler: () => openBoardModal('create', null, spaceId) }
      ]);
    });
  }

  function getClosedSpaceTabs() {
    try {
      const raw = localStorage.getItem('tasks.closedSpaces');
      return raw ? JSON.parse(raw) : [];
    } catch (_) {
      return [];
    }
  }

  function setClosedSpaceTabs(list) {
    try {
      localStorage.setItem('tasks.closedSpaces', JSON.stringify(list));
    } catch (_) {
      // ignore
    }
  }

  function isSpaceTabClosed(spaceId) {
    return getClosedSpaceTabs().includes(spaceId);
  }

  function closeSpaceTab(spaceId) {
    const closed = new Set(getClosedSpaceTabs());
    closed.add(spaceId);
    setClosedSpaceTabs(Array.from(closed));
  }

  function reopenSpaceTab(spaceId) {
    const closed = getClosedSpaceTabs().filter(id => id !== spaceId);
    setClosedSpaceTabs(closed);
  }

  function getSpaceOrder() {
    try {
      const raw = localStorage.getItem('tasks.spaceOrder');
      return raw ? JSON.parse(raw) : [];
    } catch (_) {
      return [];
    }
  }

  function setSpaceOrder(order) {
    try {
      localStorage.setItem('tasks.spaceOrder', JSON.stringify(order));
    } catch (_) {
      // ignore
    }
  }

  function positionContextMenu(menu, x, y) {
    const padding = 10;
    const maxX = window.innerWidth - menu.offsetWidth - padding;
    const maxY = window.innerHeight - menu.offsetHeight - padding;
    menu.style.left = `${Math.max(padding, Math.min(x, maxX))}px`;
    menu.style.top = `${Math.max(padding, Math.min(y, maxY))}px`;
  }

  function showContextMenu(x, y, actions) {
    const menu = document.getElementById('tasks-context-menu');
    if (!menu) return;
    menu.innerHTML = '';
    menu.hidden = false;
    menu.style.visibility = 'hidden';
    menu.style.left = '0px';
    menu.style.top = '0px';
    menu.dataset.anchorX = `${x}`;
    menu.dataset.anchorY = `${y}`;

    actions.forEach(act => {
      if (act.row && Array.isArray(act.buttons)) {
        const row = document.createElement('div');
        row.className = 'context-menu-row';
        act.buttons.forEach(child => {
          const btn = document.createElement('button');
          btn.type = 'button';
          btn.textContent = child.label || '';
          if (child.danger) btn.classList.add('danger');
          btn.onclick = () => {
            if (!child.keepOpen) hideContextMenu();
            child.handler();
          };
          row.appendChild(btn);
        });
        menu.appendChild(row);
        return;
      }
      if (act.children && act.children.length) {
        const wrap = document.createElement('div');
        wrap.className = 'context-menu-group';
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.textContent = act.label || '';
        if (act.danger) btn.classList.add('danger');
        const sub = document.createElement('div');
        sub.className = 'context-submenu';
        sub.hidden = true;
        act.children.forEach(child => {
          const subBtn = document.createElement('button');
          subBtn.type = 'button';
          subBtn.textContent = child.label || '';
          if (child.danger) subBtn.classList.add('danger');
          subBtn.onclick = () => {
            if (!child.keepOpen) hideContextMenu();
            child.handler();
          };
          sub.appendChild(subBtn);
        });
        btn.onclick = (e) => {
          e.stopPropagation();
          sub.hidden = !sub.hidden;
          const anchorX = parseInt(menu.dataset.anchorX || '0', 10);
          const anchorY = parseInt(menu.dataset.anchorY || '0', 10);
          positionContextMenu(menu, anchorX, anchorY);
        };
        wrap.appendChild(btn);
        wrap.appendChild(sub);
        menu.appendChild(wrap);
        return;
      }
      const btn = document.createElement('button');
      btn.type = 'button';
      if (act.danger) btn.classList.add('danger');
      btn.textContent = act.label || '';
      btn.onclick = () => {
        if (!act.keepOpen) hideContextMenu();
        act.handler();
      };
      menu.appendChild(btn);
    });

    positionContextMenu(menu, x, y);
    menu.style.visibility = '';
  }

  async function loadSpaces(selectId) {
    try {
      const res = await Api.get('/api/tasks/spaces');
      state.spaces = res.items || [];
      state.spaceMap = {};
      state.spaces.forEach(sp => { state.spaceMap[sp.id] = sp; });
      setConnectionBanner(false);
    } catch (err) {
      if (isNetworkError(err)) setConnectionBanner(true);
      showError(err, 'common.error');
      return;
    }
    await loadSpaceSummary();
    renderSpaceTabs(selectId);
    renderSpaceList();
  }

  async function loadSpaceSummary() {
    try {
      const res = await Api.get('/api/tasks/spaces/summary');
      const summary = {};
      (res.items || []).forEach(item => {
        if (item && item.id) summary[item.id] = item;
      });
      state.spaceSummary = summary;
    } catch (_) {
      state.spaceSummary = {};
    }
  }

  async function loadTags() {
    try {
      const res = await Api.get('/api/tasks/tags');
      state.tags = res.items || [];
    } catch (_) {
      state.tags = [];
    }
  }

  function renderSpaceTabs(selectId) {
    const tabs = document.getElementById('tasks-tabs');
    if (!tabs) return;
    const existing = Array.from(tabs.querySelectorAll('[data-space-tab]'));
    existing.forEach(btn => btn.remove());
    const homeBtn = tabs.querySelector('[data-tab="tasks-tab-home"]');
    const archiveBtn = tabs.querySelector('[data-tab="tasks-tab-archive"]');
    if (homeBtn && !homeBtn.dataset.bound) {
      homeBtn.dataset.bound = '1';
      homeBtn.addEventListener('click', () => switchSpaceTab(null));
    }
    if (archiveBtn && !archiveBtn.dataset.bound) {
      archiveBtn.dataset.bound = '1';
      archiveBtn.addEventListener('click', () => switchSpaceTab('archive'));
    }
    const hashSpace = parseSpaceFromPath();
    const saved = selectId || hashSpace || 0;
    const current = state.spaces.find(s => s.id === saved) || null;
    state.spaceId = current?.id || null;
    if (state.spaceId && isSpaceTabClosed(state.spaceId)) {
      state.spaceId = null;
    }
    state.spaces.forEach(space => {
      if (isSpaceTabClosed(space.id)) return;
      const btn = document.createElement('a');
      btn.className = 'tab-btn';
      btn.dataset.tab = `tasks-space-${space.id}`;
      btn.dataset.spaceTab = space.id;
      const title = document.createElement('span');
      title.className = 'tab-title';
      title.textContent = space.name || `#${space.id}`;
      btn.appendChild(title);
      if (space.id === state.spaceId) btn.classList.add('active');
      btn.href = `/tasks/space/${space.id}`;
      btn.addEventListener('click', (e) => {
        e.preventDefault();
        switchSpaceTab(space.id);
      });
      const close = document.createElement('span');
      close.className = 'tab-close';
      close.textContent = 'x';
      close.setAttribute('role', 'button');
      close.setAttribute('aria-label', t('common.close') || 'Close');
      close.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        closeSpaceTab(space.id);
        if (state.spaceId === space.id) {
          switchSpaceTab(null);
        } else {
          renderSpaceTabs(state.spaceId);
        }
      });
      btn.appendChild(close);
      tabs.appendChild(btn);
    });
    switchSpaceTab(state.spaceId || null);
  }

  function switchSpaceTab(spaceId) {
    if (spaceId === 'archive') {
      state.spaceId = null;
      document.querySelectorAll('#tasks-tabs .tab-btn').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.tab === 'tasks-tab-archive');
      });
      activateTab('tasks-tab-archive');
      loadArchivedTasks();
      updateSpaceHash(null);
      return;
    }
    if (!spaceId) {
      state.spaceId = null;
      document.querySelectorAll('#tasks-tabs .tab-btn').forEach(btn => {
        const active = btn.dataset.tab === 'tasks-tab-home';
        btn.classList.toggle('active', active);
      });
      activateTab('tasks-tab-home');
      loadSpaceSummary().then(renderSpaceList).catch(() => {});
      if (TasksPage.renderTemplatesHome) {
        TasksPage.renderTemplatesHome();
      }
      updateSpaceHash(null);
      return;
    }
    state.spaceId = spaceId;
    localStorage.setItem('tasks.spaceId', `${spaceId}`);
    const tabs = document.querySelectorAll('#tasks-tabs .tab-btn');
    tabs.forEach(btn => {
      btn.classList.toggle('active', btn.dataset.spaceTab === `${spaceId}`);
    });
    activateTab(`tasks-space-${spaceId}`);
    ensureSpacePanel(spaceId);
    loadBoards(spaceId);
    updateSpaceHash(spaceId);
  }

  function activateTab(tabId) {
    document.querySelectorAll('.tasks-panel').forEach(panel => {
      panel.hidden = panel.id !== tabId && panel.dataset.tab !== tabId;
    });
  }

  function ensureSpacePanel(spaceId) {
    const host = document.getElementById('tasks-space-panels');
    if (!host) return;
    let panel = host.querySelector(`.tasks-space-view[data-space-id="${spaceId}"]`);
    if (!panel) {
      const tpl = document.getElementById('tasks-space-panel-template');
      if (!tpl) return;
      panel = tpl.content.firstElementChild.cloneNode(true);
      panel.dataset.spaceId = spaceId;
      panel.id = `tasks-space-${spaceId}`;
      panel.classList.add('tasks-panel');
      panel.dataset.tab = `tasks-space-${spaceId}`;
      host.appendChild(panel);
      bindSpacePanel(panel, spaceId);
    }
    document.querySelectorAll('#tasks-space-panels .tasks-space-view').forEach(el => {
      el.hidden = el.dataset.spaceId !== `${spaceId}`;
    });
  }

  function bindSpacePanel(panel, spaceId) {
    const onSpaceContext = (e) => {
      if (!hasPermission('tasks.manage')) return;
      if (e.target.closest('.tasks-board-frame') || e.target.closest('.tasks-column') || e.target.closest('.task-card')) {
        return;
      }
      e.preventDefault();
      e.stopPropagation();
      showContextMenu(e.clientX, e.clientY, [
        { label: t('tasks.actions.createBoard'), handler: () => openBoardModal('create', null, spaceId) }
      ]);
    };
    panel.addEventListener('contextmenu', onSpaceContext);
  }

  function renderSpaceList() {
    const list = document.getElementById('tasks-space-list');
    const empty = document.getElementById('tasks-space-empty');
    if (!list || !empty) return;
    list.innerHTML = '';
    if (!state.spaces.length) {
      empty.hidden = false;
      return;
    }
    empty.hidden = true;
    const order = getSpaceOrder();
    const orderIndex = new Map(order.map((id, idx) => [id, idx]));
    const spaces = state.spaces.slice().sort((a, b) => {
      const ia = orderIndex.has(a.id) ? orderIndex.get(a.id) : null;
      const ib = orderIndex.has(b.id) ? orderIndex.get(b.id) : null;
      if (ia !== null && ib !== null) return ia - ib;
      if (ia !== null) return -1;
      if (ib !== null) return 1;
      return (a.name || '').localeCompare(b.name || '');
    });
    spaces.forEach(space => {
      const card = document.createElement('div');
      card.className = 'tasks-space-card';
      card.dataset.spaceId = space.id;
      const actions = [];
      if (hasPermission('tasks.manage')) {
        actions.push(`<button class="btn ghost btn-sm" data-edit="${space.id}">${t('common.edit')}</button>`);
        actions.push(`<button class="btn ghost btn-sm danger" data-delete="${space.id}">${t('common.delete')}</button>`);
      }
      const summary = state.spaceSummary?.[space.id] || {};
      const boardCount = summary.board_count || 0;
      const taskCount = summary.task_count || 0;
      const boards = Array.isArray(summary.boards) ? summary.boards : [];
      const boardRows = boards.slice(0, 4).map(b => `
        <div class="tasks-space-board-row">
          <span>${escapeHtml(b.name || `#${b.id}`)}</span>
          <span>${t('tasks.spaces.tasksCount')}: ${b.task_count ?? 0}</span>
        </div>
      `).join('');
      const moreBoards = boards.length > 4 ? `<div class="muted">${t('tasks.spaces.moreBoards')}: ${boards.length - 4}</div>` : '';
      card.innerHTML = `
        <div class="tasks-space-card-head">
          <div>
            <div class="tasks-space-card-title">${escapeHtml(space.name || '')}</div>
            <div class="muted">${escapeHtml(space.description || '')}</div>
          </div>
          <div class="tasks-space-card-actions">${actions.join('')}</div>
        </div>
        <div class="tasks-space-card-meta">
          <div class="tasks-space-card-stats">
            <span>${t('tasks.spaces.boardsCount')}: ${boardCount}</span>
            <span>${t('tasks.spaces.tasksCount')}: ${taskCount}</span>
          </div>
          <div class="tasks-space-board-list">
            ${boardRows || `<div class="muted">${t('tasks.boards.empty')}</div>`}
            ${moreBoards}
          </div>
        </div>
      `;
      card.addEventListener('click', (e) => {
        if (card.dataset.suppressClick) {
          delete card.dataset.suppressClick;
          return;
        }
        if (e.target.closest('button')) return;
        reopenSpaceTab(space.id);
        renderSpaceTabs(space.id);
        switchSpaceTab(space.id);
      });
      bindSpaceDrag(card, list);
      const editBtn = card.querySelector('[data-edit]');
      const delBtn = card.querySelector('[data-delete]');
      if (editBtn) editBtn.onclick = () => openSpaceModal('edit', space.id);
      if (delBtn) delBtn.onclick = () => deleteSpace(space.id);
      list.appendChild(card);
    });
  }
  async function loadBoards(spaceId) {
    if (!spaceId) return;
    try {
      const res = await Api.get(`/api/tasks/boards?space_id=${spaceId}`);
      const boards = (res.items || []).slice().sort((a, b) => (a.position || 0) - (b.position || 0));
      state.boardsBySpace[spaceId] = boards;
      setConnectionBanner(false);
    } catch (err) {
      if (isNetworkError(err)) setConnectionBanner(true);
      showError(err, 'common.error');
      return;
    }
    await loadBoardLayout(spaceId);
    await preloadBoards(spaceId);
    renderBoards(spaceId);
  }

  async function preloadBoards(spaceId) {
    const boards = state.boardsBySpace[spaceId] || [];
    await Promise.all(boards.map(async (board) => {
      await loadColumns(board.id, true, false);
      await loadTasks(board.id);
    }));
  }

  async function loadColumns(boardId, force = false) {
    if (!boardId) return;
    if (state.columnsByBoard[boardId] && !force) return;
    try {
      const res = await Api.get(`/api/tasks/boards/${boardId}/columns`);
      const cols = (res.items || []).slice().sort((a, b) => a.position - b.position);
      state.columnsByBoard[boardId] = cols;
      await loadSubcolumns(boardId, true);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function loadSubcolumns(boardId, force = false) {
    if (!boardId) return;
    if (state.subcolumnsByBoard[boardId] && !force) return;
    try {
      const res = await Api.get(`/api/tasks/boards/${boardId}/subcolumns`);
      const subs = (res.items || []).slice();
      state.subcolumnsByBoard[boardId] = subs;
      const columns = state.columnsByBoard[boardId] || [];
      columns.forEach(col => { state.subcolumnsByColumn[col.id] = []; });
      subs.forEach(sub => {
        if (!state.subcolumnsByColumn[sub.column_id]) state.subcolumnsByColumn[sub.column_id] = [];
        state.subcolumnsByColumn[sub.column_id].push(sub);
      });
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function loadTasks(boardId) {
    if (!boardId) return;
    const params = new URLSearchParams();
    params.set('board_id', boardId);
    if (state.filters.search) params.set('search', state.filters.search);
    if (state.filters.archived) params.set('include_archived', '1');
    if (state.filters.mine) params.set('mine', '1');
    try {
      const res = await Api.get(`/api/tasks?${params.toString()}`);
      state.tasksByBoard[boardId] = res.items || [];
      state.tasksByBoard[boardId].forEach(t => state.taskMap.set(t.id, t));
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  function filteredTasks(boardId) {
    let items = (state.tasksByBoard[boardId] || []).slice();
    if (!state.filters.archived) {
      items = items.filter(t => !t.is_archived);
    }
    if (state.filters.overdue) {
      const now = new Date();
      items = items.filter(ti => {
        if (!ti.due_date || ti.closed_at || ti.is_archived) return false;
        const due = new Date(ti.due_date);
        if (Number.isNaN(due.getTime())) return false;
        return due < now;
      });
    }
    if (state.filters.high) {
      items = items.filter(ti => ['high', 'critical'].includes((ti.priority || '').toLowerCase()));
    }
    return items;
  }

  function renderBoards(spaceId) {
    const panel = document.querySelector(`.tasks-space-view[data-space-id="${spaceId}"]`);
    if (!panel) return;
    const space = state.spaceMap[spaceId];
    const title = panel.querySelector('.tasks-space-title');
    const meta = panel.querySelector('.tasks-space-meta');
    const boardShell = panel.querySelector('[data-space-boards]');
    const empty = panel.querySelector('[data-space-empty]');
    if (title) title.textContent = space?.name || '';
    if (meta) meta.textContent = space?.description || '';
    if (!boardShell || !empty) return;
    const boards = state.boardsBySpace[spaceId] || [];
    state.boards = boards;
    if (!state.boardId || !boards.find(b => b.id === state.boardId)) {
      state.boardId = boards[0]?.id || null;
    }
    boardShell.innerHTML = '';
    boardShell.classList.remove('vertical');
    boardShell.classList.toggle('is-empty', !boards.length);
    if (!boards.length) {
      empty.hidden = false;
      return;
    }
    empty.hidden = true;
    boards.forEach((board) => {
      const boardEl = renderBoard(board, boards);
      boardShell.appendChild(boardEl);
    });
    requestAnimationFrame(() => {
      measureBoardSizes(boardShell);
      applyBoardLayout(boardShell, spaceId);
    });
    bindBoardDrag(boardShell, spaceId);
  }

  function renderBoard(board, boards) {
    const tpl = document.getElementById('tasks-board-template');
    const el = tpl?.content.firstElementChild.cloneNode(true);
    if (!el) return document.createElement('div');
    el.dataset.boardId = board.id;
    el.setAttribute('draggable', 'false');
    // layout is free-form via drag & drop ordering
    // no per-board resizing
    const title = el.querySelector('.tasks-board-title');
    if (title) title.textContent = board.name || '';
    const menuBtn = el.querySelector('.tasks-menu-btn');
    const toggleBtn = el.querySelector('.tasks-board-toggle');
    if (menuBtn) {
      menuBtn.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        showContextMenu(e.clientX, e.clientY, buildBoardActions(board));
      });
    }
    if (toggleBtn) {
      toggleBtn.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        const next = !el.classList.contains('collapsed');
        el.classList.toggle('collapsed', next);
        setBoardCollapsed(board.id, next);
        if (next) {
          const fullW = parseFloat(el.dataset.fullW || '0');
          if (fullW) el.style.width = `${fullW}px`;
        } else {
          el.style.width = '';
        }
        measureBoardSizes(el.parentElement);
        applyBoardLayout(el.parentElement, board.space_id);
      });
    }
    if (isBoardCollapsed(board.id)) {
      el.classList.add('collapsed');
    }
    el.addEventListener('contextmenu', (e) => {
      e.preventDefault();
      e.stopPropagation();
      showContextMenu(e.clientX, e.clientY, buildBoardActions(board));
    });
    const columnsWrap = el.querySelector('.tasks-board-columns');
    const columns = (state.columnsByBoard[board.id] || []).slice().sort((a, b) => a.position - b.position);
    const tasksByColumn = new Map();
    const tasksBySubcolumn = new Map();
    filteredTasks(board.id).forEach(task => {
      if (task.subcolumn_id) {
        if (!tasksBySubcolumn.has(task.subcolumn_id)) tasksBySubcolumn.set(task.subcolumn_id, []);
        tasksBySubcolumn.get(task.subcolumn_id).push(task);
      } else {
        if (!tasksByColumn.has(task.column_id)) tasksByColumn.set(task.column_id, []);
        tasksByColumn.get(task.column_id).push(task);
      }
    });
    columns.forEach(col => {
      const colTasks = (tasksByColumn.get(col.id) || []).slice().sort((a, b) => (a.position || 0) - (b.position || 0));
      const subcolumns = (state.subcolumnsByColumn[col.id] || []).slice().sort((a, b) => a.position - b.position);
      columnsWrap.appendChild(renderColumn(col, colTasks, subcolumns, tasksBySubcolumn, columns));
    });
    return el;
  }

  function buildBoardActions(board) {
    const actions = [];
    if (hasPermission('tasks.manage')) {
      actions.push({
        row: true,
        buttons: [
          { label: '|<<', handler: () => moveBoardFromUI(board.id, 'first') },
          { label: '<', handler: () => moveBoardFromUI(board.id, 'left') },
          { label: '>', handler: () => moveBoardFromUI(board.id, 'right') },
          { label: '>>|', handler: () => moveBoardFromUI(board.id, 'last') }
        ]
      });
      const boardTemplateMenu = buildDefaultTemplateMenu(board.id, null, board.default_template_id || 0, setBoardDefaultTemplate);
      if (boardTemplateMenu.length) {
        actions.push({ label: t('tasks.defaultTemplate'), children: boardTemplateMenu });
      }
      actions.push({ label: t('tasks.boards.rename'), handler: () => openBoardModal('rename', board) });
      actions.push({ label: t('tasks.boards.delete'), danger: true, handler: () => deleteBoard(board) });
      actions.push({ label: t('tasks.columns.add'), handler: () => openColumnModal('add', board.id) });
      actions.push({ label: t('tasks.actions.createBoard'), handler: () => openBoardModal('create', null, board.space_id) });
    }
    return actions;
  }

  function buildDefaultTemplateMenu(boardId, columnId, currentId, setter) {
    const items = [];
    const standardLabel = currentId ? t('tasks.templateStandard') : '* ' + t('tasks.templateStandard');
    items.push({ label: standardLabel, handler: () => setter(boardId, columnId, 0) });
    const templates = (state.templates || []).filter(tpl => tpl.is_active !== false && tpl.board_id === boardId);
    const filtered = columnId ? templates.filter(tpl => tpl.column_id === columnId) : templates;
    filtered.forEach(tpl => {
      const label = tpl.id === currentId ? '* ' + tpl.title_template : tpl.title_template;
      items.push({ label, handler: () => setter(boardId, columnId, tpl.id) });
    });
    return items;
  }

  async function setBoardDefaultTemplate(boardId, _columnId, templateId) {
    try {
      const updated = await Api.put(`/api/tasks/boards/${boardId}`, { default_template_id: templateId });
      updateBoardDefaultState(updated);
      renderBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function setColumnDefaultTemplate(_boardId, columnId, templateId) {
    try {
      const updated = await Api.put(`/api/tasks/columns/${columnId}`, { default_template_id: templateId });
      updateColumnDefaultState(updated);
      renderBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  function updateBoardDefaultState(updated) {
    if (!updated?.id) return;
    for (const spaceId in state.boardsBySpace) {
      const list = state.boardsBySpace[spaceId] || [];
      list.forEach((b, idx) => {
        if (b.id === updated.id) list[idx] = updated;
      });
    }
    state.boards = (state.boards || []).map(b => (b.id === updated.id ? updated : b));
  }

  function updateColumnDefaultState(updated) {
    if (!updated?.id) return;
    for (const boardId in state.columnsByBoard) {
      const list = state.columnsByBoard[boardId] || [];
      list.forEach((c, idx) => {
        if (c.id === updated.id) list[idx] = updated;
      });
    }
  }

  function renderColumn(column, tasks, subcolumns, tasksBySubcolumn, columns) {
    const tpl = document.getElementById('tasks-column-template');
    const colEl = tpl?.content.firstElementChild.cloneNode(true);
    if (!colEl) return document.createElement('div');
    colEl.dataset.columnId = column.id;
    if (subcolumns && subcolumns.length) {
      const count = subcolumns.length;
      const gapCount = Math.max(0, count - 1);
      const totalWidth = `calc((var(--tasks-col-width) * ${count}) + (var(--tasks-subcolumn-gap) * ${gapCount}) + (var(--tasks-column-body-padding) * 2))`;
      colEl.style.width = totalWidth;
      colEl.style.minWidth = totalWidth;
      colEl.style.maxWidth = totalWidth;
    } else {
      colEl.style.width = '';
      colEl.style.minWidth = '';
      colEl.style.maxWidth = '';
    }
    const header = colEl.querySelector('.tasks-column-header');
    const title = colEl.querySelector('[data-column-title]');
    if (title) title.textContent = column.name || '';
    if (header) {
      header.addEventListener('dblclick', (e) => {
        if (e.target.closest('button')) return;
        openColumnModal('edit', column.board_id, column);
      });
    }
    const typeMeta = colEl.querySelector('[data-column-type]');
    if (typeMeta) {
      const type = getColumnType(column);
      typeMeta.textContent = columnTypeLabel(type);
    }
    const menuBtn = colEl.querySelector('.tasks-menu-btn');
    if (menuBtn) {
      menuBtn.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        showContextMenu(e.clientX, e.clientY, buildColumnActions(column, columns, colEl));
      });
    }
    const addBtn = colEl.querySelector('.tasks-column-add-btn');
    const hasSubcolumns = subcolumns && subcolumns.length > 0;
    if (addBtn) {
      addBtn.hidden = hasSubcolumns;
      addBtn.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        const rect = addBtn.getBoundingClientRect();
        showContextMenu(rect.left, rect.bottom, buildColumnAddActions(column, columns, colEl, hasSubcolumns));
      });
    }
    colEl.addEventListener('contextmenu', (e) => {
      e.preventDefault();
      e.stopPropagation();
      showContextMenu(e.clientX, e.clientY, buildColumnActions(column, columns, colEl));
    });
    // no per-column resizing
    const body = colEl.querySelector('[data-column-body]');
    if (!hasSubcolumns) {
      tasks.forEach(task => {
        body.appendChild(renderTaskCard(task));
      });
    }
    if (subcolumns && subcolumns.length) {
      const row = document.createElement('div');
      row.className = 'tasks-subcolumns-row';
      subcolumns.forEach(sub => {
        const subTasks = (tasksBySubcolumn.get(sub.id) || []).slice().sort((a, b) => (a.position || 0) - (b.position || 0));
        row.appendChild(renderSubcolumn(sub, subTasks, column));
      });
      body.appendChild(row);
    }
    if (!tasks.length) {
      // keep column body clean when there are no cards
    }
    if (!hasSubcolumns) {
      const addControl = buildColumnAddControl(column, colEl);
      if (addControl) body.appendChild(addControl);
    }
    if (!hasSubcolumns) {
      bindTaskDrop(body);
    }
    return colEl;
  }

  function renderSubcolumn(subcolumn, tasks, column) {
    const tpl = document.getElementById('tasks-subcolumn-template');
    const subEl = tpl?.content.firstElementChild.cloneNode(true);
    if (!subEl) return document.createElement('div');
    subEl.dataset.subcolumnId = subcolumn.id;
    const title = subEl.querySelector('.tasks-subcolumn-title');
    if (title) title.textContent = subcolumn.name || '';
    const header = subEl.querySelector('.tasks-subcolumn-header');
    if (header) {
      header.addEventListener('dblclick', (e) => {
        if (e.target.closest('button')) return;
        openSubcolumnModal('edit', column.id, subcolumn);
      });
    }
    const menuBtn = subEl.querySelector('.tasks-menu-btn');
    if (menuBtn) {
      menuBtn.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        showContextMenu(e.clientX, e.clientY, buildSubcolumnActions(subcolumn, column));
      });
    }
    subEl.addEventListener('contextmenu', (e) => {
      e.preventDefault();
      e.stopPropagation();
      showContextMenu(e.clientX, e.clientY, buildSubcolumnActions(subcolumn, column));
    });
    const body = subEl.querySelector('[data-subcolumn-body]');
    tasks.forEach(task => {
      body.appendChild(renderTaskCard(task));
    });
    const addBtn = subEl.querySelector('.tasks-subcolumn-add-btn');
    if (addBtn) {
      addBtn.hidden = !hasPermission('tasks.create');
      addBtn.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        focusSubcolumnAdd(subEl);
      });
    }
    const addControl = buildSubcolumnAddControl(subcolumn, subEl, column);
    if (addControl) body.appendChild(addControl);
    bindTaskDrop(body);
    return subEl;
  }

  function buildColumnActions(column, columns, colEl) {
    const actions = [];
    if (hasPermission('tasks.manage')) {
      actions.push({
        row: true,
        buttons: [
          { label: '|<<', handler: () => moveColumnFromUI(column.id, 'first') },
          { label: '<', handler: () => moveColumnFromUI(column.id, 'left') },
          { label: '>', handler: () => moveColumnFromUI(column.id, 'right') },
          { label: '>>|', handler: () => moveColumnFromUI(column.id, 'last') }
        ]
      });
      const columnTemplateMenu = buildDefaultTemplateMenu(column.board_id, column.id, column.default_template_id || 0, setColumnDefaultTemplate);
      if (columnTemplateMenu.length) {
        actions.push({ label: t('tasks.defaultTemplate'), children: columnTemplateMenu });
      }
      actions.push({ label: t('tasks.columns.edit'), handler: () => openColumnModal('edit', column.board_id, column) });
      if (hasPermission('tasks.archive')) {
        actions.push({ label: t('tasks.columns.archiveTasks'), danger: true, handler: () => archiveColumnTasks(column) });
      }
      actions.push({ label: t('tasks.columns.delete'), danger: true, handler: () => deleteColumn(column) });
      actions.push({
        label: t('tasks.columns.create'),
        children: [
          { label: t('tasks.columns.createLeft'), handler: () => createColumnRelative(column, 'left') },
          { label: t('tasks.columns.createRight'), handler: () => createColumnRelative(column, 'right') }
        ]
      });
      actions.push({ label: t('tasks.columns.addSubcolumns'), handler: () => addSubcolumns(column) });
    }
    return actions;
  }

  function buildSubcolumnActions(subcolumn, column) {
    const actions = [];
    if (hasPermission('tasks.manage')) {
      actions.push({
        row: true,
        buttons: [
          { label: '|<<', handler: () => moveSubcolumnFromUI(subcolumn.id, 'first') },
          { label: '<', handler: () => moveSubcolumnFromUI(subcolumn.id, 'left') },
          { label: '>', handler: () => moveSubcolumnFromUI(subcolumn.id, 'right') },
          { label: '>>|', handler: () => moveSubcolumnFromUI(subcolumn.id, 'last') }
        ]
      });
      actions.push({ label: t('tasks.subcolumns.edit'), handler: () => openSubcolumnModal('edit', column.id, subcolumn) });
      actions.push({ label: t('tasks.subcolumns.delete'), danger: true, handler: () => deleteSubcolumn(subcolumn) });
    }
    return actions;
  }

  function openTemplatePickerForColumn(column, subcolumn) {
    if (!TasksPage.openTemplatePicker || !hasPermission('tasks.create') || !hasPermission('tasks.templates.view')) return;
    const boardId = column?.board_id || 0;
    if (!boardId) return;
    const tasks = state.tasksByBoard[boardId] || [];
    const position = subcolumn
      ? tasks.filter(t => t.subcolumn_id === subcolumn.id && !t.is_archived).length + 1
      : tasks.filter(t => t.column_id === column.id && !t.subcolumn_id && !t.is_archived).length + 1;
    const defaultTemplateId = resolveDefaultTemplateId(column);
    TasksPage.openTemplatePicker({
      boardId,
      columnId: column.id,
      subcolumnId: subcolumn?.id || null,
      position,
      defaultTemplateId
    });
  }

  function buildColumnAddActions(column, columns, colEl, hasSubcolumns) {
    const actions = [];
    if (hasPermission('tasks.create') && !hasSubcolumns) {
      actions.push({ label: t('tasks.actions.createTask'), handler: () => focusColumnAdd(colEl) });
      actions.push({ label: t('tasks.actions.createFromTemplate'), handler: () => openTemplatePickerForColumn(column, null) });
    }
    if (hasPermission('tasks.manage')) {
      actions.push({ label: t('tasks.columns.addSubcolumns'), handler: () => addSubcolumns(column) });
      actions.push({
        label: t('tasks.columns.create'),
        children: [
          { label: t('tasks.columns.createLeft'), handler: () => createColumnRelative(column, 'left') },
          { label: t('tasks.columns.createRight'), handler: () => createColumnRelative(column, 'right') }
        ]
      });
    }
    return actions;
  }

  function buildColumnAddControl(column, colEl) {
    if (!hasPermission('tasks.create')) return null;
    const wrap = document.createElement('div');
    wrap.className = 'tasks-column-add';
    wrap.classList.add('is-hidden');
    const placeholder = document.createElement('div');
    placeholder.className = 'tasks-column-add-placeholder';
    placeholder.textContent = t('tasks.actions.createTask');
    const input = document.createElement('input');
    input.type = 'text';
    input.className = 'tasks-column-add-input';
    input.placeholder = t('tasks.fields.title');
    input.hidden = true;
    wrap.appendChild(placeholder);
    wrap.appendChild(input);

    const activate = () => {
      wrap.classList.remove('is-hidden');
      placeholder.hidden = true;
      input.hidden = false;
      input.focus();
      input.select();
    };

    wrap.addEventListener('click', (e) => {
      e.stopPropagation();
      activate();
    });

    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        submitColumnTask(column, input, wrap, placeholder);
      } else if (e.key === 'Escape') {
        input.value = '';
        input.blur();
      }
    });

    input.addEventListener('blur', () => {
      if (!input.value.trim()) resetColumnAdd(wrap, input, placeholder);
    });

    colEl.addEventListener('mouseenter', () => {
      wrap.classList.remove('is-hidden');
    });
    colEl.addEventListener('mouseleave', () => {
      if (document.activeElement === input) return;
      if (!input.value.trim()) resetColumnAdd(wrap, input, placeholder, true);
    });

    return wrap;
  }

  function resetColumnAdd(wrap, input, placeholder, hideWrap = false) {
    placeholder.hidden = false;
    input.hidden = true;
    input.value = '';
    if (hideWrap) wrap.classList.add('is-hidden');
  }

  function focusColumnAdd(colEl) {
    const wrap = colEl.querySelector('.tasks-column-add');
    if (!wrap) return;
    const placeholder = wrap.querySelector('.tasks-column-add-placeholder');
    const input = wrap.querySelector('.tasks-column-add-input');
    if (!placeholder || !input) return;
    wrap.classList.remove('is-hidden');
    placeholder.hidden = true;
    input.hidden = false;
    input.focus();
    input.select();
  }

  function focusSubcolumnAdd(subEl) {
    const wrap = subEl.querySelector('.tasks-subcolumn-add');
    if (!wrap) return;
    const placeholder = wrap.querySelector('.tasks-subcolumn-add-placeholder');
    const input = wrap.querySelector('.tasks-subcolumn-add-input');
    if (!placeholder || !input) return;
    wrap.classList.remove('is-hidden');
    placeholder.hidden = true;
    input.hidden = false;
    input.focus();
    input.select();
  }

  async function submitColumnTask(column, input, wrap, placeholder) {
    const title = (input.value || '').trim();
    if (!title) {
      input.blur();
      return;
    }
    input.value = '';
    resetColumnAdd(wrap, input, placeholder);
    await createTaskFromColumn(column, title);
  }

  function findBoard(boardId) {
    if (!boardId) return null;
    for (const spaceId in state.boardsBySpace) {
      const found = (state.boardsBySpace[spaceId] || []).find(b => b.id === boardId);
      if (found) return found;
    }
    const fallback = state.boards.find(b => b.id === boardId);
    return fallback || null;
  }

  function resolveDefaultTemplateId(column) {
    const colDefault = column?.default_template_id || 0;
    if (colDefault) return colDefault;
    const board = findBoard(column?.board_id);
    const boardDefault = board?.default_template_id || 0;
    return boardDefault || 0;
  }

  async function createTaskFromTemplate(templateId, opts = {}) {
    try {
      const payload = { column_id: opts.columnId };
      if (opts.subcolumnId) payload.subcolumn_id = opts.subcolumnId;
      if (opts.title) payload.title = opts.title;
      const created = await Api.post(`/api/tasks/templates/${templateId}/create-task`, payload);
      if (created && created.id) {
        state.taskMap.set(created.id, created);
      }
      await loadTasks(opts.boardId);
      renderBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function createTaskFromColumn(column, title) {
    if (!hasPermission('tasks.create')) return;
    const defaultTemplateId = resolveDefaultTemplateId(column);
    if (defaultTemplateId && hasPermission('tasks.templates.view')) {
      await createTaskFromTemplate(defaultTemplateId, {
        boardId: column.board_id,
        columnId: column.id,
        title
      });
      return;
    }
    const tasks = state.tasksByBoard[column.board_id] || [];
    const position = tasks.filter(t => t.column_id === column.id && !t.subcolumn_id && !t.is_archived).length + 1;
    try {
      const created = await Api.post('/api/tasks', {
        board_id: column.board_id,
        column_id: column.id,
        title,
        position
      });
      if (created && created.id) {
        state.taskMap.set(created.id, created);
      }
      await loadTasks(column.board_id);
      renderBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  function buildSubcolumnAddControl(subcolumn, subEl, column) {
    if (!hasPermission('tasks.create')) return null;
    const wrap = document.createElement('div');
    wrap.className = 'tasks-subcolumn-add';
    wrap.classList.add('is-hidden');
    const placeholder = document.createElement('div');
    placeholder.className = 'tasks-subcolumn-add-placeholder';
    placeholder.textContent = t('tasks.actions.createTask');
    const input = document.createElement('input');
    input.type = 'text';
    input.className = 'tasks-subcolumn-add-input';
    input.placeholder = t('tasks.fields.title');
    input.hidden = true;
    wrap.appendChild(placeholder);
    wrap.appendChild(input);

    const activate = () => {
      wrap.classList.remove('is-hidden');
      placeholder.hidden = true;
      input.hidden = false;
      input.focus();
      input.select();
    };

    wrap.addEventListener('click', (e) => {
      e.stopPropagation();
      activate();
    });

    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        submitSubcolumnTask(subcolumn, input, wrap, placeholder, column);
      } else if (e.key === 'Escape') {
        input.value = '';
        input.blur();
      }
    });

    input.addEventListener('blur', () => {
      if (!input.value.trim()) resetSubcolumnAdd(wrap, input, placeholder);
    });

    subEl.addEventListener('mouseenter', () => {
      wrap.classList.remove('is-hidden');
    });
    subEl.addEventListener('mouseleave', () => {
      if (document.activeElement === input) return;
      if (!input.value.trim()) resetSubcolumnAdd(wrap, input, placeholder, true);
    });

    return wrap;
  }

  function resetSubcolumnAdd(wrap, input, placeholder, hideWrap = false) {
    placeholder.hidden = false;
    input.hidden = true;
    input.value = '';
    if (hideWrap) wrap.classList.add('is-hidden');
  }

  async function submitSubcolumnTask(subcolumn, input, wrap, placeholder, column) {
    const title = (input.value || '').trim();
    if (!title) {
      input.blur();
      return;
    }
    input.value = '';
    resetSubcolumnAdd(wrap, input, placeholder);
    await createTaskFromSubcolumn(subcolumn, title, column);
  }

  async function createTaskFromSubcolumn(subcolumn, title, column) {
    if (!hasPermission('tasks.create')) return;
    const boardId = column?.board_id || findColumn(subcolumn.column_id)?.board_id;
    if (!boardId) return;
    const defaultTemplateId = resolveDefaultTemplateId(column || findColumn(subcolumn.column_id));
    if (defaultTemplateId && hasPermission('tasks.templates.view')) {
      await createTaskFromTemplate(defaultTemplateId, {
        boardId,
        columnId: subcolumn.column_id,
        subcolumnId: subcolumn.id,
        title
      });
      return;
    }
    const tasks = state.tasksByBoard[boardId] || [];
    const position = tasks.filter(t => t.subcolumn_id === subcolumn.id && !t.is_archived).length + 1;
    try {
      const created = await Api.post('/api/tasks', {
        board_id: boardId,
        column_id: subcolumn.column_id,
        subcolumn_id: subcolumn.id,
        title,
        position
      });
      if (created && created.id) {
        state.taskMap.set(created.id, created);
      }
      await loadTasks(boardId);
      renderBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function createColumnRelative(column, side) {
    if (!hasPermission('tasks.manage')) return;
    const position = side === 'left' ? (column.position || 0) : (column.position || 0) + 1;
    const name = t('tasks.columns.newDefault');
    try {
      await Api.post(`/api/tasks/boards/${column.board_id}/columns`, { name, position });
      await loadColumns(column.board_id, true);
      renderBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function moveColumnTasksToLeftmostSubcolumn(columnId) {
    const column = findColumn(columnId);
    if (!column) return;
    const subcolumns = (state.subcolumnsByColumn[columnId] || []).slice().sort((a, b) => a.position - b.position);
    const leftmost = subcolumns[0];
    if (!leftmost) return;
    const tasks = (state.tasksByBoard[column.board_id] || [])
      .filter(t => t.column_id === columnId && !t.subcolumn_id && !t.is_archived)
      .slice()
      .sort((a, b) => (a.position || 0) - (b.position || 0));
    if (!tasks.length) return;
    try {
      let position = 1;
      for (const task of tasks) {
        await Api.post(`/api/tasks/${task.id}/move`, {
          column_id: columnId,
          subcolumn_id: leftmost.id,
          position
        });
        position += 1;
      }
      await loadTasks(column.board_id);
    } catch (err) {
      showError(err, 'tasks.errors.moveFailed');
    }
  }

  async function addSubcolumns(column) {
    if (!hasPermission('tasks.manage')) return;
    const base = t('tasks.columns.subcolumn');
    const first = `${base} 1`;
    const second = `${base} 2`;
    try {
      await Api.post(`/api/tasks/columns/${column.id}/subcolumns`, { name: first, position: 0 });
      await Api.post(`/api/tasks/columns/${column.id}/subcolumns`, { name: second, position: 0 });
      await loadSubcolumns(column.board_id, true);
      await moveColumnTasksToLeftmostSubcolumn(column.id);
      renderBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function moveColumnFromUI(columnId, dir) {
    const colId = parseInt(columnId || '0', 10);
    if (!colId) return;
    const column = findColumn(colId);
    if (!column) return;
    const columns = state.columnsByBoard[column.board_id] || [];
    const idx = columns.findIndex(c => c.id === colId);
    if (idx === -1) return;
    let target = idx;
    if (dir === 'first') target = 0;
    if (dir === 'last') target = columns.length - 1;
    if (dir === 'left') target = Math.max(0, idx - 1);
    if (dir === 'right') target = Math.min(columns.length - 1, idx + 1);
    const position = (columns[target]?.position || target + 1);
    try {
      await Api.post(`/api/tasks/columns/${colId}/move`, { position });
      await loadColumns(column.board_id, true);
      renderBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function moveSubcolumnFromUI(subcolumnId, dir) {
    const subId = parseInt(subcolumnId || '0', 10);
    if (!subId) return;
    const subcolumn = findSubcolumn(subId);
    if (!subcolumn) return;
    const columnSubcolumns = (state.subcolumnsByColumn[subcolumn.column_id] || []).slice().sort((a, b) => a.position - b.position);
    const idx = columnSubcolumns.findIndex(sc => sc.id === subId);
    if (idx === -1) return;
    let target = idx;
    if (dir === 'first') target = 0;
    if (dir === 'last') target = columnSubcolumns.length - 1;
    if (dir === 'left') target = Math.max(0, idx - 1);
    if (dir === 'right') target = Math.min(columnSubcolumns.length - 1, idx + 1);
    const position = (columnSubcolumns[target]?.position || target + 1);
    try {
      await Api.post(`/api/tasks/subcolumns/${subId}/move`, { position });
      const column = findColumn(subcolumn.column_id);
      if (column) {
        await loadSubcolumns(column.board_id, true);
      }
      renderBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function moveBoardFromUI(boardId, dir) {
    const boards = state.boardsBySpace[state.spaceId] || [];
    const idx = boards.findIndex(b => b.id === boardId);
    if (idx === -1) return;
    let target = idx;
    if (dir === 'first') target = 0;
    if (dir === 'last') target = boards.length - 1;
    if (dir === 'left') target = Math.max(0, idx - 1);
    if (dir === 'right') target = Math.min(boards.length - 1, idx + 1);
    const position = (boards[target]?.position || target + 1);
    try {
      await Api.post(`/api/tasks/boards/${boardId}/move`, { position });
      await loadBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  function getColumnType(column) {
    if (!column?.id) return 'normal';
    try {
      const saved = localStorage.getItem(`tasks.column.type.${column.id}`);
      if (saved) return saved;
    } catch (_) {
      // ignore
    }
    if (column.is_final) return 'done';
    return 'normal';
  }

  function setColumnType(columnId, type) {
    if (!columnId) return;
    try {
      localStorage.setItem(`tasks.column.type.${columnId}`, type);
    } catch (_) {
      // ignore
    }
  }

  function columnTypeLabel(type) {
    const map = {
      normal: 'tasks.columnTypes.normal',
      waiting: 'tasks.columnTypes.waiting',
      in_progress: 'tasks.columnTypes.inProgress',
      rework: 'tasks.columnTypes.rework',
      done: 'tasks.columnTypes.done'
    };
    const key = map[type] || map.normal;
    return t(key);
  }

  function renderTaskCard(task) {
    const tpl = document.getElementById('tasks-card-template');
    const card = tpl?.content.firstElementChild.cloneNode(true);
    if (!card) return document.createElement('div');
    card.dataset.taskId = task.id;
    if (task.closed_at) card.classList.add('task-closed');
    if (task.is_archived) card.classList.add('task-archived');
    if (task.is_blocked) card.classList.add('task-blocked');
      const title = card.querySelector('.task-card-title');
      const meta = card.querySelector('.task-card-meta');
      const tags = card.querySelector('.task-card-tags');
      const blocker = card.querySelector('.task-card-blocker');
      const sizeEl = card.querySelector('.task-card-size');
      const assignees = card.querySelector('.task-card-assignees');
      if (title) title.textContent = task.title || '';
      if (meta) {
        const parts = [];
        parts.push(t(`tasks.priority.${(task.priority || 'medium').toLowerCase()}`));
        if (task.due_date) {
          const short = formatDateShort(task.due_date);
          if (short) parts.push(`${t('tasks.fields.dueDateShort')}: ${short}`);
        }
        if (task.is_blocked) {
          parts.push(t('tasks.blocks.badge'));
        }
        meta.textContent = parts.filter(Boolean).join(' - ');
      }
      if (tags) {
        tags.innerHTML = '';
        (task.tags || []).forEach(tag => {
          const chip = document.createElement('span');
          chip.className = 'task-tag';
          chip.textContent = tag;
          if (applyTagStyle) applyTagStyle(chip, tag);
          tags.appendChild(chip);
        });
      }
      if (sizeEl) {
        if (typeof task.size_estimate === 'number') {
          sizeEl.hidden = false;
          sizeEl.textContent = `${t('tasks.fields.sizeShort')}: ${task.size_estimate}`;
        } else {
          sizeEl.hidden = true;
          sizeEl.textContent = '';
        }
      }
      if (blocker) {
        const blockedBy = Array.isArray(task.blocked_by_tasks) ? task.blocked_by_tasks : [];
        if (blockedBy.length) {
          const blockerId = blockedBy[0];
          const blockerTask = state.taskMap.get(blockerId);
          const title = blockerTask?.title ? blockerTask.title : `#${blockerId}`;
          blocker.hidden = false;
          blocker.textContent = `${t('tasks.blocks.blockedByCard')} «${title}»`;
          blocker.onclick = (e) => {
            e.preventDefault();
            e.stopPropagation();
            openTaskById(blockerId);
          };
        } else {
          blocker.hidden = true;
          blocker.textContent = '';
          blocker.onclick = null;
        }
      }
      if (assignees) renderAssignees(assignees, task.assigned_to || []);
    const menuBtn = card.querySelector('.task-card-menu');
    if (menuBtn) {
      menuBtn.addEventListener('click', (e) => {
        e.preventDefault();
        e.stopPropagation();
        showContextMenu(e.clientX, e.clientY, buildTaskActions(task));
      });
    }
    card.addEventListener('contextmenu', (e) => {
      e.preventDefault();
      e.stopPropagation();
      showContextMenu(e.clientX, e.clientY, buildTaskActions(task));
    });
    card.addEventListener('click', (e) => {
      if (card.dataset.suppressClick) {
        delete card.dataset.suppressClick;
        e.preventDefault();
        e.stopPropagation();
        return;
      }
      openTaskById(task.id);
    });
    const canDrag = hasPermission('tasks.move') && !task.closed_at && !task.is_archived;
    if (canDrag) {
      bindTaskDrag(card, task);
    }
    return card;
  }

  function renderAssignees(container, ids) {
    const dir = (typeof window !== 'undefined' && window.UserDirectory)
      ? window.UserDirectory
      : (typeof UserDirectory !== 'undefined' ? UserDirectory : null);
    container.innerHTML = '';
    if (!ids.length) return;
    const max = 3;
    ids.slice(0, max).forEach(id => {
      const name = dir && dir.name ? dir.name(id) : `#${id}`;
      const badge = document.createElement('div');
      badge.className = 'task-assignee';
      badge.title = name;
      badge.textContent = initials(name);
      container.appendChild(badge);
    });
    if (ids.length > max) {
      const more = document.createElement('div');
      more.className = 'task-assignee task-assignee-more';
      more.textContent = `+${ids.length - max}`;
      container.appendChild(more);
    }
  }

  function initials(name) {
    const parts = (name || '').trim().split(/\s+/).filter(Boolean);
    if (!parts.length) return '?';
    const first = parts[0][0] || '';
    const second = parts.length > 1 ? parts[1][0] : '';
    return `${first}${second}`.toUpperCase();
  }

  function buildTaskActions(task) {
    const actions = [];
    if (hasPermission('tasks.create')) {
      actions.push({ label: t('tasks.actions.clone'), handler: () => cloneTask(task.id) });
    }
    if (hasPermission('tasks.move')) {
      actions.push({ label: t('tasks.actions.moveTask'), handler: () => openTaskMoveModal(task.id) });
    }
    if (hasPermission('tasks.close') && !task.closed_at) {
      actions.push({ label: t('tasks.actions.closeTask'), handler: () => closeTask(task.id) });
    }
    if (hasPermission('tasks.archive')) {
      actions.push({ label: t('tasks.actions.archive'), danger: true, handler: () => archiveTask(task.id) });
    }
    if (hasPermission('tasks.archive')) {
      actions.push({ label: t('common.delete'), danger: true, handler: () => deleteTask(task.id) });
    }
    return actions;
  }
  function bindBoardDrag(container, spaceId) {
    if (!hasPermission('tasks.manage') && !hasPermission('tasks.create')) return;
    if (!container || container.dataset.boardDragInit) return;
    container.dataset.boardDragInit = '1';
    container.addEventListener('pointerdown', (e) => {
      if (e.button !== 0) return;
      if (state.drag.active) return;
      const handle = e.target.closest('.tasks-board-drag');
      const header = e.target.closest('.tasks-board-header');
      if (!handle && !header) return;
      if (!handle && header && e.target.closest('button')) return;
      const boardEl = (handle || header).closest('.tasks-board-frame');
      if (!boardEl) return;
      startBoardDrag(boardEl, e, container, spaceId);
    }, true);
  }

  function startBoardDrag(boardEl, e, container, spaceId) {
    e.preventDefault();
    e.stopPropagation();
    const boardId = parseInt(boardEl.dataset.boardId || '0', 10);
    if (!boardId) return;
    const containerRect = container.getBoundingClientRect();
    const rect = boardEl.getBoundingClientRect();
    const startX = e.clientX;
    const startY = e.clientY;
    const startLeft = parseFloat(boardEl.dataset.posX || `${rect.left - containerRect.left}`);
    const startTop = parseFloat(boardEl.dataset.posY || `${rect.top - containerRect.top}`);
    const offsetX = startX - rect.left;
    const offsetY = startY - rect.top;
      const pointerId = e.pointerId;
      let dragging = false;
      let ghost = null;
      let placeholder = null;
      let targetColumn = null;
      let targetIndex = null;
      let layout = null;
      let lastPreview = null;
      const dragId = boardEl.dataset.boardId;

    const startDrag = () => {
      dragging = true;
      state.drag.active = true;
      state.drag.taskId = null;
      state.drag.boardId = boardId;
      state.drag.type = 'board';
      ghost = boardEl.cloneNode(true);
      ghost.classList.add('tasks-board-ghost');
      ghost.style.display = getComputedStyle(boardEl).display;
      ghost.style.boxSizing = 'border-box';
      ghost.style.overflow = 'hidden';
      ghost.style.position = 'fixed';
      ghost.style.pointerEvents = 'none';
      ghost.style.zIndex = '9999';
      ghost.style.opacity = '0.95';
      ghost.style.width = `${rect.width}px`;
      ghost.style.height = `${rect.height}px`;
      ghost.style.maxWidth = `${rect.width}px`;
      ghost.style.maxHeight = `${rect.height}px`;
      ghost.style.left = `${rect.left}px`;
      ghost.style.top = `${rect.top}px`;
      ghost.style.visibility = 'visible';
      ghost.style.transform = 'translateZ(0)';
      const ghostHost = document.getElementById('tasks-page') || document.body;
      ghostHost.appendChild(ghost);
      boardEl.classList.add('dragging');
      boardEl.style.opacity = '0.2';
      layout = normalizeBoardLayout(spaceId, container);
      placeholder = getPlaceholder('tasks-board-placeholder');
      placeholder.style.position = 'absolute';
      placeholder.style.width = `${rect.width}px`;
      placeholder.style.height = `${rect.height}px`;
      placeholder.style.left = `${startLeft}px`;
      placeholder.style.top = `${startTop}px`;
      placeholder.style.display = 'block';
      placeholder.style.opacity = '1';
      placeholder.style.pointerEvents = 'none';
      container.appendChild(placeholder);
    };

    const onMove = (evt) => {
      if (evt.pointerId !== pointerId) return;
      const dx = Math.abs(evt.clientX - startX);
      const dy = Math.abs(evt.clientY - startY);
      if (!dragging && (dx > 4 || dy > 4)) startDrag();
      if (!dragging) return;
      evt.preventDefault();
        const base = computeBoardPositions(layout, container, { excludeId: dragId });
        const localX = evt.clientX - containerRect.left;
        const localY = evt.clientY - containerRect.top;
        let col = getBoardColumnAtX(localX, base.widths, base.metrics.gap);
        if (col < 0) col = 0;
        targetColumn = col;
        const columnBoards = base.columns[targetColumn] || [];
        targetIndex = getBoardInsertIndex(columnBoards, localY, base.positions);
        normalizeColumnIndexes(layout);
        const preview = computeBoardPositions(layout, container, {
          excludeId: dragId,
          draggingId: dragId,
          targetColumn,
          targetIndex
        });
        lastPreview = preview;
        if (placeholder) {
          const pos = preview.positions[dragId];
          if (pos) {
            placeholder.style.left = `${pos.x}px`;
            placeholder.style.top = `${pos.y}px`;
            placeholder.style.width = `${pos.width}px`;
            placeholder.style.height = `${pos.height}px`;
          }
        }
        ghost.style.left = `${evt.clientX - offsetX}px`;
        ghost.style.top = `${evt.clientY - offsetY}px`;
        if (preview.positions[dragId]) {
          ensureBoardShellSize(container, preview.metrics, preview.positions[dragId]);
        }
      };

    const onUp = (evt) => {
      if (evt.pointerId !== pointerId) return;
      document.removeEventListener('pointermove', onMove, true);
      document.removeEventListener('pointerup', onUp, true);
      if (!dragging) return;
      if (ghost) ghost.remove();
      if (placeholder) placeholder.remove();
      boardEl.classList.remove('dragging');
      boardEl.style.opacity = '';
      state.drag.active = false;
      state.drag.boardId = null;
      state.drag.type = null;
        if (layout && typeof targetColumn === 'number' && typeof targetIndex === 'number') {
          const preview = lastPreview || computeBoardPositions(layout, container, {
            excludeId: dragId,
            draggingId: dragId,
            targetColumn,
            targetIndex
          });
          rebuildLayoutFromColumns(layout, preview.columns);
          normalizeColumnIndexes(layout);
          saveBoardLayout(spaceId, layout);
        }
      measureBoardSizes(container);
      applyBoardLayout(container, spaceId);
      syncBoardOrder(container, boardId);
    };

    document.addEventListener('pointermove', onMove, true);
    document.addEventListener('pointerup', onUp, true);
  }

  function measureBoardSizes(container) {
    container.querySelectorAll('.tasks-board-frame').forEach(board => {
      const rect = board.getBoundingClientRect();
      const collapsed = board.classList.contains('collapsed');
      if (!collapsed && rect.width && rect.height) {
        board.dataset.fullW = `${rect.width}`;
        board.dataset.fullH = `${rect.height}`;
      }
      const fullW = parseFloat(board.dataset.fullW || `${rect.width || 280}`);
      const fullH = parseFloat(board.dataset.fullH || `${rect.height || 240}`);
      board.dataset.boardW = `${fullW}`;
      board.dataset.boardH = `${fullH}`;
      if (collapsed) {
        board.style.width = `${fullW}px`;
      } else {
        board.style.width = '';
      }
    });
  }

    function getBoardMetrics(container) {
      if (!container) return { gap: 8 };
      const style = getComputedStyle(container);
      const rawGap = style.getPropertyValue('--tasks-board-gap');
      const gap = Number.isFinite(parseFloat(rawGap)) ? parseFloat(rawGap) : 8;
      return { gap };
    }

  function normalizeBoardLayout(spaceId, container) {
    const layout = getBoardLayout(spaceId);
    const boardIds = [...container.querySelectorAll('.tasks-board-frame')].map(el => el.dataset.boardId);
    layout.order = layout.order || [];
    layout.columns = layout.columns || {};
    boardIds.forEach(id => {
      if (!layout.order.includes(id)) layout.order.push(id);
      if (typeof layout.columns[id] !== 'number') layout.columns[id] = null;
    });
    layout.order = layout.order.filter(id => boardIds.includes(id));
    return layout;
  }

  function normalizeColumnIndexes(layout) {
    const used = Array.from(new Set(Object.values(layout.columns).filter(v => typeof v === 'number'))).sort((a, b) => a - b);
    const map = new Map(used.map((val, idx) => [val, idx]));
    Object.keys(layout.columns).forEach(id => {
      if (map.has(layout.columns[id])) {
        layout.columns[id] = map.get(layout.columns[id]);
      }
    });
  }

  function getMaxUsedColumn(layout) {
    const values = Object.values(layout.columns).filter(v => typeof v === 'number');
    if (!values.length) return -1;
    return Math.max(...values);
  }

    function computeColumnX(col, widths, gap) {
      let x = 0;
      for (let i = 0; i < col; i += 1) {
        x += (widths[i] || 0) + gap;
      }
      return x;
    }

    function buildBoardColumns(layout, metrics, options = {}) {
      const excludeId = options.excludeId || null;
      const columnCount = Math.max(1, getMaxUsedColumn(layout) + 1);
      const columns = Array.from({ length: columnCount }, () => []);
      const widths = Array(columnCount).fill(0);
      const heights = Array(columnCount).fill(0);
      layout.order.forEach(id => {
        if (id === excludeId) return;
        const el = document.querySelector(`.tasks-board-frame[data-board-id="${id}"]`);
        if (!el) return;
        const w = parseFloat(el.dataset.boardW || '280');
        const h = parseFloat(el.dataset.boardH || '240');
        let col = layout.columns[id];
        if (typeof col !== 'number' || col < 0 || col >= columnCount) {
          col = heights.indexOf(Math.min(...heights));
          layout.columns[id] = col;
        }
        const isTopInColumn = columns[col].length === 0;
        columns[col].push(id);
        if (isTopInColumn) {
          // Keep top-row spacing compact: lower wide boards must not push neighbor columns away.
          widths[col] = w;
        }
        heights[col] += h + metrics.gap;
      });
      return { columns, widths };
    }

    function computeBoardPositions(layout, container, options = {}) {
      const metrics = getBoardMetrics(container);
      const base = buildBoardColumns(layout, metrics, { excludeId: options.excludeId });
      let columns = base.columns;
      let widths = base.widths;
      if (options.draggingId) {
        let col = typeof options.targetColumn === 'number' ? options.targetColumn : columns.length - 1;
        if (col < 0) col = 0;
        if (col > columns.length) {
          while (columns.length <= col) {
            columns.push([]);
            widths.push(0);
          }
        }
        const list = columns[col] || (columns[col] = []);
        const dragEl = document.querySelector(`.tasks-board-frame[data-board-id="${options.draggingId}"]`);
        const dragW = parseFloat(dragEl?.dataset.boardW || '280');
        const dragH = parseFloat(dragEl?.dataset.boardH || '240');
        const insertAt = typeof options.targetIndex === 'number'
          ? Math.max(0, Math.min(list.length, options.targetIndex))
          : list.length;
        list.splice(insertAt, 0, options.draggingId);
        if (insertAt === 0 || list.length === 1) {
          widths[col] = dragW;
        }
      }
      const positions = {};
      const heights = Array(columns.length).fill(0);
      columns.forEach((ids, col) => {
        const x = computeColumnX(col, widths, metrics.gap);
        ids.forEach(id => {
          const el = document.querySelector(`.tasks-board-frame[data-board-id="${id}"]`);
          if (!el) return;
          const w = parseFloat(el.dataset.boardW || '280');
          const h = parseFloat(el.dataset.boardH || '240');
          const y = heights[col];
          positions[id] = { x, y, width: w, height: h };
          heights[col] += h + metrics.gap;
        });
      });
      return { metrics, columns, widths, positions, heights };
    }

    function getBoardColumnAtX(localX, widths, gap) {
      let cursor = 0;
      for (let col = 0; col < widths.length; col += 1) {
        const end = cursor + (widths[col] || 0);
        if (localX <= end + gap * 0.5) return col;
        cursor = end + gap;
      }
      return widths.length;
    }

    function getBoardInsertIndex(columnBoards, localY, positions) {
      for (let i = 0; i < columnBoards.length; i += 1) {
        const id = columnBoards[i];
        const pos = positions[id];
        if (!pos) continue;
        if (localY < pos.y + pos.height / 2) return i;
      }
      return columnBoards.length;
    }

    function rebuildLayoutFromColumns(layout, columns) {
      layout.order = [];
      layout.columns = {};
      columns.forEach((ids, col) => {
        ids.forEach(id => {
          layout.order.push(id);
          layout.columns[id] = col;
        });
      });
    }

    function applyBoardLayout(container, spaceId) {
      if (!container) return;
      const layout = normalizeBoardLayout(spaceId, container);
      normalizeColumnIndexes(layout);
      const computed = computeBoardPositions(layout, container);
      computed.columns.forEach((ids, col) => {
        ids.forEach(id => {
          const pos = computed.positions[id];
          const el = container.querySelector(`.tasks-board-frame[data-board-id="${id}"]`);
          if (!el || !pos) return;
          el.style.left = `${pos.x}px`;
          el.style.top = `${pos.y}px`;
          el.dataset.posX = `${pos.x}`;
          el.dataset.posY = `${pos.y}`;
        });
      });
      saveBoardLayout(spaceId, layout);
      ensureBoardShellSize(container, { ...computed.metrics, widths: computed.widths });
    }

  function ensureBoardShellSize(container, metrics, preview) {
    const boards = [...container.querySelectorAll('.tasks-board-frame')];
    let maxRight = 0;
    let maxBottom = 0;
    boards.forEach(board => {
      const w = parseFloat(board.dataset.boardW || '280');
      const h = parseFloat(board.dataset.boardH || '240');
      const x = parseFloat(board.dataset.posX || '0');
      const y = parseFloat(board.dataset.posY || '0');
      maxRight = Math.max(maxRight, x + w);
      maxBottom = Math.max(maxBottom, y + h);
    });
    if (preview) {
      maxRight = Math.max(maxRight, preview.x + (preview.width || 280));
      maxBottom = Math.max(maxBottom, preview.y + (preview.height || 240));
    }
    container.style.minWidth = `${Math.max(maxRight + metrics.gap * 2, container.clientWidth)}px`;
    container.style.minHeight = `${Math.max(maxBottom + metrics.gap * 2, 240)}px`;
  }

  async function loadBoardLayout(spaceId) {
    if (!spaceId || boardLayoutLoaded.has(spaceId)) return;
    boardLayoutLoaded.add(spaceId);
    let serverLayout = null;
    try {
      const res = await Api.get(`/api/tasks/spaces/${spaceId}/layout`);
      if (res && res.layout) serverLayout = res.layout;
    } catch (_) {
      // ignore and fall back to local
    }
    const localLayout = getLocalBoardLayout(spaceId);
    if (serverLayout && serverLayout.order && serverLayout.columns) {
      boardLayouts[spaceId] = serverLayout;
      localStorage.setItem(`tasks.board.layout.${spaceId}`, JSON.stringify(serverLayout));
      return;
    }
    if (localLayout) {
      boardLayouts[spaceId] = localLayout;
      queueBoardLayoutSave(spaceId, localLayout);
    }
  }

  function getLocalBoardLayout(spaceId) {
    const raw = localStorage.getItem(`tasks.board.layout.${spaceId}`);
    if (!raw) return null;
    try {
      const parsed = JSON.parse(raw);
      if (parsed && parsed.columns && parsed.order) return parsed;
      return null;
    } catch {
      return null;
    }
  }

  function getBoardLayout(spaceId) {
    if (boardLayouts[spaceId]) return boardLayouts[spaceId];
    const local = getLocalBoardLayout(spaceId);
    if (local) {
      boardLayouts[spaceId] = local;
      return local;
    }
    const empty = { order: [], columns: {} };
    boardLayouts[spaceId] = empty;
    return empty;
  }

  function saveBoardLayout(spaceId, layout) {
    if (!spaceId || !layout) return;
    boardLayouts[spaceId] = layout;
    localStorage.setItem(`tasks.board.layout.${spaceId}`, JSON.stringify(layout));
    queueBoardLayoutSave(spaceId, layout);
  }

  function queueBoardLayoutSave(spaceId, layout) {
    if (!spaceId || !layout) return;
    if (boardLayoutTimers[spaceId]) clearTimeout(boardLayoutTimers[spaceId]);
    boardLayoutTimers[spaceId] = setTimeout(async () => {
      try {
        await Api.post(`/api/tasks/spaces/${spaceId}/layout`, { layout });
      } catch (_) {
        // ignore
      }
    }, 600);
  }

  function moveBoardInOrder(order, boardId, columns, targetColumn) {
    const next = order.filter(id => id !== boardId);
    let insertAt = -1;
    for (let i = next.length - 1; i >= 0; i -= 1) {
      const id = next[i];
      if (columns[id] === targetColumn) {
        insertAt = i + 1;
        break;
      }
    }
    if (insertAt < 0) insertAt = next.length;
    next.splice(insertAt, 0, boardId);
    return next;
  }

  function syncBoardOrder(container, draggedId) {
    const layout = getBoardLayout(state.spaceId);
    if (!layout.order.length) return;
    const idx = layout.order.indexOf(`${draggedId}`);
    if (idx < 0) return;
    Api.post(`/api/tasks/boards/${draggedId}/move`, { position: idx + 1 })
      .then(() => loadBoards(state.spaceId))
      .catch(err => showError(err, 'common.error'));
  }

  function isBoardCollapsed(boardId) {
    return localStorage.getItem(`tasks.board.collapsed.${boardId}`) === '1';
  }

  function setBoardCollapsed(boardId, value) {
    localStorage.setItem(`tasks.board.collapsed.${boardId}`, value ? '1' : '0');
  }

  function bindSpaceDrag(card, list) {
    card.addEventListener('mousedown', (e) => {
      if (e.button !== 0) return;
      if (state.drag.active) return;
      if (e.target.closest('button')) return;
      const spaceId = parseInt(card.dataset.spaceId || '0', 10);
      if (!spaceId) return;
      const rect = card.getBoundingClientRect();
      const placeholder = getPlaceholder('tasks-space-placeholder');
      const offsetX = e.clientX - rect.left;
      const offsetY = e.clientY - rect.top;
      let dragging = false;
      let ghost = null;

      const startDrag = () => {
        dragging = true;
        state.drag.active = true;
        state.drag.type = 'space';
        state.drag.spaceId = spaceId;
        placeholder.style.height = `${rect.height}px`;
        list.insertBefore(placeholder, card);
        card.classList.add('dragging');
        card.style.opacity = '0.15';
        ghost = card.cloneNode(true);
        ghost.classList.add('tasks-board-ghost');
        ghost.style.position = 'fixed';
        ghost.style.pointerEvents = 'none';
        ghost.style.zIndex = '9999';
        ghost.style.opacity = '0.95';
        ghost.style.width = `${rect.width}px`;
        ghost.style.height = `${rect.height}px`;
        ghost.style.maxWidth = `${rect.width}px`;
        ghost.style.maxHeight = `${rect.height}px`;
        ghost.style.left = `${rect.left}px`;
        ghost.style.top = `${rect.top}px`;
        ghost.style.visibility = 'visible';
        document.body.appendChild(ghost);
        card.dataset.suppressClick = '1';
      };

      const onMove = (evt) => {
        const dx = Math.abs(evt.clientX - e.clientX);
        const dy = Math.abs(evt.clientY - e.clientY);
        if (!dragging && (dx > 4 || dy > 4)) startDrag();
        if (!dragging || !state.drag.active || state.drag.type !== 'space') return;
        ghost.style.left = `${evt.clientX - offsetX}px`;
        ghost.style.top = `${evt.clientY - offsetY}px`;
        const afterEl = getSpaceDragAfterElement(list, evt.clientY);
        if (!afterEl) {
          list.appendChild(placeholder);
        } else {
          list.insertBefore(placeholder, afterEl);
        }
      };

      const onUp = () => {
        window.removeEventListener('mousemove', onMove);
        if (!dragging) return;
        if (ghost) ghost.remove();
        card.classList.remove('dragging');
        card.style.opacity = '';
        delete card.dataset.suppressClick;
        state.drag.active = false;
        state.drag.type = null;
        state.drag.spaceId = null;
        const currentPlaceholder = getPlaceholder('tasks-space-placeholder');
        if (currentPlaceholder.parentElement === list) {
          currentPlaceholder.replaceWith(card);
        } else {
          list.appendChild(card);
        }
        const ids = Array.from(list.querySelectorAll('.tasks-space-card'))
          .map(node => parseInt(node.dataset.spaceId || '0', 10))
          .filter(Boolean);
        setSpaceOrder(ids);
        renderSpaceList();
      };

      window.addEventListener('mousemove', onMove);
      window.addEventListener('mouseup', onUp, { once: true });
    });
  }

  function bindTaskDrag(card, task) {
    card.addEventListener('mousedown', (e) => {
      if (e.button !== 0) return;
      if (state.drag.active) return;
      if (e.target.closest('button')) return;
      card.dataset.dragReady = '1';
      const startX = e.clientX;
      const startY = e.clientY;
      let dragging = false;
      let ghost = null;
      const rect = card.getBoundingClientRect();
      const offsetX = startX - rect.left;
      const offsetY = startY - rect.top;
      const placeholder = getPlaceholder('tasks-drop-placeholder');
      placeholder.style.width = `${rect.width}px`;
      placeholder.style.height = `${rect.height}px`;
      let lastContainer = null;

      const startDrag = () => {
        dragging = true;
        state.drag.active = true;
        state.drag.taskId = task.id;
        state.drag.type = 'task';
        card.dataset.suppressClick = '1';
        card.classList.add('dragging');
        ghost = card.cloneNode(true);
        ghost.classList.add('task-drag-ghost');
        ghost.style.display = getComputedStyle(card).display;
        ghost.style.boxSizing = 'border-box';
        ghost.style.overflow = 'hidden';
        ghost.style.position = 'fixed';
        ghost.style.pointerEvents = 'none';
        ghost.style.zIndex = '9999';
        ghost.style.opacity = '0.95';
        ghost.style.width = `${rect.width}px`;
        ghost.style.height = `${rect.height}px`;
        ghost.style.maxWidth = `${rect.width}px`;
        ghost.style.maxHeight = `${rect.height}px`;
        ghost.style.left = `${startX - offsetX}px`;
        ghost.style.top = `${startY - offsetY}px`;
        ghost.style.visibility = 'visible';
        ghost.style.transform = 'translateZ(0)';
        const ghostHost = document.getElementById('tasks-page') || document.body;
        ghostHost.appendChild(ghost);
        card.style.display = 'none';
        card.after(placeholder);
        lastContainer = placeholder.parentElement;
      };

      const onMove = (evt) => {
        const dx = Math.abs(evt.clientX - startX);
        const dy = Math.abs(evt.clientY - startY);
        if (!dragging && (dx > 4 || dy > 4)) startDrag();
        if (!dragging) return;
        ghost.style.left = `${evt.clientX - offsetX}px`;
        ghost.style.top = `${evt.clientY - offsetY}px`;
        const hit = getTaskDropTarget(evt.clientX, evt.clientY);
        if (!hit || !hit.container) return;
        const afterEl = getDragAfterElement(hit.container, evt.clientY, evt.clientX);
        if (!afterEl) {
          const anchor = getTaskAppendAnchor(hit.container);
          if (anchor) {
            hit.container.insertBefore(placeholder, anchor);
          } else {
            hit.container.appendChild(placeholder);
          }
        } else {
          hit.container.insertBefore(placeholder, afterEl);
        }
        lastContainer = hit.container;
      };

      const onUp = () => {
        window.removeEventListener('mousemove', onMove);
        if (!dragging) return;
        if (ghost) ghost.remove();
        card.classList.remove('dragging');
        card.style.display = '';
        delete card.dataset.dragReady;
        if (lastContainer && placeholder.parentElement === lastContainer) {
          dropTaskInto(lastContainer, task.id);
        } else if (placeholder.parentElement) {
          placeholder.replaceWith(card);
          cleanupPlaceholder();
          state.drag.active = false;
          state.drag.taskId = null;
          state.drag.type = null;
        } else {
          state.drag.active = false;
          state.drag.taskId = null;
          state.drag.type = null;
        }
      };

      window.addEventListener('mousemove', onMove);
      window.addEventListener('mouseup', onUp, { once: true });
    });
  }

  function getTaskDropTarget(x, y) {
    const el = document.elementFromPoint(x, y);
    if (!el) return null;
    const subBody = el.closest('[data-subcolumn-body]');
    if (subBody) {
      const columnId = parseInt(subBody.closest('.tasks-column')?.dataset.columnId || '0', 10);
      const subcolumnId = parseInt(subBody.closest('.tasks-subcolumn')?.dataset.subcolumnId || '0', 10);
      if (!columnId || !subcolumnId) return null;
      return { container: subBody, columnId, subcolumnId };
    }
    const colBody = el.closest('[data-column-body]');
    if (colBody) {
      const columnId = parseInt(colBody.closest('.tasks-column')?.dataset.columnId || '0', 10);
      if (!columnId) return null;
      if ((state.subcolumnsByColumn[columnId] || []).length > 0) return null;
      return { container: colBody, columnId, subcolumnId: 0 };
    }
    return null;
  }

  function getTaskAppendAnchor(container) {
    return container.querySelector('.tasks-column-add, .tasks-subcolumn-add');
  }

  function dropTaskInto(container, taskId) {
    const columnId = parseInt(container.closest('.tasks-column')?.dataset.columnId || '0', 10);
    const subcolumnId = parseInt(container.closest('.tasks-subcolumn')?.dataset.subcolumnId || '0', 10);
    const task = state.taskMap.get(taskId);
    if (!columnId || !task) {
      onDragEnd({ target: container });
      return;
    }
    const hasSubcolumns = (state.subcolumnsByColumn[columnId] || []).length > 0;
    if (hasSubcolumns && !subcolumnId) {
      onDragEnd({ target: container });
      return;
    }
    const targetBoardId = findBoardIdByColumn(columnId) || task.board_id;
    const placeholder = getPlaceholder('tasks-drop-placeholder');
    const card = document.querySelector(`.task-card[data-task-id="${taskId}"]`);
    if (placeholder.parentElement === container) {
      placeholder.replaceWith(card);
    } else {
      const anchor = getTaskAppendAnchor(container);
      if (anchor) {
        container.insertBefore(card, anchor);
      } else {
        container.appendChild(card);
      }
    }
    const newIndex = Array.from(container.querySelectorAll('.task-card')).indexOf(card) + 1;
    cleanupPlaceholder();
    if (card) card.classList.remove('dragging');
    state.drag.active = false;
    state.drag.taskId = null;
    state.drag.type = null;
    const payload = {
      column_id: columnId,
      position: newIndex
    };
    if (subcolumnId) payload.subcolumn_id = subcolumnId;
    const movePromise = (targetBoardId && targetBoardId !== task.board_id)
      ? Api.post(`/api/tasks/${taskId}/relocate`, { board_id: targetBoardId, ...payload })
      : Api.post(`/api/tasks/${taskId}/move`, payload);
    movePromise
      .then(async (res) => {
        if (res && res.id) {
          state.taskMap.set(res.id, res);
          if (res.board_id !== task.board_id) {
            await Promise.all([loadTasks(task.board_id), loadTasks(res.board_id)]);
          } else {
            await loadTasks(res.board_id);
          }
          renderBoards(state.spaceId);
        }
      })
      .catch(async (err) => {
        showError(err, 'tasks.errors.moveFailed');
        await loadTasks(task.board_id);
        renderBoards(state.spaceId);
      });
  }

  function getBoardAfterElement(container, x, y) {
    const elements = [...container.querySelectorAll('.tasks-board-frame:not(.dragging)')];
    if (!elements.length) return null;
    const items = elements.map(el => {
      const rect = el.getBoundingClientRect();
      const centerX = rect.left + rect.width / 2;
      const centerY = rect.top + rect.height / 2;
      const dx = centerX - x;
      const dy = centerY - y;
      return { el, rect, centerX, centerY, dist: Math.hypot(dx, dy) };
    });
    let nearest = items[0];
    for (const item of items) {
      if (item.dist < nearest.dist) nearest = item;
    }
    if (y < nearest.centerY || (y <= nearest.rect.bottom && x < nearest.centerX)) {
      return nearest.el;
    }
    return nearest.el.nextElementSibling;
  }

  function getBoardDropPosition(container, x, y) {
    const hit = document.elementFromPoint(x, y);
    const spacer = hit?.closest('.tasks-board-spacer');
    if (spacer) {
      const elements = [...container.querySelectorAll('.tasks-board-frame:not(.dragging)')];
      if (!elements.length) return null;
      return { ref: elements[elements.length - 1], before: false };
    }
    const hovered = hit?.closest('.tasks-board-frame:not(.dragging)');
    if (hovered) {
      const rect = hovered.getBoundingClientRect();
      const dxLeft = Math.abs(x - rect.left);
      const dxRight = Math.abs(rect.right - x);
      const dyTop = Math.abs(y - rect.top);
      const dyBottom = Math.abs(rect.bottom - y);
      const min = Math.min(dxLeft, dxRight, dyTop, dyBottom);
      if (min === dyTop) return { ref: hovered, before: true };
      if (min === dyBottom) return { ref: hovered, before: false };
      if (min === dxLeft) return { ref: hovered, before: true };
      return { ref: hovered, before: false };
    }

    const elements = [...container.querySelectorAll('.tasks-board-frame:not(.dragging)')];
    if (!elements.length) return null;
    let nearest = null;
    let nearestDist = Infinity;
    let lastEl = null;
    let maxBottom = -Infinity;
    elements.forEach(el => {
      const rect = el.getBoundingClientRect();
      if (rect.bottom > maxBottom) {
        maxBottom = rect.bottom;
        lastEl = el;
      }
      const dx = x < rect.left ? rect.left - x : (x > rect.right ? x - rect.right : 0);
      const dy = y < rect.top ? rect.top - y : (y > rect.bottom ? y - rect.bottom : 0);
      const dist = Math.hypot(dx, dy);
      if (dist < nearestDist) {
        nearest = el;
        nearestDist = dist;
      }
    });
    if (!nearest) return null;
    if (y > maxBottom + 8 && lastEl) {
      return { ref: lastEl, before: false };
    }
    const rect = nearest.getBoundingClientRect();
    const dxLeft = Math.abs(x - rect.left);
    const dxRight = Math.abs(rect.right - x);
    const dyTop = Math.abs(y - rect.top);
    const dyBottom = Math.abs(rect.bottom - y);
    const min = Math.min(dxLeft, dxRight, dyTop, dyBottom);
    if (min === dyTop) return { ref: nearest, before: true };
    if (min === dyBottom) return { ref: nearest, before: false };
    if (min === dxLeft) return { ref: nearest, before: true };
    return { ref: nearest, before: false };
  }


  function bindTaskDrop(container) {
    container.addEventListener('dragover', onDragOver);
    container.addEventListener('drop', onDrop);
  }

  function onDragStart(e) {
    const card = e.target.closest('.task-card');
    if (!card) return;
    const taskId = parseInt(card.dataset.taskId || '0', 10);
    if (!taskId) return;
    state.drag.active = true;
    state.drag.taskId = taskId;
    state.drag.boardId = null;
    card.classList.add('dragging');
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/plain', `${taskId}`);
    }
  }

  function onDragEnd(e) {
    const card = e.target.closest('.task-card');
    if (card) card.classList.remove('dragging');
    cleanupPlaceholder();
    state.drag.active = false;
    state.drag.taskId = null;
    state.drag.type = null;
  }

  function onDragOver(e) {
    if (!state.drag.active || !state.drag.taskId) return;
    e.preventDefault();
    const container = e.currentTarget;
    const columnId = parseInt(container.closest('.tasks-column')?.dataset.columnId || '0', 10);
    const subcolumnId = parseInt(container.closest('.tasks-subcolumn')?.dataset.subcolumnId || '0', 10);
    if (columnId && !subcolumnId && (state.subcolumnsByColumn[columnId] || []).length > 0) {
      return;
    }
    const afterEl = getDragAfterElement(container, e.clientY);
    const placeholder = getPlaceholder('tasks-drop-placeholder');
    if (afterEl && afterEl.parentElement !== container) {
      container.appendChild(placeholder);
      return;
    }
    if (!afterEl) {
      container.appendChild(placeholder);
    } else {
      container.insertBefore(placeholder, afterEl);
    }
  }

  function onDrop(e) {
    if (!state.drag.active || !state.drag.taskId) return;
    e.preventDefault();
    const container = e.currentTarget;
    const columnId = parseInt(container.closest('.tasks-column')?.dataset.columnId || '0', 10);
    const subcolumnId = parseInt(container.closest('.tasks-subcolumn')?.dataset.subcolumnId || '0', 10);
    const taskId = state.drag.taskId;
    const task = state.taskMap.get(taskId);
    if (!columnId || !task) {
      onDragEnd(e);
      return;
    }
    const hasSubcolumns = (state.subcolumnsByColumn[columnId] || []).length > 0;
    if (hasSubcolumns && !subcolumnId) {
      onDragEnd(e);
      return;
    }
    const targetBoardId = findBoardIdByColumn(columnId) || task.board_id;
    const placeholder = getPlaceholder('tasks-drop-placeholder');
    const card = document.querySelector(`.task-card[data-task-id="${taskId}"]`);
    if (placeholder.parentElement === container) {
      placeholder.replaceWith(card);
    } else {
      container.appendChild(card);
    }
    const newIndex = Array.from(container.querySelectorAll('.task-card')).indexOf(card) + 1;
    cleanupPlaceholder();
    if (card) card.classList.remove('dragging');
    state.drag.active = false;
    state.drag.taskId = null;
    const payload = {
      column_id: columnId,
      position: newIndex
    };
    if (subcolumnId) payload.subcolumn_id = subcolumnId;
    const movePromise = (targetBoardId && targetBoardId !== task.board_id)
      ? Api.post(`/api/tasks/${taskId}/relocate`, { board_id: targetBoardId, ...payload })
      : Api.post(`/api/tasks/${taskId}/move`, payload);
    movePromise
      .then(async (res) => {
        if (res && res.id) {
          state.taskMap.set(res.id, res);
          if (res.board_id !== task.board_id) {
            await Promise.all([loadTasks(task.board_id), loadTasks(res.board_id)]);
          } else {
            await loadTasks(res.board_id);
          }
          renderBoards(state.spaceId);
        }
      })
      .catch(async (err) => {
        showError(err, 'tasks.errors.moveFailed');
        await loadTasks(task.board_id);
        renderBoards(state.spaceId);
      });
  }

  function getPlaceholder(cls) {
    let placeholder = document.querySelector(`.${cls}`);
    if (!placeholder) {
      placeholder = document.createElement('div');
      placeholder.className = cls;
    }
    return placeholder;
  }

  function cleanupPlaceholder() {
    document.querySelectorAll('.tasks-drop-placeholder, .tasks-board-placeholder').forEach(el => el.remove());
  }

  function findBoardIdByColumn(columnId) {
    for (const [boardId, columns] of Object.entries(state.columnsByBoard)) {
      if ((columns || []).some(col => col.id === columnId)) {
        return parseInt(boardId, 10);
      }
    }
    return null;
  }

  function getDragAfterElement(container, y, x) {
    const isBoard = container.classList.contains('tasks-boards');
    const elements = [...container.querySelectorAll(isBoard ? '.tasks-board-frame:not(.dragging)' : '.task-card:not(.dragging)')];
    return elements.reduce((closest, child) => {
      const box = child.getBoundingClientRect();
      const offset = isBoard ? x - box.left - box.width / 2 : y - box.top - box.height / 2;
      if (offset < 0 && offset > closest.offset) {
        return { offset, element: child };
      }
      return closest;
    }, { offset: Number.NEGATIVE_INFINITY, element: null }).element;
  }

  function getSpaceDragAfterElement(container, y) {
    const elements = [...container.querySelectorAll('.tasks-space-card:not(.dragging)')];
    return elements.reduce((closest, child) => {
      const box = child.getBoundingClientRect();
      const offset = y - box.top - box.height / 2;
      if (offset < 0 && offset > closest.offset) {
        return { offset, element: child };
      }
      return closest;
    }, { offset: Number.NEGATIVE_INFINITY, element: null }).element;
  }

  function openSpaceModal(mode, spaceId) {
    if (!hasPermission('tasks.manage')) return;
    spaceModalState = { mode, spaceId: spaceId || null };
    const title = document.getElementById('tasks-space-modal-title');
    const name = document.getElementById('tasks-space-name');
    const desc = document.getElementById('tasks-space-description');
    const users = document.getElementById('tasks-space-users');
    const departments = document.getElementById('tasks-space-departments');
    hideAlert('tasks-space-alert');
    if (mode === 'edit' && spaceId) {
      const space = state.spaceMap[spaceId];
      if (title) title.textContent = t('tasks.actions.editSpace');
      if (name) name.value = space?.name || '';
      if (desc) desc.value = space?.description || '';
      loadSpaceACL(spaceId);
    } else {
      if (title) title.textContent = t('tasks.actions.createSpace');
      if (name) name.value = '';
      if (desc) desc.value = '';
      selectAccessMode('all');
      if (users) users.innerHTML = '';
      if (departments) departments.innerHTML = '';
    }
    populateAccessLists();
    syncAccessMode();
    openModal('tasks-space-modal');
  }

  function selectAccessMode(mode) {
    const input = document.querySelector(`input[name="tasks-space-access"][value="${mode}"]`);
    if (input) input.checked = true;
  }

  async function loadSpaceACL(spaceId) {
    try {
      const res = await Api.get(`/api/tasks/spaces?include_inactive=1`);
      const space = (res.items || []).find(sp => sp.id === spaceId);
      if (!space) return;
    } catch (_) {
      // ignore
    }
  }

  function populateAccessLists() {
    const usersSel = document.getElementById('tasks-space-users');
    const deptSel = document.getElementById('tasks-space-departments');
    const dir = (typeof window !== 'undefined' && window.UserDirectory)
      ? window.UserDirectory
      : (typeof UserDirectory !== 'undefined' ? UserDirectory : null);
    if (usersSel) {
      usersSel.innerHTML = '';
      (dir?.all ? dir.all() : []).forEach(user => {
        const opt = document.createElement('option');
        opt.value = user.username || `${user.id}`;
        opt.textContent = user.full_name || user.username;
        usersSel.appendChild(opt);
      });
    }
    if (deptSel) {
      deptSel.innerHTML = '';
      const departments = new Set();
      (dir?.all ? dir.all() : []).forEach(user => {
        if (user.department) departments.add(user.department);
      });
      Array.from(departments).sort().forEach(dep => {
        const opt = document.createElement('option');
        opt.value = dep;
        opt.textContent = dep;
        deptSel.appendChild(opt);
      });
    }
    if (typeof DocsPage !== 'undefined' && DocsPage.enhanceMultiSelects) {
      DocsPage.enhanceMultiSelects(['tasks-space-users', 'tasks-space-departments']);
    }
    usersSel?.addEventListener('selectionrefresh', () => renderSelectedHint(usersSel, 'tasks-space-users-hint'));
    deptSel?.addEventListener('selectionrefresh', () => renderSelectedHint(deptSel, 'tasks-space-departments-hint'));
    renderSelectedHint(usersSel, 'tasks-space-users-hint');
    renderSelectedHint(deptSel, 'tasks-space-departments-hint');
  }

  function renderSelectedHint(select, hintId) {
    const hint = document.getElementById(hintId);
    if (!select || !hint) return;
    const selected = Array.from(select.selectedOptions).map(o => o.textContent).filter(Boolean);
    hint.textContent = selected.length ? selected.join(', ') : t('tasks.empty.noSelection');
  }
  async function saveSpace() {
    if (!hasPermission('tasks.manage')) return;
    hideAlert('tasks-space-alert');
    const name = (document.getElementById('tasks-space-name')?.value || '').trim();
    const description = (document.getElementById('tasks-space-description')?.value || '').trim();
    let layout = 'row';
    if (spaceModalState.mode === 'edit' && spaceModalState.spaceId) {
      layout = state.spaceMap[spaceModalState.spaceId]?.layout || 'row';
    }
    if (!name) {
      showAlert('tasks-space-alert', t('tasks.spaceNameRequired'));
      return;
    }
    const mode = currentAccessMode();
    const users = Array.from(document.getElementById('tasks-space-users')?.selectedOptions || []).map(o => o.value);
    const departments = Array.from(document.getElementById('tasks-space-departments')?.selectedOptions || []).map(o => o.value);
    let acl = [];
    if (mode === 'all') {
      acl = [{ subject_type: 'all', subject_id: 'all', permission: 'manage' }];
    } else if (mode === 'users') {
      acl = users.map(u => ({ subject_type: 'user', subject_id: u, permission: 'manage' }));
    } else if (mode === 'departments') {
      acl = departments.map(d => ({ subject_type: 'department', subject_id: d, permission: 'manage' }));
    }
    try {
      if (spaceModalState.mode === 'edit' && spaceModalState.spaceId) {
        await Api.put(`/api/tasks/spaces/${spaceModalState.spaceId}`, { name, description, layout, acl });
      } else {
        await Api.post('/api/tasks/spaces', { name, description, layout, acl });
      }
      closeModal('tasks-space-modal');
      await loadSpaces(spaceModalState.spaceId);
    } catch (err) {
      showAlert('tasks-space-alert', resolveErrorMessage(err, 'common.error'));
    }
  }


  async function deleteSpace(spaceId) {
    if (!hasPermission('tasks.manage') || !spaceId) return;
    const ok = await confirmAction({ message: t('tasks.spaces.deleteConfirm') });
    if (!ok) return;
    try {
      await Api.del(`/api/tasks/spaces/${spaceId}`);
      await loadSpaces();
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  function openBoardModal(mode, board, spaceId) {
    if (!hasPermission('tasks.manage')) return;
    boardModalState = { mode, boardId: board?.id || null, spaceId: spaceId || board?.space_id || state.spaceId };
    const title = document.getElementById('tasks-board-modal-title');
    const name = document.getElementById('tasks-board-name');
    const desc = document.getElementById('tasks-board-description');
    hideAlert('tasks-board-alert');
    if (mode === 'rename' && board) {
      if (title) title.textContent = t('tasks.boards.renameTitle');
      if (name) name.value = board.name || '';
      if (desc) desc.value = board.description || '';
    } else {
      if (title) title.textContent = t('tasks.actions.createBoard');
      if (name) name.value = '';
      if (desc) desc.value = '';
    }
    openModal('tasks-board-modal');
  }

  async function saveBoard() {
    if (!hasPermission('tasks.manage')) return;
    hideAlert('tasks-board-alert');
    const name = (document.getElementById('tasks-board-name')?.value || '').trim();
    const description = (document.getElementById('tasks-board-description')?.value || '').trim();
    if (!name) {
      showAlert('tasks-board-alert', t('tasks.boardNameRequired'));
      return;
    }
    const payload = { name, description, space_id: boardModalState.spaceId };
    try {
      if (boardModalState.mode === 'rename' && boardModalState.boardId) {
        await Api.put(`/api/tasks/boards/${boardModalState.boardId}`, payload);
      } else {
        await Api.post('/api/tasks/boards', payload);
      }
      closeModal('tasks-board-modal');
      await loadBoards(boardModalState.spaceId);
    } catch (err) {
      showAlert('tasks-board-alert', resolveErrorMessage(err, 'common.error'));
    }
  }

  async function deleteBoard(board) {
    if (!hasPermission('tasks.manage')) return;
    const ok = await confirmAction({ message: t('tasks.boards.deleteConfirm') });
    if (!ok) return;
    try {
      await Api.del(`/api/tasks/boards/${board.id}`);
      await loadBoards(board.space_id);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  function openColumnModal(mode, boardId, column) {
    if (!hasPermission('tasks.manage')) return;
    columnModalState = { mode, columnId: column?.id || null, boardId };
    hideAlert('tasks-column-alert');
    const title = document.getElementById('tasks-column-modal-title');
    const name = document.getElementById('tasks-column-name');
    const wip = document.getElementById('tasks-column-wip');
    const typeSel = document.getElementById('tasks-column-type');
    if (typeSel) {
      typeSel.querySelectorAll('option').forEach(opt => {
        const key = opt.dataset.i18n;
        if (key) opt.textContent = t(key);
      });
    }
    if (mode === 'rename' && column) {
      if (title) title.textContent = t('tasks.columns.renameTitle');
      if (name) name.value = column.name || '';
      if (wip) wip.value = column.wip_limit ?? '';
      if (typeSel) typeSel.value = getColumnType(column);
    } else if (mode === 'edit' && column) {
      if (title) title.textContent = t('tasks.columns.editTitle');
      if (name) name.value = column.name || '';
      if (wip) wip.value = column.wip_limit ?? '';
      if (typeSel) typeSel.value = getColumnType(column);
    } else {
      if (title) title.textContent = t('tasks.columns.addTitle');
      if (name) name.value = '';
      if (wip) wip.value = '';
      if (typeSel) typeSel.value = 'normal';
    }
    openModal('tasks-column-modal');
  }

  async function saveColumn() {
    if (!hasPermission('tasks.manage')) return;
    hideAlert('tasks-column-alert');
    const name = (document.getElementById('tasks-column-name')?.value || '').trim();
    const wipRaw = (document.getElementById('tasks-column-wip')?.value || '').trim();
    const type = document.getElementById('tasks-column-type')?.value || 'normal';
    const wipLimit = wipRaw === '' ? null : parseInt(wipRaw, 10);
    if (!name) {
      showAlert('tasks-column-alert', t('tasks.columnNameRequired'));
      return;
    }
    const isFinal = type === 'done';
    try {
      if ((columnModalState.mode === 'rename' || columnModalState.mode === 'edit') && columnModalState.columnId) {
        const payload = { name, is_final: isFinal };
        if (wipLimit !== null && !Number.isNaN(wipLimit)) payload.wip_limit = wipLimit;
        await Api.put(`/api/tasks/columns/${columnModalState.columnId}`, payload);
        setColumnType(columnModalState.columnId, type);
      } else {
        const payload = { name, position: 0, is_final: isFinal };
        if (wipLimit !== null && !Number.isNaN(wipLimit)) payload.wip_limit = wipLimit;
        const created = await Api.post(`/api/tasks/boards/${columnModalState.boardId}/columns`, payload);
        if (created?.id) setColumnType(created.id, type);
      }
      closeModal('tasks-column-modal');
      await loadColumns(columnModalState.boardId, true);
      renderBoards(state.spaceId);
    } catch (err) {
      showAlert('tasks-column-alert', resolveErrorMessage(err, 'common.error'));
    }
  }

  function openSubcolumnModal(mode, columnId, subcolumn) {
    if (!hasPermission('tasks.manage')) return;
    subcolumnModalState = { mode, subcolumnId: subcolumn?.id || null, columnId };
    hideAlert('tasks-subcolumn-alert');
    const title = document.getElementById('tasks-subcolumn-modal-title');
    const name = document.getElementById('tasks-subcolumn-name');
    if (mode === 'edit' && subcolumn) {
      if (title) title.textContent = t('tasks.subcolumns.editTitle');
      if (name) name.value = subcolumn.name || '';
    } else {
      if (title) title.textContent = t('tasks.subcolumns.addTitle');
      if (name) name.value = '';
    }
    openModal('tasks-subcolumn-modal');
  }

  async function saveSubcolumn() {
    if (!hasPermission('tasks.manage')) return;
    hideAlert('tasks-subcolumn-alert');
    const name = (document.getElementById('tasks-subcolumn-name')?.value || '').trim();
    if (!name) {
      showAlert('tasks-subcolumn-alert', t('tasks.subcolumnNameRequired'));
      return;
    }
    try {
      const isEdit = subcolumnModalState.mode === 'edit' && subcolumnModalState.subcolumnId;
      if (isEdit) {
        await Api.put(`/api/tasks/subcolumns/${subcolumnModalState.subcolumnId}`, { name });
      } else {
        await Api.post(`/api/tasks/columns/${subcolumnModalState.columnId}/subcolumns`, { name, position: 0 });
      }
      closeModal('tasks-subcolumn-modal');
      const column = findColumn(subcolumnModalState.columnId);
      if (column) {
        await loadSubcolumns(column.board_id, true);
        if (!isEdit) {
          await moveColumnTasksToLeftmostSubcolumn(column.id);
        }
      }
      renderBoards(state.spaceId);
    } catch (err) {
      showAlert('tasks-subcolumn-alert', resolveErrorMessage(err, 'common.error'));
    }
  }

  async function deleteColumn(column) {
    if (!hasPermission('tasks.manage')) return;
    const tasks = (state.tasksByBoard[column.board_id] || []).filter(t => t.column_id === column.id && !t.is_archived);
    if (tasks.length) {
      showError({ message: 'tasks.columnNotEmpty' }, 'tasks.columnNotEmpty');
      return;
    }
    const ok = await confirmAction({ message: t('tasks.columns.deleteConfirm') });
    if (!ok) return;
    try {
      await Api.del(`/api/tasks/columns/${column.id}`);
      await loadColumns(column.board_id, true);
      renderBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function archiveColumnTasks(column) {
    if (!hasPermission('tasks.archive')) return;
    const ok = await confirmAction({ message: t('tasks.columns.archiveConfirm') });
    if (!ok) return;
    try {
      const res = await Api.post(`/api/tasks/columns/${column.id}/archive`, {});
      await loadTasks(column.board_id);
      renderBoards(state.spaceId);
      if (res && Number.isFinite(res.archived)) {
        window.alert(`${t('tasks.columns.archiveDone')}: ${res.archived}`);
      }
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function deleteSubcolumn(subcolumn) {
    if (!hasPermission('tasks.manage')) return;
    const ok = await confirmAction({ message: t('tasks.subcolumns.deleteConfirm') });
    if (!ok) return;
    try {
      await Api.del(`/api/tasks/subcolumns/${subcolumn.id}`);
      const column = findColumn(subcolumn.column_id);
      if (column) {
        await loadSubcolumns(column.board_id, true);
      }
      renderBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  function findColumn(columnId) {
    for (const boardId in state.columnsByBoard) {
      const col = (state.columnsByBoard[boardId] || []).find(c => c.id === columnId);
      if (col) return col;
    }
    return null;
  }

  function findSubcolumn(subcolumnId) {
    for (const [columnId, subcolumns] of Object.entries(state.subcolumnsByColumn)) {
      const sub = (subcolumns || []).find(sc => sc.id === subcolumnId);
      if (sub) return sub;
    }
    return null;
  }

  async function openTaskById(taskId) {
    if (!taskId) return;
    if (TasksPage.openTask) TasksPage.openTask(taskId);
  }

  function parseTasksRoute() {
    const parts = window.location.pathname.split('/').filter(Boolean);
    if (parts[0] !== 'tasks') return null;
    return parts.slice(1);
  }

  function parsePendingTaskFromPath() {
    const parts = parseTasksRoute();
    if (parts && parts[0] === 'space' && parts[1] && parts[2] === 'task' && parts[3]) {
      return parseInt(parts[3], 10);
    }
    if (parts && parts[0] === 'task' && parts[1]) {
      return parseInt(parts[1], 10);
    }
    const legacy = window.location.hash.replace('#', '').split('/');
    if (legacy[0] === 'tasks' && legacy[1] === 'task' && legacy[2]) {
      return parseInt(legacy[2], 10);
    }
    return null;
  }

  function parseSpaceFromPath() {
    const parts = parseTasksRoute();
    if (parts && parts[0] === 'space' && parts[1] && parts[2] === 'task' && parts[3]) {
      return parseInt(parts[1], 10);
    }
    if (parts && parts[0] === 'space' && parts[1]) {
      return parseInt(parts[1], 10);
    }
    const legacy = window.location.hash.replace('#', '').split('/');
    if (legacy[0] === 'tasks' && legacy[1] === 'space' && legacy[2]) {
      return parseInt(legacy[2], 10);
    }
    return null;
  }

  function updateSpaceHash(spaceId) {
    const next = spaceId ? `/tasks/space/${spaceId}` : '/tasks';
    if (window.location.pathname !== next) {
      window.history.replaceState({}, '', next);
    }
  }

  function updateTaskHash(taskId) {
    let next = '/tasks';
    if (taskId && state.spaceId) {
      next = `/tasks/space/${state.spaceId}/task/${taskId}`;
    } else if (taskId) {
      next = `/tasks/task/${taskId}`;
    }
    if (window.location.pathname !== next) {
      window.history.replaceState({}, '', next);
    }
  }

  async function openTaskMoveModal(taskId) {
    taskMoveState = { taskId, mode: 'move' };
    const title = document.querySelector('#task-move-modal h3');
    if (title) title.textContent = t('tasks.actions.moveTask');
    await loadAllBoards();
    populateMoveSpaces();
    openModal('task-move-modal');
  }

  async function openTaskRestoreModal(taskId) {
    taskMoveState = { taskId, mode: 'restore' };
    const title = document.querySelector('#task-move-modal h3');
    if (title) title.textContent = t('tasks.actions.restoreTask');
    await loadAllBoards();
    populateMoveSpaces();
    openModal('task-move-modal');
  }

  async function loadAllBoards() {
    try {
      const res = await Api.get('/api/tasks/boards');
      state.boards = res.items || [];
    } catch (_) {
      state.boards = [];
    }
  }

  function populateMoveSpaces() {
    const spaceSel = document.getElementById('task-move-space');
    if (!spaceSel) return;
    spaceSel.innerHTML = '';
    state.spaces.forEach(space => {
      const opt = document.createElement('option');
      opt.value = space.id;
      opt.textContent = space.name;
      if (space.id === state.spaceId) opt.selected = true;
      spaceSel.appendChild(opt);
    });
    populateMoveBoards();
  }

  function populateMoveBoards() {
    const spaceId = parseInt(document.getElementById('task-move-space')?.value || '0', 10);
    const boardSel = document.getElementById('task-move-board');
    if (!boardSel) return;
    boardSel.innerHTML = '';
    const boards = state.boards.filter(b => b.space_id === spaceId);
    boards.forEach(board => {
      const opt = document.createElement('option');
      opt.value = board.id;
      opt.textContent = board.name;
      boardSel.appendChild(opt);
    });
    populateMoveColumns();
  }

  async function populateMoveColumns() {
    const boardId = parseInt(document.getElementById('task-move-board')?.value || '0', 10);
    if (!boardId) return;
    await loadColumns(boardId, true);
    const columnSel = document.getElementById('task-move-column');
    if (!columnSel) return;
    columnSel.innerHTML = '';
    (state.columnsByBoard[boardId] || []).forEach(col => {
      const opt = document.createElement('option');
      opt.value = col.id;
      opt.textContent = col.name;
      columnSel.appendChild(opt);
    });
  }

  async function saveTaskMove() {
    const taskId = taskMoveState.taskId;
    if (!taskId) return;
    const boardId = parseInt(document.getElementById('task-move-board')?.value || '0', 10);
    const columnId = parseInt(document.getElementById('task-move-column')?.value || '0', 10);
    if (!boardId || !columnId) {
      showAlert('task-move-alert', t('tasks.columnRequired'));
      return;
    }
    try {
      if (taskMoveState.mode === 'restore') {
        const restored = await Api.post(`/api/tasks/archive/${taskId}/restore`, { board_id: boardId, column_id: columnId });
        state.taskMap.set(restored.id, restored);
        if (state.spaceId) {
          await loadBoards(state.spaceId);
        }
        await loadArchivedTasks(true);
      } else {
        const updated = await Api.post(`/api/tasks/${taskId}/relocate`, { board_id: boardId, column_id: columnId, position: 0 });
        state.taskMap.set(updated.id, updated);
        await loadBoards(state.spaceId);
      }
      closeModal('task-move-modal');
    } catch (err) {
      showAlert('task-move-alert', resolveErrorMessage(err, 'common.error'));
    }
  }

  async function cloneTask(taskId) {
    try {
      const cloned = await Api.post(`/api/tasks/${taskId}/clone`, {});
      if (cloned && cloned.board_id) {
        await loadBoards(state.spaceId);
      }
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function archiveTask(taskId) {
    try {
      await Api.post(`/api/tasks/${taskId}/archive`, {});
      await loadBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function loadArchivedTasks(force = false) {
    if (!force && document.getElementById('tasks-tab-archive')?.hidden) return;
    try {
      const params = new URLSearchParams();
      if (state.spaceId) params.set('space_id', state.spaceId);
      const res = await Api.get(`/api/tasks/archive?${params.toString()}`);
      state.archivedTasks = res.items || [];
      renderArchivedList();
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  function renderArchivedList() {
    const list = document.getElementById('tasks-archive-list');
    const empty = document.getElementById('tasks-archive-empty');
    if (!list || !empty) return;
    list.innerHTML = '';
    const items = state.archivedTasks || [];
    if (!items.length) {
      empty.hidden = false;
      return;
    }
    empty.hidden = true;
    items.forEach(item => {
      const row = document.createElement('div');
      row.className = 'task-link-item';
      const boardName = findBoard(item.archived_board_id || item.board_id)?.name || `#${item.archived_board_id || item.board_id}`;
      const column = findColumn(item.archived_column_id || item.column_id);
      const columnName = column?.name || `#${item.archived_column_id || item.column_id}`;
      row.innerHTML = `
        <div class="task-link-main">
          <div class="task-link-title">${escapeHtml(item.title || '')}</div>
          <div class="muted">${escapeHtml(boardName)} - ${escapeHtml(columnName)} - ${formatDateShort(item.archived_at)}</div>
        </div>
      `;
      const actions = document.createElement('div');
      actions.className = 'btn-group';
      const openBtn = document.createElement('button');
      openBtn.type = 'button';
      openBtn.className = 'btn ghost btn-sm';
      openBtn.textContent = t('common.open');
      openBtn.addEventListener('click', () => openTaskById(item.id));
      actions.appendChild(openBtn);
      if (hasPermission('tasks.archive')) {
        const restoreBtn = document.createElement('button');
        restoreBtn.type = 'button';
        restoreBtn.className = 'btn ghost btn-sm';
        restoreBtn.textContent = t('tasks.actions.restoreTask');
        restoreBtn.addEventListener('click', async () => {
          try {
            await Api.post(`/api/tasks/archive/${item.id}/restore`, {});
            await loadArchivedTasks(true);
            if (state.spaceId) {
              await loadBoards(state.spaceId);
            }
          } catch (err) {
            if ((err?.message || '').trim() === 'tasks.restoreTargetRequired') {
              openTaskRestoreModal(item.id);
              return;
            }
            showError(err, 'common.error');
          }
        });
        actions.appendChild(restoreBtn);
      }
      row.appendChild(actions);
      list.appendChild(row);
    });
  }

  async function closeTask(taskId) {
    const ok = await confirmAction({ message: t('tasks.actions.closeTask') });
    if (!ok) return;
    try {
      await Api.post(`/api/tasks/${taskId}/close`, {});
      await loadBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  async function deleteTask(taskId) {
    const ok = await confirmAction({ message: t('tasks.deleteConfirm') });
    if (!ok) return;
    try {
      await Api.del(`/api/tasks/${taskId}`);
      await loadBoards(state.spaceId);
    } catch (err) {
      showError(err, 'common.error');
    }
  }

  TasksPage.init = init;
  TasksPage.loadBoards = loadBoards;
  TasksPage.loadColumns = loadColumns;
  TasksPage.loadTasks = loadTasks;
  TasksPage.renderBoards = renderBoards;
  TasksPage.renderBoard = () => renderBoards(state.spaceId);
  TasksPage.populateAccessLists = populateAccessLists;
  TasksPage.openTaskMoveModal = openTaskMoveModal;
  TasksPage.updateSpaceHash = updateSpaceHash;
  TasksPage.updateTaskHash = updateTaskHash;
  TasksPage.loadArchivedTasks = loadArchivedTasks;
  TasksPage.openTaskRestoreModal = openTaskRestoreModal;
})();
