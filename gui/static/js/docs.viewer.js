(() => {
  const state = DocsPage.state;
  const UNSAFE_URL_PATTERN = /[\u0000-\u001F\u007F\s]+/g;
  const isEditableFormat = (format) => {
    const fmt = String(format || '').toLowerCase();
    if (fmt === 'md' || fmt === 'txt') return true;
    if (fmt !== 'docx') return false;
    return !!(window.DocsOnlyOffice && typeof window.DocsOnlyOffice.open === 'function');
  };

  function sanitizeUrl(raw, opts = {}) {
    const val = String(raw || '').trim().replace(UNSAFE_URL_PATTERN, '');
    if (!val) return null;
    if (val.startsWith('#') || val.startsWith('/') || val.startsWith('./') || val.startsWith('../') || val.startsWith('?')) {
      return val;
    }
    let parsed;
    try {
      parsed = new URL(val, window.location.origin);
    } catch (_) {
      return null;
    }
    const protocol = (parsed.protocol || '').toLowerCase();
    const allow = opts.forImage ? ['http:', 'https:'] : ['http:', 'https:', 'mailto:'];
    if (!allow.includes(protocol)) return null;
    return parsed.href;
  }

  function sanitizeHtmlFragment(html) {
    const tpl = document.createElement('template');
    tpl.innerHTML = String(html || '');
    const blocked = new Set(['SCRIPT', 'STYLE', 'IFRAME', 'OBJECT', 'EMBED', 'LINK', 'META', 'BASE', 'FORM', 'INPUT', 'BUTTON', 'TEXTAREA', 'SELECT', 'SVG', 'MATH']);
    const walker = document.createTreeWalker(tpl.content, NodeFilter.SHOW_ELEMENT);
    const toRemove = [];
    while (walker.nextNode()) {
      const el = walker.currentNode;
      if (blocked.has(el.tagName)) {
        toRemove.push(el);
        continue;
      }
      Array.from(el.attributes).forEach(attr => {
        const name = attr.name.toLowerCase();
        if (name.startsWith('on') || name === 'style' || name === 'srcdoc') {
          el.removeAttribute(attr.name);
        }
      });
      if (el.tagName === 'A') {
        const safeHref = sanitizeUrl(el.getAttribute('href'), { forImage: false });
        if (!safeHref) {
          el.removeAttribute('href');
        } else {
          el.setAttribute('href', safeHref);
          el.setAttribute('target', '_blank');
          el.setAttribute('rel', 'noopener noreferrer');
        }
      }
      if (el.tagName === 'IMG') {
        const safeSrc = sanitizeUrl(el.getAttribute('src'), { forImage: true });
        if (!safeSrc) {
          toRemove.push(el);
        } else {
          el.setAttribute('src', safeSrc);
          el.setAttribute('loading', 'lazy');
          el.removeAttribute('srcset');
        }
      }
    }
    toRemove.forEach(el => el.remove());
    return tpl.innerHTML;
  }

  async function exportDoc(docId, format) {
    const fmt = String(format || 'pdf').trim().toLowerCase() || 'pdf';
    try {
      const res = await fetch(`/api/docs/${docId}/export?format=${encodeURIComponent(fmt)}`, {
        method: 'GET',
        credentials: 'include'
      });
      if (!res.ok) {
        const code = (await res.text()).trim();
        if (code === 'docs.export.approvalRequired') {
          alert(BerkutI18n.t('docs.exportApprovalRequired'));
          return;
        }
        throw new Error(code || `status_${res.status}`);
      }
      const blob = await res.blob();
      const cd = res.headers.get('Content-Disposition') || '';
      const match = /filename=\"?([^\";]+)\"?/i.exec(cd);
      const filename = (match && match[1]) ? match[1] : `doc-${docId}.${fmt}`;
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = filename;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
    } catch (err) {
      alert(BerkutI18n.t((err && err.message) || 'common.error'));
    }
  }

  async function approveExport(docId) {
    const requestedUsername = (prompt(BerkutI18n.t('docs.exportApproveRequesterPrompt')) || '').trim();
    if (!requestedUsername) return;
    const reason = (prompt(BerkutI18n.t('docs.exportApproveReasonPrompt')) || '').trim();
    try {
      await Api.post(`/api/docs/${docId}/export-approve`, {
        requested_username: requestedUsername,
        reason
      });
      alert(BerkutI18n.t('docs.exportApproveSaved'));
    } catch (err) {
      alert(BerkutI18n.t((err && err.message) || 'common.error'));
    }
  }

  async function openViewer(docId) {
    if (DocsPage.openDocTab) {
      DocsPage.openDocTab(docId, 'view');
      return;
    }
    window.location.href = `/docs/${encodeURIComponent(docId)}`;
  }

  async function renderDocInfo(meta, docId) {
    const createdEl = document.getElementById('doc-view-created');
    const updatedEl = document.getElementById('doc-view-updated');
    if (createdEl) createdEl.textContent = DocsPage.formatDate(meta.created_at);
    if (updatedEl) updatedEl.textContent = DocsPage.formatDate(meta.updated_at || meta.created_at);
    const versionsBtn = document.getElementById('doc-view-versions-btn');
    if (versionsBtn) {
      versionsBtn.onclick = () => DocsPage.openVersions(docId);
      versionsBtn.textContent = BerkutI18n.t('docs.view');
    }
    try {
      const verRes = await Api.get(`/api/docs/${docId}/versions`);
      if (versionsBtn) versionsBtn.textContent = `${BerkutI18n.t('docs.versionsTitle')} (${(verRes.versions || []).length})`;
    } catch (err) {
      console.warn('versions info', err);
    }
    const aclBox = document.getElementById('doc-view-acl');
    if (aclBox) {
      aclBox.textContent = '';
      try {
        const aclRes = await Api.get(`/api/docs/${docId}/acl`);
        const acl = aclRes.acl || [];
        const users = acl.filter(a => a.subject_type === 'user').map(a => a.subject_id);
        const roles = acl.filter(a => a.subject_type === 'role').map(a => a.subject_id);
        const parts = [];
        const rolesLabel = BerkutI18n.t('accounts.roles') || 'Roles';
        const usersLabel = BerkutI18n.t('accounts.users') || 'Users';
        if (roles.length) parts.push(`${rolesLabel}: ${roles.join(', ')}`);
        if (users.length) parts.push(`${usersLabel}: ${users.join(', ')}`);
        aclBox.textContent = parts.join(' | ') || BerkutI18n.t('docs.aclEmpty') || '-';
      } catch (err) {
        aclBox.textContent = '-';
        console.warn('acl info', err);
      }
    }
    const linksBox = document.getElementById('doc-view-links');
    if (linksBox) {
      linksBox.textContent = '';
      try {
        const linksRes = await Api.get(`/api/docs/${docId}/links`);
        if ((linksRes.links || []).length === 0) {
          linksBox.textContent = BerkutI18n.t('docs.linksEmpty') || '-';
        } else {
          linksRes.links.forEach(l => {
            const span = document.createElement('span');
            span.className = 'tag';
            span.textContent = `${l.target_type} #${l.target_id}`;
            linksBox.appendChild(span);
          });
        }
      } catch (err) {
        linksBox.textContent = '-';
        console.warn('links info', err);
      }
    }
    const controlsBox = document.getElementById('doc-view-controls');
    if (controlsBox) {
      controlsBox.textContent = '';
      try {
        const res = await Api.get(`/api/docs/${docId}/control-links`);
        const items = res.items || [];
        if (!items.length) {
          controlsBox.textContent = BerkutI18n.t('docs.controlsEmpty') || '-';
        } else {
          items.forEach(item => {
            const link = document.createElement('a');
            link.className = 'tag';
            link.href = `/controls?control=${item.control_id}`;
            link.textContent = `${item.code} - ${item.title}`;
            controlsBox.appendChild(link);
          });
        }
      } catch (err) {
        controlsBox.textContent = '-';
        console.warn('controls links info', err);
      }
    }
  }

  function renderViewerMarkdown(content) {
    const mdPane = document.getElementById('doc-viewer-md');
    if (!mdPane) return;
    mdPane.hidden = false;
    const rendered = renderMarkdown(content || '');
    mdPane.innerHTML = sanitizeHtmlFragment(rendered.html);
    mdPane.querySelectorAll('[data-code-idx]').forEach(btn => {
      btn.onclick = async () => {
        const idx = parseInt(btn.getAttribute('data-code-idx'), 10);
        const code = (rendered.codeBlocks[idx] && rendered.codeBlocks[idx].code) || '';
        try {
          await navigator.clipboard.writeText(code);
          btn.textContent = 'Copied';
          setTimeout(() => { btn.textContent = 'Copy'; }, 1200);
        } catch (_) {
          btn.textContent = 'Error';
          setTimeout(() => { btn.textContent = 'Copy'; }, 1200);
        }
      };
    });
  }

  async function renderViewerDocx(docxUrl) {
    const mdPane = document.getElementById('doc-viewer-md');
    const frameWrap = document.getElementById('doc-viewer-frame-wrap');
    const frame = document.getElementById('doc-viewer-frame');
    if (!mdPane) return false;
    if (!window.mammoth || !window.mammoth.convertToHtml) {
      return false;
    }
    try {
      const res = await fetch(docxUrl, { credentials: 'include' });
      if (!res.ok) {
        throw new Error(await res.text());
      }
      const arrayBuffer = await res.arrayBuffer();
      const result = await window.mammoth.convertToHtml({ arrayBuffer });
      if (frame) {
        frame.src = 'about:blank';
        frame.hidden = true;
      }
      if (frameWrap) frameWrap.hidden = true;
      mdPane.hidden = false;
      mdPane.innerHTML = `<div class="docx-view">${sanitizeHtmlFragment(result.value || '')}</div>`;
      return true;
    } catch (err) {
      return false;
    }
  }

  function renderMarkdown(md) {
    const esc = (str) => (str || '').replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
    const codeBlocks = [];
    const mdWithoutCodes = (md || '').replace(/```(\w+)?\n([\s\S]*?)```/g, (_, lang, code) => {
      const idx = codeBlocks.length;
      codeBlocks.push({ code: code || '', lang: lang || '' });
      return `@@CODE${idx}@@`;
    });

    const blocks = mdWithoutCodes.split(/\n{2,}/).map(b => {
      const lines = b.trim().split('\n');
      if (lines.length >= 2 && lines.every(l => l.trim().startsWith('|'))) {
        const [header, separator, ...rows] = lines;
        const headers = header.split('|').filter(Boolean).map(c => esc(c.trim()));
        const body = rows.map(r => r.split('|').filter(Boolean).map(c => esc(c.trim())));
        const headHtml = `<tr>${headers.map(h => `<th>${h}</th>`).join('')}</tr>`;
        const bodyHtml = body.map(r => `<tr>${r.map(c => `<td>${c}</td>`).join('')}</tr>`).join('');
        return `<table class="md-table"><thead>${headHtml}</thead><tbody>${bodyHtml}</tbody></table>`;
      }
      return esc(b);
    }).join('\n\n');

    let html = blocks;
    html = html.replace(/^###### (.*)$/gm, '<h6>$1</h6>')
      .replace(/^##### (.*)$/gm, '<h5>$1</h5>')
      .replace(/^#### (.*)$/gm, '<h4>$1</h4>')
      .replace(/^### (.*)$/gm, '<h3>$1</h3>')
      .replace(/^## (.*)$/gm, '<h2>$1</h2>')
      .replace(/^# (.*)$/gm, '<h1>$1</h1>')
      .replace(/!\[([^\]]*)\]\(([^)]+)\)/g, (_, alt, src) => {
        const safeSrc = sanitizeUrl(src, { forImage: true });
        return safeSrc ? `<img alt="${esc(alt)}" src="${esc(safeSrc)}">` : '';
      })
      .replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_, label, href) => {
        const safeHref = sanitizeUrl(href, { forImage: false });
        const safeLabel = esc(label);
        return safeHref ? `<a href="${esc(safeHref)}" target="_blank" rel="noopener noreferrer">${safeLabel}</a>` : safeLabel;
      })
      .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
      .replace(/__(.+?)__/g, '<strong>$1</strong>')
      .replace(/\*(.+?)\*/g, '<em>$1</em>')
      .replace(/_(.+?)_/g, '<em>$1</em>')
      .replace(/~~(.+?)~~/g, '<del>$1</del>')
      .replace(/`([^`\n]+)`/g, '<code>$1</code>')
      .replace(/^> (.*)$/gm, '<blockquote><p>$1</p></blockquote>')
      .replace(/^- (.*)$/gm, '<li>$1</li>')
      .replace(/(\r?\n){2,}/g, '</p><p>')
      .replace(/\r?\n/g, '<br>');

    html = html.replace(/@@CODE(\d+)@@/g, (_, idx) => {
      const i = parseInt(idx, 10);
      const block = codeBlocks[i] || { code: '', lang: '' };
      const code = esc(block.code);
      return `<div class="code-block"><div class="code-block-bar"><span class="code-lang">${esc(block.lang || 'code')}</span><button type="button" class="btn ghost icon-btn copy-code-btn" data-code-idx="${i}">Copy</button></div><pre><code>${code}</code></pre></div>`;
    });
    return { html: `<div class="md-view"><p>${html}</p></div>`, codeBlocks };
  }

  function bindViewerControls() {
    const zoomInput = document.getElementById('view-zoom');
    const zoomLabel = document.getElementById('view-zoom-label');
    if (zoomInput) {
      zoomInput.oninput = () => applyZoom(parseInt(zoomInput.value, 10) || 100);
      if (zoomLabel) zoomLabel.textContent = `${zoomInput.value}%`;
    }
    const searchBtn = document.getElementById('view-search-btn');
    const searchInput = document.getElementById('view-search');
    if (searchBtn && searchInput) {
      searchBtn.onclick = () => runSearch(searchInput.value.trim());
      searchInput.onkeypress = (e) => {
        if (e.key === 'Enter') {
          e.preventDefault();
          runSearch(searchInput.value.trim());
        }
      };
    }
    const editBtn = document.getElementById('view-edit-btn');
    if (editBtn) {
      editBtn.hidden = !isEditableFormat(state.viewerFormat);
      editBtn.onclick = () => {
        if (state.viewerDoc?.id) {
          DocsPage.openEditor(state.viewerDoc.id);
        }
      };
    }
  }

  function applyZoom(val) {
    const zoomInput = document.getElementById('view-zoom');
    const zoomLabel = document.getElementById('view-zoom-label');
    const mdPane = document.getElementById('doc-viewer-md');
    const frame = document.getElementById('doc-viewer-frame');
    const zoom = Math.min(200, Math.max(50, val || 100));
    if (zoomInput) zoomInput.value = zoom;
    if (zoomLabel) zoomLabel.textContent = `${zoom}%`;
    if (mdPane && !mdPane.hidden) mdPane.style.fontSize = `${zoom}%`;
    if (frame && !frame.hidden) {
      frame.style.transformOrigin = '0 0';
      frame.style.transform = `scale(${zoom / 100})`;
      frame.style.width = `${10000 / zoom}%`;
      frame.style.height = `${10000 / zoom}%`;
    }
  }

  function runSearch(term) {
    resetSearch();
    if (!term) return;
    if (state.viewerFormat !== 'md' && state.viewerFormat !== 'txt') return;
    const mdPane = document.getElementById('doc-viewer-md');
    if (!mdPane) return;
    const regex = new RegExp(`(${term.replace(/[.*+?^${}()|[\\]\\\\]/g, '\\\\$&')})`, 'gi');
    const rendered = renderMarkdown(state.viewerContent || '');
    mdPane.innerHTML = sanitizeHtmlFragment((rendered.html || '').replace(regex, '<mark>$1</mark>'));
  }

  function resetSearch() {
    if (state.viewerFormat === 'md' || state.viewerFormat === 'txt') {
      renderViewerMarkdown(state.viewerContent || '');
    }
  }

  DocsPage.exportDoc = exportDoc;
  DocsPage.openViewer = openViewer;
  DocsPage.renderDocInfo = renderDocInfo;
  DocsPage.renderViewerMarkdown = renderViewerMarkdown;
  DocsPage.renderMarkdown = renderMarkdown;
  DocsPage.sanitizeHtmlFragment = sanitizeHtmlFragment;
  DocsPage.sanitizeUrl = sanitizeUrl;
  DocsPage.bindViewerControls = bindViewerControls;
  DocsPage.applyZoom = applyZoom;
  DocsPage.runSearch = runSearch;
  DocsPage.resetSearch = resetSearch;
  DocsPage.approveExport = approveExport;
})();
