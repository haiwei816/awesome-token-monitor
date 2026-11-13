// Universal AI Agent Token & Model Monitor Client (v3.0 Telemetry)
function getLocalDateString() {
  const d = new Date();
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

let currentAgent = 'all';
let currentModel = 'all';
let currentDate = 'all'; // 默认查看全部历史总量
let currentLedgerView = 'day'; // 'day' or 'month'
let cachedDailyList = [];
let cachedMonthlyList = [];
let currentGranularity = 'day';
let currentSort = 'total_tokens';
let currentOrder = 'desc';
let currentModelSortMetric = 'total_tokens';
let lastLoadedModels = [];
let allCachedModels = [];
let searchQuery = '';
let currentPage = 0;
const pageSize = 20;

let trendChartInstance = null;
let trendRawBuckets = [];
let stepGrowthChartInstance = null;
let autoRefreshTimer = null;
let latestSummaryData = null;
let allTimeTotalTokens = 0;
let activeDetailRequestId = 0;
let currentDrawerCid = '';
let isLedgerCollapsed = false;

// Security & HTML Escaping Helper
function escapeHtml(str) {
  if (str === null || str === undefined) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

// Unified HTTP Fetch with Cache Busting
async function fetchAPI(url) {
  const sep = url.includes('?') ? '&' : '?';
  const bustUrl = `${url}${sep}_t=${Date.now()}`;
  const res = await fetch(bustUrl, {
    cache: 'no-store',
    headers: {
      'Cache-Control': 'no-cache, no-store, must-revalidate',
      'Pragma': 'no-cache',
      'Expires': '0'
    }
  });
  if (!res.ok) {
    throw new Error(`HTTP error ${res.status}`);
  }
  return await res.json();
}

// Quick Date Filter Switching Helpers
window.switchToToday = function() {
  const todayStr = getLocalDateString();
  if (currentDate === todayStr || currentDate === 'today') {
    window.switchToAllDates();
    return;
  }
  currentDate = todayStr;
  const sel = document.getElementById('dateFilterSelect');
  if (sel) sel.value = currentDate;
  currentPage = 0;
  showToast(`已切换至【今日 (${todayStr})】数据视图`, 'info');
  loadAllData();
};

window.switchToThisMonth = function() {
  const curMonth = getLocalDateString().slice(0, 7);
  if (currentDate === curMonth || currentDate === 'this_month') {
    window.switchToAllDates();
    return;
  }
  currentDate = curMonth;
  const sel = document.getElementById('dateFilterSelect');
  if (sel) sel.value = currentDate;
  currentPage = 0;
  showToast(`已切换至【本月 (${curMonth})】数据视图`, 'info');
  loadAllData();
};

window.switchToAllDates = function() {
  currentDate = 'all';
  const sel = document.getElementById('dateFilterSelect');
  if (sel) sel.value = 'all';
  currentPage = 0;
  showToast('已切回【全部历史】数据视图', 'info');
  loadAllData();
};

window.resetAllFilters = function() {
  currentAgent = 'all';
  currentModel = 'all';
  currentDate = 'all';
  searchQuery = '';
  currentPage = 0;

  document.querySelectorAll('.agent-tab').forEach(t => {
    t.classList.toggle('active', t.dataset.agent === 'all');
  });
  const dateSel = document.getElementById('dateFilterSelect');
  if (dateSel) dateSel.value = 'all';
  const modelSel = document.getElementById('modelFilterSelect');
  if (modelSel) modelSel.value = 'all';
  const searchEl = document.getElementById('searchInput');
  if (searchEl) searchEl.value = '';
  updateSearchControlsUI();

  showToast('已重置全部筛选条件', 'info');
  loadAllData();
};

// Agent Info Configuration
const AGENT_CONFIG = {
  codex:        { name: 'Codex',         icon: '🧠', badgeClass: 'badge-codex' },
  claude:       { name: 'Claude Code',   icon: '🟣', badgeClass: 'badge-claude' },
  pi:           { name: 'Pi Agent',      icon: '🥧', badgeClass: 'badge-pi' },
  qoder:        { name: 'Qoder',         icon: '⚡', badgeClass: 'badge-qoder' },
  antigravity:  { name: 'Antigravity',   icon: '🛰️', badgeClass: 'badge-antigravity' },
  workbuddy:    { name: 'WorkBuddy',     icon: '💼', badgeClass: 'badge-workbuddy' },
  zcode:        { name: 'ZCode',         icon: '🔷', badgeClass: 'badge-zcode' }
};

// Formatters
function formatTokens(num) {
  if (num === null || num === undefined || isNaN(num)) return '--';
  if (num >= 1_000_000) {
    return (num / 1_000_000).toFixed(2) + 'M';
  } else if (num >= 1_000) {
    return (num / 1_000).toFixed(1) + 'K';
  }
  return Number(num).toLocaleString();
}

function formatExact(num) {
  if (num === null || num === undefined || isNaN(num)) return '0';
  return Number(num).toLocaleString();
}

function formatDate(isoStr) {
  if (!isoStr) return '--';
  try {
    const d = new Date(isoStr);
    if (isNaN(d.getTime())) return isoStr;
    const pad = (n) => String(n).padStart(2, '0');
    return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
  } catch {
    return isoStr;
  }
}

// Toast Notification Helper
function showToast(message, type = 'info') {
  const container = document.getElementById('toastContainer');
  if (!container) return;
  const toast = document.createElement('div');
  toast.className = `toast toast-${type}`;
  let icon = 'ℹ️';
  if (type === 'success') icon = '✅';
  else if (type === 'error') icon = '⚠️';
  toast.innerHTML = `<span>${icon}</span><span>${escapeHtml(message)}</span>`;
  container.appendChild(toast);
  requestAnimationFrame(() => {
    toast.classList.add('show');
  });
  setTimeout(() => {
    toast.classList.remove('show');
    setTimeout(() => {
      if (toast.parentNode) toast.parentNode.removeChild(toast);
    }, 300);
  }, 3000);
}

// App Initialization
document.addEventListener('DOMContentLoaded', () => {
  initEventListeners();
  updateSortUI();
  updateSearchControlsUI();
  loadAllData();
  setupAutoRefresh();
});

function updateSearchControlsUI() {
  const searchEl = document.getElementById('searchInput');
  const clearBtn = document.getElementById('btnClearSearch');
  const kbdHint = document.getElementById('searchKbdHint');
  const hasVal = searchEl && searchEl.value.trim().length > 0;
  if (clearBtn) clearBtn.style.display = hasVal ? 'inline-flex' : 'none';
  if (kbdHint) kbdHint.style.display = hasVal ? 'none' : 'inline-block';
}

function updateSortUI() {
  const sel = document.getElementById('sortSelect');
  if (sel && sel.value !== currentSort) sel.value = currentSort;

  const dirBtn = document.getElementById('btnToggleSortOrder');
  if (dirBtn) {
    dirBtn.textContent = currentOrder === 'desc' ? '↓ 降序' : '↑ 升序';
    dirBtn.title = currentOrder === 'desc' ? '当前降序，点击切换为升序' : '当前升序，点击切换为降序';
  }

  document.querySelectorAll('.sortable-th').forEach(th => {
    const col = th.dataset.sort;
    const arrow = th.querySelector('.sort-arrow');
    const isMatch = (col === currentSort) || (col === 'input_tokens' && currentSort === 'output_tokens');
    th.classList.toggle('active-sort', isMatch);
    if (arrow) {
      arrow.textContent = isMatch ? (currentOrder === 'desc' ? '↓' : '↑') : '';
    }
  });
}

function renderActiveFilterBar() {
  const bar = document.getElementById('activeFilterBar');
  const chipsContainer = document.getElementById('activeFilterChips');
  if (!bar || !chipsContainer) return;

  const chips = [];
  if (currentAgent !== 'all') {
    const conf = AGENT_CONFIG[currentAgent] || { name: currentAgent, icon: '🤖' };
    chips.push({ key: 'agent', label: `平台: ${conf.icon} ${conf.name}` });
  }
  if (currentDate !== 'all') {
    const todayStr = getLocalDateString();
    const curMonthStr = todayStr.slice(0, 7);
    let dateDesc = currentDate;
    if (currentDate === todayStr || currentDate === 'today') dateDesc = `今日 (${todayStr})`;
    else if (currentDate === curMonthStr || currentDate === 'this_month') dateDesc = `本月 (${curMonthStr})`;
    else if (currentDate.length === 7) dateDesc = `月份 (${currentDate})`;
    chips.push({ key: 'date', label: `时段: 📅 ${dateDesc}` });
  }
  if (currentModel !== 'all') {
    chips.push({ key: 'model', label: `模型: 🤖 ${currentModel}` });
  }
  if (searchQuery) {
    chips.push({ key: 'search', label: `关键词: "${searchQuery}"` });
  }

  if (chips.length === 0) {
    bar.style.display = 'none';
    chipsContainer.innerHTML = '';
    return;
  }

  bar.style.display = 'flex';
  chipsContainer.innerHTML = chips.map(c => `
    <button type="button" class="filter-chip" data-clear-key="${escapeHtml(c.key)}" title="点击清除此筛选条件">
      <span>${escapeHtml(c.label)}</span>
      <span class="filter-chip-close">✕</span>
    </button>
  `).join('');
}

function closeExportMenu() {
  const menu = document.getElementById('exportMenu');
  const btn = document.getElementById('btnExport');
  if (menu) menu.classList.remove('open');
  if (btn) btn.setAttribute('aria-expanded', 'false');
}

function triggerSync() {
  const btn = document.getElementById('btnSync');
  if (btn && !btn.disabled) {
    btn.click();
  }
}

function initEventListeners() {
  // Sync Button
  document.getElementById('btnSync').addEventListener('click', async () => {
    const btn = document.getElementById('btnSync');
    btn.disabled = true;
    btn.innerHTML = `<svg class="spin" width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg> 同步中...`;
    const t0 = performance.now();
    try {
      await fetch('/api/scan', { method: 'POST' });
      await loadAllData();
      const elapsed = Math.round(performance.now() - t0);
      showToast(`遥测数据同步完成 (耗时 ${elapsed}ms)`, 'success');
    } catch (e) {
      console.error(e);
      showToast(`同步失败: ${e.message}`, 'error');
    } finally {
      btn.disabled = false;
      btn.innerHTML = `<svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M21 2v6h-6M3 12a9 9 0 0 1 15-6.7L21 8M3 22v-6h6M21 12a9 9 0 0 1-15 6.7L3 16"/></svg> 立即同步`;
    }
  });

  // Split Export Dropdown
  const btnExport = document.getElementById('btnExport');
  const exportMenu = document.getElementById('exportMenu');
  if (btnExport && exportMenu) {
    btnExport.addEventListener('click', (e) => {
      e.stopPropagation();
      const isOpen = exportMenu.classList.toggle('open');
      btnExport.setAttribute('aria-expanded', String(isOpen));
    });
    document.getElementById('btnExportCSV').addEventListener('click', () => {
      closeExportMenu();
      const query = `agent=${encodeURIComponent(currentAgent)}&model=${encodeURIComponent(currentModel)}&date=${encodeURIComponent(currentDate)}`;
      showToast('正在导出 CSV 报表...', 'info');
      window.location.href = `/api/export?format=csv&${query}`;
    });
    document.getElementById('btnExportJSON').addEventListener('click', () => {
      closeExportMenu();
      const query = `agent=${encodeURIComponent(currentAgent)}&model=${encodeURIComponent(currentModel)}&date=${encodeURIComponent(currentDate)}`;
      showToast('正在导出 JSON 完整快照...', 'info');
      window.location.href = `/api/export?format=json&${query}`;
    });
    document.addEventListener('click', (e) => {
      const wrapper = document.getElementById('exportDropdownWrapper');
      if (wrapper && !wrapper.contains(e.target)) {
        closeExportMenu();
      }
    });
  }

  // Agent Selector Tabs (Click + Keyboard)
  document.querySelectorAll('.agent-tab').forEach(tab => {
    const activateAgent = () => {
      document.querySelectorAll('.agent-tab').forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
      currentAgent = tab.dataset.agent;
      currentPage = 0;
      loadAllData();
    };
    tab.addEventListener('click', activateAgent);
    tab.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        activateAgent();
      }
    });
  });

  // Active Filter Chips Event Delegation + Reset All
  const activeFilterChips = document.getElementById('activeFilterChips');
  if (activeFilterChips) {
    activeFilterChips.addEventListener('click', (e) => {
      const chip = e.target.closest('.filter-chip');
      if (!chip) return;
      const key = chip.dataset.clearKey;
      if (key === 'agent') {
        currentAgent = 'all';
        document.querySelectorAll('.agent-tab').forEach(t => t.classList.toggle('active', t.dataset.agent === 'all'));
      } else if (key === 'date') {
        currentDate = 'all';
        const sel = document.getElementById('dateFilterSelect');
        if (sel) sel.value = 'all';
      } else if (key === 'model') {
        currentModel = 'all';
        const sel = document.getElementById('modelFilterSelect');
        if (sel) sel.value = 'all';
      } else if (key === 'search') {
        searchQuery = '';
        const searchEl = document.getElementById('searchInput');
        if (searchEl) searchEl.value = '';
        updateSearchControlsUI();
      }
      currentPage = 0;
      loadAllData();
    });
  }
  const btnResetAll = document.getElementById('btnResetAllFilters');
  if (btnResetAll) {
    btnResetAll.addEventListener('click', window.resetAllFilters);
  }

  // Date Filter Select
  document.getElementById('dateFilterSelect').addEventListener('change', (e) => {
    const val = e.target.value;
    if (val === 'today') {
      currentDate = getLocalDateString();
    } else if (val === 'this_month') {
      currentDate = getLocalDateString().slice(0, 7);
    } else {
      currentDate = val;
    }
    currentPage = 0;
    loadAllData();
  });

  // Model Filter Select
  document.getElementById('modelFilterSelect').addEventListener('change', (e) => {
    currentModel = e.target.value;
    currentPage = 0;
    loadAllData();
  });

  // Auto refresh select
  document.getElementById('autoRefreshSelect').addEventListener('change', setupAutoRefresh);

  // Ledger View Tabs (Daily vs Monthly)
  document.querySelectorAll('#ledgerViewTabs .tab-btn').forEach(btn => {
    btn.addEventListener('click', (e) => {
      document.querySelectorAll('#ledgerViewTabs .tab-btn').forEach(b => b.classList.remove('active'));
      e.currentTarget.classList.add('active');
      currentLedgerView = e.currentTarget.dataset.view || 'day';
      const titleEl = document.getElementById('ledgerTitleText');
      if (titleEl) {
        titleEl.textContent = currentLedgerView === 'month'
          ? '按月统计归档 (Monthly Ledger Timeline)'
          : '按天归档时间流 (Daily Ledger Timeline)';
      }
      renderLedgerCards();
    });
  });

  // Ledger Collapse/Expand Toggle
  const btnToggleLedger = document.getElementById('btnToggleLedger');
  const ledgerWrapper = document.getElementById('ledgerBodyWrapper');
  if (btnToggleLedger && ledgerWrapper) {
    btnToggleLedger.addEventListener('click', () => {
      isLedgerCollapsed = !isLedgerCollapsed;
      ledgerWrapper.style.display = isLedgerCollapsed ? 'none' : 'block';
      btnToggleLedger.textContent = isLedgerCollapsed ? '▸ 展开' : '▾ 收起';
    });
  }

  // Trend Granularity Tabs
  document.querySelectorAll('#trendGranularityTabs .tab-btn').forEach(btn => {
    btn.addEventListener('click', (e) => {
      document.querySelectorAll('#trendGranularityTabs .tab-btn').forEach(b => b.classList.remove('active'));
      e.currentTarget.classList.add('active');
      currentGranularity = e.currentTarget.dataset.granularity;
      loadTrends();
    });
  });

  // Model Ranking Sort Tabs
  document.querySelectorAll('#modelSortTabs .tab-btn').forEach(btn => {
    btn.addEventListener('click', (e) => {
      document.querySelectorAll('#modelSortTabs .tab-btn').forEach(b => b.classList.remove('active'));
      e.currentTarget.classList.add('active');
      currentModelSortMetric = e.currentTarget.dataset.metric || 'total_tokens';
      renderModelsList(lastLoadedModels);
    });
  });

  // Search input with debounce + clear button
  let searchTimer;
  const searchInput = document.getElementById('searchInput');
  searchInput.addEventListener('input', (e) => {
    updateSearchControlsUI();
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      searchQuery = e.target.value.trim();
      currentPage = 0;
      renderActiveFilterBar();
      loadConversations();
    }, 260);
  });

  const btnClearSearch = document.getElementById('btnClearSearch');
  if (btnClearSearch) {
    btnClearSearch.addEventListener('click', () => {
      searchInput.value = '';
      searchQuery = '';
      currentPage = 0;
      updateSearchControlsUI();
      renderActiveFilterBar();
      loadConversations();
      searchInput.focus();
    });
  }

  // Sort Select & Direction Toggle & Sortable Headers
  document.getElementById('sortSelect').addEventListener('change', (e) => {
    currentSort = e.target.value;
    currentPage = 0;
    updateSortUI();
    loadConversations();
  });

  const btnSortDir = document.getElementById('btnToggleSortOrder');
  if (btnSortDir) {
    btnSortDir.addEventListener('click', () => {
      currentOrder = currentOrder === 'desc' ? 'asc' : 'desc';
      currentPage = 0;
      updateSortUI();
      loadConversations();
    });
  }

  document.querySelectorAll('.sortable-th').forEach(th => {
    th.addEventListener('click', () => {
      const col = th.dataset.sort;
      if (!col) return;
      if (currentSort === col) {
        currentOrder = currentOrder === 'desc' ? 'asc' : 'desc';
      } else {
        currentSort = col;
        currentOrder = 'desc';
      }
      currentPage = 0;
      updateSortUI();
      loadConversations();
    });
  });

  // Pagination
  document.getElementById('btnPrevPage').addEventListener('click', () => {
    if (currentPage > 0) {
      currentPage--;
      loadConversations();
    }
  });

  document.getElementById('btnNextPage').addEventListener('click', () => {
    currentPage++;
    loadConversations();
  });

  // Drawer Close & Copy CID
  document.getElementById('btnDrawerClose').addEventListener('click', closeDrawer);
  document.getElementById('drawerBackdrop').addEventListener('click', closeDrawer);
  const btnCopyDrawerCid = document.getElementById('btnCopyDrawerCid');
  if (btnCopyDrawerCid) {
    btnCopyDrawerCid.addEventListener('click', () => {
      if (currentDrawerCid) copyId(currentDrawerCid);
    });
  }

  // Clear Model Filter Button
  const btnClearModel = document.getElementById('btnClearModelFilter');
  if (btnClearModel) {
    btnClearModel.addEventListener('click', () => {
      currentModel = 'all';
      const sel = document.getElementById('modelFilterSelect');
      if (sel) sel.value = 'all';
      currentPage = 0;
      loadAllData();
    });
  }

  // KPI Card 1 (Full card + Badges)
  const cardTodayOrTotal = document.getElementById('cardTodayOrTotal');
  if (cardTodayOrTotal) {
    cardTodayOrTotal.addEventListener('click', () => {
      window.switchToToday();
    });
  }

  const badgeToday = document.getElementById('kpiTodayTotalBadge');
  if (badgeToday) {
    badgeToday.addEventListener('click', (e) => {
      e.stopPropagation();
      window.switchToToday();
    });
  }

  const badgeMonth = document.getElementById('kpiMonthTotalBadge');
  if (badgeMonth) {
    badgeMonth.addEventListener('click', (e) => {
      e.stopPropagation();
      window.switchToThisMonth();
    });
  }

  // Card 2 (This Month card + delegated Today sub-link)
  const cardThisMonth = document.getElementById('cardThisMonth');
  if (cardThisMonth) {
    cardThisMonth.addEventListener('click', (e) => {
      if (e.target.closest('#subTodayLink')) {
        e.stopPropagation();
        window.switchToToday();
        return;
      }
      window.switchToThisMonth();
    });
  }

  // Card 5 (Cache card -> Toggle Today / All-Time)
  const cardCache = document.getElementById('cardCache');
  if (cardCache) {
    cardCache.addEventListener('click', () => {
      window.switchToToday();
    });
  }

  // Quick All Months Button
  const btnQuickAll = document.getElementById('btnQuickAllMonths');
  if (btnQuickAll) {
    btnQuickAll.addEventListener('click', () => {
      window.switchToAllDates();
    });
  }

  // Global Keyboard Shortcuts ('/' search, 'Esc' close drawer/menu, 'r' sync)
  document.addEventListener('keydown', (e) => {
    const tag = (e.target && e.target.tagName) ? e.target.tagName.toLowerCase() : '';
    const isTyping = (tag === 'input' || tag === 'textarea' || tag === 'select' || e.target?.isContentEditable);

    if (e.key === 'Escape') {
      closeExportMenu();
      const drawer = document.getElementById('conversationDrawer');
      if (drawer && drawer.classList.contains('open')) {
        closeDrawer();
      }
      return;
    }

    if (isTyping || e.ctrlKey || e.metaKey || e.altKey) return;

    if (e.key === '/') {
      e.preventDefault();
      searchInput.focus();
      searchInput.select();
    } else if (e.key === 'r' || e.key === 'R') {
      e.preventDefault();
      triggerSync();
    }
  });
}

function setupAutoRefresh() {
  if (autoRefreshTimer) clearInterval(autoRefreshTimer);
  const seconds = parseInt(document.getElementById('autoRefreshSelect').value, 10);
  if (seconds > 0) {
    autoRefreshTimer = setInterval(() => {
      loadAllData(true);
    }, seconds * 1000);
    document.getElementById('liveStatusText').textContent = `实时同步中 (${seconds}s)`;
  } else {
    document.getElementById('liveStatusText').textContent = '自动刷新已暂停';
  }
}

async function loadAllData(silent = false) {
  try {
    if (currentDate === 'today') {
      currentDate = getLocalDateString();
    } else if (currentDate === 'this_month') {
      currentDate = getLocalDateString().slice(0, 7);
    }
    renderActiveFilterBar();
    await Promise.all([
      loadSummary(),
      loadLedgerRecords(),
      loadTrends(),
      loadConversations(),
      loadAgentNavPills(),
      loadModelsList()
    ]);
  } catch (err) {
    console.error("Failed to load dashboard data:", err);
    if (!silent) {
      showToast('数据加载失败，请检查后端服务状态', 'error');
    }
  }
}

// 1. Load Summary
async function loadSummary() {
  const url = `/api/summary?agent=${encodeURIComponent(currentAgent)}&model=${encodeURIComponent(currentModel)}&date=${encodeURIComponent(currentDate)}`;
  const data = await fetchAPI(url);
  latestSummaryData = data;

  if (currentDate === 'all' && data.total_tokens > 0) {
    allTimeTotalTokens = data.total_tokens;
  }

  const todayStr = getLocalDateString();
  const curMonthStr = todayStr.slice(0, 7);
  const isToday = (currentDate === todayStr || currentDate === 'today');
  const isMonth = (currentDate && currentDate.length === 7);

  // KPI 1: Total Tokens
  const valTotalTokensEl = document.getElementById('valTotalTokens');
  if (valTotalTokensEl) valTotalTokensEl.textContent = formatTokens(data.total_tokens);
  const valTotalTokensRawEl = document.getElementById('valTotalTokensRaw');
  if (valTotalTokensRawEl) valTotalTokensRawEl.textContent = formatExact(data.total_tokens);

  const totalTitleEl = document.getElementById('kpiTotalTitle');
  const todayBadge = document.getElementById('kpiTodayTotalBadge');
  const monthBadge = document.getElementById('kpiMonthTotalBadge');
  const totalSubEl = document.getElementById('kpiTotalSub');

  if (todayBadge) {
    if (isToday) {
      todayBadge.textContent = '📅 今日已选 (点击看全部)';
      todayBadge.classList.add('active');
    } else {
      todayBadge.textContent = `+${formatTokens(data.today_total)} 今日`;
      todayBadge.classList.remove('active');
    }
  }

  if (monthBadge) {
    if (isMonth) {
      monthBadge.textContent = `🗓️ ${currentDate}已选 (点击看全部)`;
      monthBadge.classList.add('active');
    } else {
      monthBadge.textContent = `+${formatTokens(data.month_total)} 本月`;
      monthBadge.classList.remove('active');
    }
  }

  if (totalTitleEl) {
    if (currentDate === 'all') {
      totalTitleEl.textContent = '历史消耗总量 (All-Time Total)';
    } else if (isToday) {
      totalTitleEl.textContent = `【今日】消耗总量 (${todayStr})`;
    } else if (isMonth) {
      totalTitleEl.textContent = `【${currentDate}】月度消耗总量`;
    } else {
      totalTitleEl.textContent = `【${currentDate}】当日消耗总量`;
    }
  }

  if (totalSubEl) {
    if (currentDate === 'all') {
      let subHtml = `<span>精确计:</span> <span class="highlight">${formatExact(data.total_tokens)}</span> <span>Tokens</span>`;
      if (data.monthly_breakdown && data.monthly_breakdown.length > 0) {
        const topMonths = data.monthly_breakdown.slice(0, 3).map(m => `${escapeHtml(m.month_bucket.slice(5))}月: ${formatTokens(m.total_tokens)}`).join(' | ');
        subHtml += `<span style="margin: 0 4px; color: var(--border-color);">|</span><span style="color: #94a3b8; font-size: 0.72rem;">近期: ${topMonths}</span>`;
      }
      totalSubEl.innerHTML = subHtml;
    } else if (isToday) {
      const pctOfMonth = data.month_total > 0 ? ((data.total_tokens / data.month_total) * 100).toFixed(1) : 0;
      totalSubEl.innerHTML = `<span>精确计:</span> <span class="highlight">${formatExact(data.total_tokens)}</span> <span>Tokens</span> <span style="margin: 0 4px; color: var(--border-color);">|</span> <span>占本月:</span> <span class="highlight">${pctOfMonth}%</span>`;
    } else if (isMonth) {
      const pctOfAll = allTimeTotalTokens > 0 ? ((data.total_tokens / allTimeTotalTokens) * 100).toFixed(1) : 100;
      totalSubEl.innerHTML = `<span>精确计:</span> <span class="highlight">${formatExact(data.total_tokens)}</span> <span>Tokens</span> <span style="margin: 0 4px; color: var(--border-color);">|</span> <span>占历史:</span> <span class="highlight">${pctOfAll}%</span>`;
    } else {
      totalSubEl.innerHTML = `<span>精确计:</span> <span class="highlight">${formatExact(data.total_tokens)}</span> <span>Tokens</span>`;
    }
  }

  // KPI 2: This Month Tokens (保留 clickable #subTodayLink)
  const monthTitleEl = document.getElementById('kpiMonthTitle');
  if (monthTitleEl) {
    monthTitleEl.textContent = `本月消耗总量 (${curMonthStr})`;
  }
  const valMonthTotalEl = document.getElementById('valMonthTotalTokens');
  if (valMonthTotalEl) {
    valMonthTotalEl.textContent = formatTokens(data.month_total || 0);
  }
  const monthRatioEl = document.getElementById('kpiMonthRatioBadge');
  if (monthRatioEl) {
    const baseTotal = allTimeTotalTokens || (currentDate === 'all' ? data.total_tokens : 0);
    const ratio = baseTotal > 0 ? ((data.month_total / baseTotal) * 100).toFixed(1) : (data.month_total > 0 ? '100' : '0');
    monthRatioEl.textContent = `占历史 ${ratio}%`;
  }
  const kpiMonthSubEl = document.getElementById('kpiMonthSub');
  if (kpiMonthSubEl) {
    const todayPct = data.month_total > 0 ? ((data.today_total / data.month_total) * 100).toFixed(1) : 0;
    kpiMonthSubEl.innerHTML = `<span id="subTodayLink" class="sub-quick-link" title="点击直接切换至今日消耗视图">今日: <span class="highlight" style="color: #34d399;" id="valTodayTokensSub">${formatTokens(data.today_total || 0)}</span> <span style="font-size: 0.7rem; color: #94a3b8;">(${todayPct}%)</span></span> <span style="margin: 0 4px; color: var(--border-color);">|</span> <span>本月轮次:</span> <span class="highlight" id="valMonthStepsSub">${formatExact(data.month_steps || 0)} 轮</span>`;
  }

  // KPI 3: Input Tokens
  const inRatio = data.total_tokens > 0 ? ((data.total_input / data.total_tokens) * 100).toFixed(1) : 0;
  document.getElementById('valInputTokens').textContent = formatTokens(data.total_input);
  document.getElementById('valInputRatio').textContent = `${inRatio}% 占比`;
  const kpiInputTitle = document.getElementById('kpiInputTitle');
  const kpiInputSub = document.getElementById('kpiInputSub');

  if (isToday) {
    if (kpiInputTitle) kpiInputTitle.textContent = '【今日】模型输入 (Input)';
    if (kpiInputSub) kpiInputSub.innerHTML = `<span>当日精确:</span> <span class="highlight" id="valTodayInput">${formatExact(data.total_input)}</span> <span>Tokens | 占比 ${inRatio}%</span>`;
  } else if (isMonth) {
    if (kpiInputTitle) kpiInputTitle.textContent = `【${currentDate}】月度输入`;
    if (kpiInputSub) kpiInputSub.innerHTML = `<span>全月精确:</span> <span class="highlight">${formatExact(data.total_input)}</span> <span>Tokens</span>`;
  } else if (currentDate !== 'all') {
    if (kpiInputTitle) kpiInputTitle.textContent = `【${currentDate}】当日输入`;
    if (kpiInputSub) kpiInputSub.innerHTML = `<span>当日精确:</span> <span class="highlight">${formatExact(data.total_input)}</span> <span>Tokens</span>`;
  } else {
    if (kpiInputTitle) kpiInputTitle.textContent = '模型输入 (Prompt Input)';
    if (kpiInputSub) kpiInputSub.innerHTML = `<span>今日输入:</span> <span class="highlight" id="valTodayInput">${formatTokens(data.today_input)}</span> <span style="margin: 0 4px; color: var(--border-color);">|</span> <span>本月输入:</span> <span class="highlight">${formatTokens(data.month_input)}</span>`;
  }

  // KPI 4: Output Tokens
  const outRatio = data.total_tokens > 0 ? ((data.total_output / data.total_tokens) * 100).toFixed(1) : 0;
  document.getElementById('valOutputTokens').textContent = formatTokens(data.total_output);
  document.getElementById('valOutputRatio').textContent = `${outRatio}% 占比`;
  const kpiOutputTitle = document.getElementById('kpiOutputTitle');
  const kpiOutputSub = document.getElementById('kpiOutputSub');

  if (isToday) {
    if (kpiOutputTitle) kpiOutputTitle.textContent = '【今日】模型输出 (Output)';
    if (kpiOutputSub) kpiOutputSub.innerHTML = `<span>当日精确:</span> <span class="highlight" id="valTodayOutput">${formatExact(data.total_output)}</span> <span>Tokens (含思维链)</span>`;
  } else if (isMonth) {
    if (kpiOutputTitle) kpiOutputTitle.textContent = `【${currentDate}】月度输出`;
    if (kpiOutputSub) kpiOutputSub.innerHTML = `<span>全月精确:</span> <span class="highlight">${formatExact(data.total_output)}</span> <span>Tokens (含思维链)</span>`;
  } else if (currentDate !== 'all') {
    if (kpiOutputTitle) kpiOutputTitle.textContent = `【${currentDate}】当日输出`;
    if (kpiOutputSub) kpiOutputSub.innerHTML = `<span>当日精确:</span> <span class="highlight">${formatExact(data.total_output)}</span> <span>Tokens</span>`;
  } else {
    if (kpiOutputTitle) kpiOutputTitle.textContent = '模型输出 (Generation)';
    if (kpiOutputSub) kpiOutputSub.innerHTML = `<span>今日输出:</span> <span class="highlight" id="valTodayOutput">${formatTokens(data.today_output)}</span> <span style="margin: 0 4px; color: var(--border-color);">|</span> <span>本月输出:</span> <span class="highlight">${formatTokens(data.month_output)}</span> <span>(含思维链)</span>`;
  }

  // KPI 5: Cache Read
  document.getElementById('valCacheTokens').textContent = formatTokens(data.total_cache);
  document.getElementById('valCacheHitRate').textContent = `${data.cache_hit_ratio}% 命中率`;
  const kpiCacheTitle = document.getElementById('kpiCacheTitle');
  const kpiCacheSub = document.getElementById('kpiCacheSub');

  if (isToday) {
    if (kpiCacheTitle) kpiCacheTitle.textContent = '【今日】Prompt 缓存命中 (Cache)';
    if (kpiCacheSub) kpiCacheSub.innerHTML = `<span>当日精确:</span> <span class="highlight" style="color: #10b981;" id="valTodayCache">${formatExact(data.total_cache)}</span> <span>Tokens | 节省 ${data.cache_hit_ratio}% 重复计算</span>`;
  } else if (isMonth) {
    if (kpiCacheTitle) kpiCacheTitle.textContent = `【${currentDate}】月度缓存命中`;
    if (kpiCacheSub) kpiCacheSub.innerHTML = `<span>全月精确:</span> <span class="highlight" style="color: #10b981;">${formatExact(data.total_cache)}</span> <span>Tokens | 节省 ${data.cache_hit_ratio}% 重复计算</span>`;
  } else if (currentDate !== 'all') {
    if (kpiCacheTitle) kpiCacheTitle.textContent = `【${currentDate}】当日缓存命中`;
    if (kpiCacheSub) kpiCacheSub.innerHTML = `<span>当日精确:</span> <span class="highlight" style="color: #10b981;">${formatExact(data.total_cache)}</span> <span>Tokens | 节省 ${data.cache_hit_ratio}% 重复计算</span>`;
  } else {
    if (kpiCacheTitle) kpiCacheTitle.textContent = 'Prompt 缓存命中 (Cache)';
    if (kpiCacheSub) kpiCacheSub.innerHTML = `<span>今日缓存:</span> <span class="highlight" style="color: #10b981;" id="valTodayCache">${formatTokens(data.today_cache)}</span> <span style="margin: 0 4px; color: var(--border-color);">|</span> <span>本月缓存:</span> <span class="highlight" style="color: #10b981;">${formatTokens(data.month_cache)}</span> <span style="font-size: 0.72rem; color: #94a3b8;">(总节省 ${formatTokens(data.total_cache)})</span>`;
  }

  // KPI 6: Active Models
  const models = data.models_breakdown || [];
  const kpiModelsTitle = document.getElementById('kpiModelsTitle');
  if (kpiModelsTitle) {
    if (isToday) kpiModelsTitle.textContent = '【今日】涉及模型型号';
    else if (isMonth) kpiModelsTitle.textContent = `【${currentDate}】涉及模型型号`;
    else if (currentDate !== 'all') kpiModelsTitle.textContent = `【${currentDate}】涉及模型型号`;
    else kpiModelsTitle.textContent = '涉及模型型号 (Models)';
  }

  const primaryModelEl = document.getElementById('valPrimaryModel');
  if (currentModel !== 'all') {
    document.getElementById('valModelsCount').textContent = '已筛选该模型';
    primaryModelEl.textContent = currentModel;
    primaryModelEl.title = currentModel;
    document.getElementById('valPrimaryModelTokens').textContent = formatTokens(data.total_tokens);
  } else {
    document.getElementById('valModelsCount').textContent = `${models.length} 种模型`;
    if (models.length > 0) {
      primaryModelEl.textContent = models[0].model_display_name;
      primaryModelEl.title = models[0].model_display_name;
      document.getElementById('valPrimaryModelTokens').textContent = formatTokens(models[0].total_tokens);
    } else {
      primaryModelEl.textContent = '--';
      primaryModelEl.title = '';
      document.getElementById('valPrimaryModelTokens').textContent = '--';
    }
  }

  // KPI 7: Conversations & Steps
  const agentLabel = currentAgent === 'all' ? '全平台' : (AGENT_CONFIG[currentAgent]?.name || currentAgent);
  document.getElementById('valAgentTypeBadge').textContent = agentLabel;
  document.getElementById('valConversations').textContent = `${data.total_convs} 个会话`;
  const kpiConvsTitle = document.getElementById('kpiConvsTitle');
  const kpiConvsSub = document.getElementById('kpiConvsSub');
  if (kpiConvsTitle) {
    if (isToday) kpiConvsTitle.textContent = '【今日】会话 / 交互轮次';
    else if (isMonth) kpiConvsTitle.textContent = `【${currentDate}】会话 / 交互轮次`;
    else if (currentDate !== 'all') kpiConvsTitle.textContent = `【${currentDate}】会话 / 交互轮次`;
    else kpiConvsTitle.textContent = '会话 / 交互轮次';
  }
  if (kpiConvsSub) {
    const scopeWord = isToday ? '今日' : (isMonth ? '本月' : (currentDate !== 'all' ? '当日' : '总计'));
    kpiConvsSub.innerHTML = `<span>${scopeWord}:</span> <span class="highlight" id="valTotalSteps">${formatExact(data.total_steps)}</span> <span>轮 | 单轮均值:</span> <span class="highlight" id="valAvgStepTokens">${formatTokens(data.avg_tokens_per_step)}</span>`;
  }
}

// 始终获取全平台各 Agent 的 Token 总量
async function loadAgentNavPills() {
  try {
    const url = `/api/summary?agent=all&model=all&date=${encodeURIComponent(currentDate)}`;
    const data = await fetchAPI(url);
    renderAgentNavPills(data.agents_breakdown || [], data.total_tokens);
  } catch (err) {
    console.error("Failed to load agent nav pills:", err);
  }
}

function renderAgentNavPills(agents, totalTokens) {
  const map = {
    codex: 0,
    claude: 0,
    pi: 0,
    qoder: 0,
    antigravity: 0,
    workbuddy: 0,
    zcode: 0
  };
  let computedTotal = 0;
  agents.forEach(a => {
    map[a.agent_type] = a.total_tokens || 0;
    computedTotal += a.total_tokens || 0;
  });

  const finalTotal = (totalTokens !== undefined && totalTokens !== null) ? totalTokens : computedTotal;

  const setPill = (id, val) => {
    const el = document.getElementById(id);
    if (el) el.textContent = formatTokens(val || 0);
  };

  setPill('pillTotalAll', finalTotal);
  setPill('pillTotalCodex', map['codex']);
  setPill('pillTotalClaude', map['claude']);
  setPill('pillTotalPi', map['pi']);
  setPill('pillTotalQoder', map['qoder']);
  setPill('pillTotalAg', map['antigravity']);
  setPill('pillTotalWb', map['workbuddy']);
  setPill('pillTotalZcode', map['zcode']);
}

// 2. Load Ledger Records Timeline
async function loadLedgerRecords() {
  try {
    const [resDaily, resMonthly] = await Promise.all([
      fetchAPI(`/api/daily?agent=${encodeURIComponent(currentAgent)}&model=${encodeURIComponent(currentModel)}&limit=60`),
      fetchAPI(`/api/monthly?agent=${encodeURIComponent(currentAgent)}&model=${encodeURIComponent(currentModel)}&limit=24`)
    ]);
    cachedDailyList = resDaily || [];
    cachedMonthlyList = resMonthly || [];

    const todayStr = getLocalDateString();
    const hasToday = cachedDailyList.some(d => d.date_bucket === todayStr || d.is_today);
    if (!hasToday) {
      cachedDailyList.unshift({
        date_bucket: todayStr,
        is_today: true,
        conv_count: 0,
        total_tokens: 0,
        input_tokens: 0,
        output_tokens: 0,
        cache_tokens: 0,
        total_steps: 0,
        agents_list: [],
        models_list: []
      });
    }

    updateDateSelectOptions(cachedDailyList, cachedMonthlyList);
    renderMonthlyOverviewShelf(cachedMonthlyList);
    renderLedgerCards();
  } catch (err) {
    console.error("Failed to load ledger records:", err);
  }
}

function renderMonthlyOverviewShelf(monthlyList) {
  const container = document.getElementById('monthlyCardsShelf');
  if (!container) return;
  container.innerHTML = '';

  if (!monthlyList || monthlyList.length === 0) {
    container.innerHTML = '<div style="color: var(--text-dim); padding: 15px;">暂无各月份聚合数据</div>';
    return;
  }

  const curMonthStr = getLocalDateString().slice(0, 7);
  const totalTokensAllMonths = monthlyList.reduce((acc, m) => acc + (m.total_tokens || 0), 0);

  const countBadge = document.getElementById('monthlyOverviewCountBadge');
  if (countBadge) {
    countBadge.textContent = `共 ${monthlyList.length} 个自然月归档 · 点击月份卡片一键按月筛选`;
  }

  monthlyList.forEach(m => {
    const card = document.createElement('div');
    const isSelected = (currentDate === m.month_bucket) || (currentDate === 'this_month' && m.is_current_month);
    card.className = `daily-card ${isSelected ? 'active' : ''}`;
    card.style.minWidth = '240px';
    card.title = isSelected
      ? `已筛选 ${m.month_bucket}，点击切回查看全部历史`
      : `点击切换：仅查看 ${m.month_bucket} 全月的所有会话与模型消耗`;

    card.onclick = () => {
      if (isSelected) {
        window.switchToAllDates();
      } else {
        currentDate = m.month_bucket;
        const sel = document.getElementById('dateFilterSelect');
        if (sel) sel.value = currentDate;
        currentPage = 0;
        showToast(`已切换至【${m.month_bucket}】月度视图`, 'info');
        loadAllData();
      }
    };

    let tagsHtml = '';
    (m.agents_list || []).forEach(ag => {
      const conf = AGENT_CONFIG[ag] || { name: ag, icon: '🤖', badgeClass: 'badge-project' };
      tagsHtml += `<span class="badge ${conf.badgeClass}" style="font-size: 0.68rem; padding: 2px 6px;">${conf.icon} ${escapeHtml(conf.name)}</span>`;
    });
    (m.models_list || []).slice(0, 2).forEach(model => {
      tagsHtml += `<span class="badge badge-model" style="font-size: 0.68rem; padding: 2px 6px;">${escapeHtml(model)}</span>`;
    });

    const isCurrent = m.is_current_month || (m.month_bucket === curMonthStr);
    const monthTag = isCurrent ? '<span style="color: #a855f7; font-size: 0.72rem; font-weight: bold; background: rgba(168, 85, 247, 0.15); padding: 1px 6px; border-radius: 4px;">本月</span>' : '';

    const inAndCache = m.input_tokens + m.cache_tokens;
    const cacheRate = inAndCache > 0 ? ((m.cache_tokens / inAndCache) * 100).toFixed(1) : 0;
    const pctOfAll = totalTokensAllMonths > 0 ? ((m.total_tokens / totalTokensAllMonths) * 100).toFixed(1) : 0;

    card.innerHTML = `
      <div class="daily-card-header">
        <div class="daily-card-date">
          <span style="font-weight: 700;">🗓️ ${escapeHtml(m.month_bucket)}</span>
          ${monthTag}
        </div>
        <div class="daily-card-tokens" style="color: #c084fc; font-weight: 800; font-size: 1.12rem;">${formatTokens(m.total_tokens)}</div>
      </div>
      <div style="margin: 6px 0 8px; height: 4px; background: rgba(255, 255, 255, 0.08); border-radius: 2px; overflow: hidden;">
        <div style="height: 100%; width: ${pctOfAll}%; background: linear-gradient(90deg, #a855f7, #6366f1); border-radius: 2px;"></div>
      </div>
      <div class="daily-card-meta" style="font-size: 0.74rem;">
        <span>${m.conv_count} 会话 · ${formatExact(m.total_steps || 0)} 轮</span>
        <span>占总量: <b>${pctOfAll}%</b></span>
      </div>
      <div class="daily-card-meta" style="font-size: 0.72rem; color: #94a3b8; margin-top: 2px;">
        <span>入: ${formatTokens(m.input_tokens)} · 出: ${formatTokens(m.output_tokens)}</span>
        <span>缓存: ${formatTokens(m.cache_tokens)} (${cacheRate}%)</span>
      </div>
      <div class="daily-card-tags" style="margin-top: 6px;">
        ${tagsHtml}
      </div>
    `;
    container.appendChild(card);
  });
}

function renderLedgerCards() {
  if (currentLedgerView === 'month') {
    renderMonthlyCards(cachedMonthlyList || []);
  } else {
    renderDailyCards(cachedDailyList || []);
  }
}

function renderMonthlyCards(monthlyList) {
  const container = document.getElementById('dailyCardsContainer');
  container.innerHTML = '';

  if (!monthlyList || monthlyList.length === 0) {
    container.innerHTML = '<div style="color: var(--text-dim); padding: 20px;">暂无月份统计记录</div>';
    return;
  }

  const curMonthStr = getLocalDateString().slice(0, 7);

  monthlyList.forEach(m => {
    const card = document.createElement('div');
    const isActive = (currentDate === m.month_bucket) || (currentDate === 'this_month' && m.is_current_month);
    card.className = `daily-card ${isActive ? 'active' : ''}`;
    card.title = isActive
      ? `已筛选 ${m.month_bucket}，点击切回查看全部历史`
      : `点击切换筛选：仅看 ${m.month_bucket} 全月的会话与指标`;
    card.onclick = () => {
      if (isActive) {
        window.switchToAllDates();
      } else {
        currentDate = m.month_bucket;
        const sel = document.getElementById('dateFilterSelect');
        if (sel) sel.value = currentDate;
        currentPage = 0;
        showToast(`已切换至【${m.month_bucket}】月度视图`, 'info');
        loadAllData();
      }
    };

    let tagsHtml = '';
    (m.agents_list || []).forEach(ag => {
      const conf = AGENT_CONFIG[ag] || { name: ag, icon: '🤖', badgeClass: 'badge-project' };
      tagsHtml += `<span class="badge ${conf.badgeClass}" style="font-size: 0.68rem; padding: 2px 6px;">${conf.icon} ${escapeHtml(conf.name)}</span>`;
    });
    (m.models_list || []).slice(0, 3).forEach(model => {
      tagsHtml += `<span class="badge badge-model" style="font-size: 0.68rem; padding: 2px 6px;">${escapeHtml(model)}</span>`;
    });

    const isCurrent = m.is_current_month || (m.month_bucket === curMonthStr);
    const monthTag = isCurrent ? '<span style="color: #818cf8; font-size: 0.72rem; font-weight: bold;">[本月]</span>' : '';

    const inAndCache = m.input_tokens + m.cache_tokens;
    const cacheRate = inAndCache > 0 ? ((m.cache_tokens / inAndCache) * 100).toFixed(1) : 0;

    card.innerHTML = `
      <div class="daily-card-header">
        <div class="daily-card-date">
          <span>🗓️ ${escapeHtml(m.month_bucket)} 月度</span>
          ${monthTag}
        </div>
        <div class="daily-card-tokens" style="color: #c084fc;">${formatTokens(m.total_tokens)}</div>
      </div>
      <div class="daily-card-meta">
        <span>${m.conv_count} 个会话 · ${m.total_steps || 0} 轮交互</span>
        <span>缓存: ${formatTokens(m.cache_tokens)} (${cacheRate}%)</span>
      </div>
      <div class="daily-card-tags">
        ${tagsHtml}
      </div>
    `;
    container.appendChild(card);
  });
}

function renderDailyCards(dailyList) {
  const container = document.getElementById('dailyCardsContainer');
  container.innerHTML = '';

  if (!dailyList || dailyList.length === 0) {
    container.innerHTML = '<div style="color: var(--text-dim); padding: 20px;">暂无该条件下的天数记录</div>';
    return;
  }

  const todayStr = getLocalDateString();

  dailyList.forEach(d => {
    const card = document.createElement('div');
    const isTodayCard = d.is_today || (d.date_bucket === todayStr);
    const isActive = (currentDate === d.date_bucket) || ((currentDate === 'today' || currentDate === todayStr) && isTodayCard);
    card.className = `daily-card ${isActive ? 'active' : ''}`;
    card.title = isActive
      ? `已筛选 ${d.date_bucket}，点击切回查看全部历史`
      : `点击切换筛选：仅看 ${d.date_bucket} 的所有会话与指标`;
    card.onclick = () => {
      if (isActive) {
        window.switchToAllDates();
      } else {
        currentDate = d.date_bucket;
        const sel = document.getElementById('dateFilterSelect');
        if (sel) sel.value = currentDate;
        currentPage = 0;
        showToast(`已切换至【${d.date_bucket}】单日视图`, 'info');
        loadAllData();
      }
    };

    let tagsHtml = '';
    (d.agents_list || []).forEach(ag => {
      const conf = AGENT_CONFIG[ag] || { name: ag, icon: '🤖', badgeClass: 'badge-project' };
      tagsHtml += `<span class="badge ${conf.badgeClass}" style="font-size: 0.68rem; padding: 2px 6px;">${conf.icon} ${escapeHtml(conf.name)}</span>`;
    });
    (d.models_list || []).slice(0, 2).forEach(m => {
      tagsHtml += `<span class="badge badge-model" style="font-size: 0.68rem; padding: 2px 6px;">${escapeHtml(m)}</span>`;
    });

    const todayTag = isTodayCard ? '<span style="color: #10b981; font-size: 0.72rem; font-weight: bold;">[今日]</span>' : '';

    card.innerHTML = `
      <div class="daily-card-header">
        <div class="daily-card-date">
          <span>📅 ${escapeHtml(d.date_bucket)}</span>
          ${todayTag}
        </div>
        <div class="daily-card-tokens">${formatTokens(d.total_tokens)}</div>
      </div>
      <div class="daily-card-meta">
        <span>${d.conv_count} 个会话 · ${d.total_steps || 0} 轮交互</span>
        <span>缓存: ${formatTokens(d.cache_tokens)}</span>
      </div>
      <div class="daily-card-tags">
        ${tagsHtml}
      </div>
    `;
    container.appendChild(card);
  });
}

function updateDateSelectOptions(dailyList, monthlyList) {
  const sel = document.getElementById('dateFilterSelect');
  const todayStr = getLocalDateString();
  const thisMonthStr = todayStr.slice(0, 7);

  let opts = '';

  // 1. Quick Scope
  opts += `<optgroup label="快捷范围">`;
  const totalTokensAll = (monthlyList || []).reduce((acc, m) => acc + (m.total_tokens || 0), 0);
  const totalTokensStr = totalTokensAll > 0 ? ` (${formatTokens(totalTokensAll)})` : '';
  const isAllSelected = (currentDate === 'all');
  opts += `<option value="all" ${isAllSelected ? 'selected' : ''}>🌐 全部历史总量 (All Time)${totalTokensStr}</option>`;

  const monthItem = (monthlyList || []).find(m => m.is_current_month || m.month_bucket === thisMonthStr);
  const monthTokens = monthItem ? formatTokens(monthItem.total_tokens) : '--';
  const isThisMonthSelected = (currentDate === thisMonthStr || currentDate === 'this_month');
  opts += `<option value="${thisMonthStr}" ${isThisMonthSelected ? 'selected' : ''}>🗓️ 本月 (${thisMonthStr}) (${monthTokens})</option>`;

  const todayItem = (dailyList || []).find(d => d.is_today || d.date_bucket === todayStr);
  const todayTokens = todayItem ? formatTokens(todayItem.total_tokens) : '--';
  const isTodaySelected = (currentDate === todayStr || currentDate === 'today');
  opts += `<option value="${todayStr}" ${isTodaySelected ? 'selected' : ''}>📅 今日 (${todayStr}) (${todayTokens})</option>`;
  opts += `</optgroup>`;

  // 2. Monthly Scope
  if (monthlyList && monthlyList.length > 0) {
    opts += `<optgroup label="按月度归档筛选 (Monthly)">`;
    monthlyList.forEach(m => {
      const isCur = m.is_current_month || m.month_bucket === thisMonthStr;
      const curTag = isCur ? ' [本月]' : '';
      const isSelected = (m.month_bucket === currentDate) ? 'selected' : '';
      opts += `<option value="${escapeHtml(m.month_bucket)}" ${isSelected}>🗓️ ${escapeHtml(m.month_bucket)} (${formatTokens(m.total_tokens)})${curTag}</option>`;
    });
    opts += `</optgroup>`;
  }

  // 3. Daily Scope
  if (dailyList && dailyList.length > 0) {
    opts += `<optgroup label="按单天时间流筛选 (Daily)">`;
    dailyList.forEach(d => {
      if (d.date_bucket === todayStr) return;
      const isSelected = (d.date_bucket === currentDate) ? 'selected' : '';
      opts += `<option value="${escapeHtml(d.date_bucket)}" ${isSelected}>📅 ${escapeHtml(d.date_bucket)} (${formatTokens(d.total_tokens)})</option>`;
    });
    opts += `</optgroup>`;
  }

  sel.innerHTML = opts;
  if (currentDate === 'today') {
    sel.value = todayStr;
  } else if (currentDate === 'this_month') {
    sel.value = thisMonthStr;
  } else {
    sel.value = currentDate;
  }
}

// 始终获取当前 Agent 与当前日期范围下的所有模型列表 (model=all)
async function loadModelsList() {
  try {
    const [summaryData, globalModels] = await Promise.all([
      fetchAPI(`/api/summary?agent=${encodeURIComponent(currentAgent)}&model=all&date=${encodeURIComponent(currentDate)}`),
      fetchAPI('/api/models')
    ]);
    allCachedModels = globalModels || [];
    lastLoadedModels = summaryData.models_breakdown || [];
    renderModelsList(lastLoadedModels);
    updateModelSelectOptions(lastLoadedModels);
  } catch (err) {
    console.error("Failed to load models list:", err);
  }
}

function renderModelsList(models) {
  const listEl = document.getElementById('modelsList');
  const countEl = document.getElementById('modelCountBadge');
  const btnClear = document.getElementById('btnClearModelFilter');

  if (btnClear) {
    if (currentModel !== 'all') {
      btnClear.style.display = 'inline-flex';
      btnClear.textContent = `✕ 返回全部 (${currentModel})`;
    } else {
      btnClear.style.display = 'none';
    }
  }

  if (countEl) {
    countEl.textContent = `共 ${models ? models.length : 0} 种在用模型`;
  }
  listEl.innerHTML = '';

  if (!models || models.length === 0) {
    listEl.innerHTML = '<div style="color: var(--text-dim); padding: 15px; text-align: center;">暂无模型数据</div>';
    return;
  }

  const sorted = [...models].sort((a, b) => {
    const valA = Number(a[currentModelSortMetric] || 0);
    const valB = Number(b[currentModelSortMetric] || 0);
    return valB - valA;
  });

  const maxVal = Math.max(...sorted.map(m => Number(m[currentModelSortMetric] || 0)), 1);

  sorted.forEach((m, idx) => {
    const isSelected = (currentModel === m.model_display_name);
    const curVal = Number(m[currentModelSortMetric] || 0);
    const percent = Math.min(100, Math.max(2, ((curVal / maxVal) * 100))).toFixed(1);
    const item = document.createElement('div');
    item.className = `project-item ${isSelected ? 'active' : ''}`;
    item.title = isSelected
      ? `当前已筛选 ${m.model_display_name}，再次点击取消筛选`
      : `点击筛选仅看 ${m.model_display_name} 会话与指标`;

    item.onclick = () => {
      if (currentModel === m.model_display_name) {
        currentModel = 'all';
        showToast('已取消模型筛选', 'info');
      } else {
        currentModel = m.model_display_name;
        showToast(`已筛选模型【${currentModel}】`, 'info');
      }
      const sel = document.getElementById('modelFilterSelect');
      if (sel) sel.value = currentModel;
      currentPage = 0;
      loadAllData();
    };

    let rankClass = '';
    if (idx === 0) rankClass = 'rank-1';
    else if (idx === 1) rankClass = 'rank-2';
    else if (idx === 2) rankClass = 'rank-3';

    const rankBadge = `<span class="model-rank-badge ${rankClass}">#${idx + 1}</span>`;
    const activePill = isSelected
      ? '<span class="model-active-pill" style="font-size:0.7rem; color:#fbbf24; margin-left:4px;">✓ 已筛选</span>'
      : '';

    let primaryValHtml = '';
    let subStatsHtml = '';

    if (currentModelSortMetric === 'output_tokens') {
      primaryValHtml = `<span style="font-weight: 700; color: #ec4899;">${formatTokens(m.output_tokens)}</span> <span style="font-size: 0.72rem; color: #f472b6;">输出</span>`;
      subStatsHtml = `<span>总: ${formatTokens(m.total_tokens)}</span><span>入: ${formatTokens(m.input_tokens)} | 缓: ${formatTokens(m.cache_tokens)} · ${m.conv_count} 会话</span>`;
    } else if (currentModelSortMetric === 'input_tokens') {
      primaryValHtml = `<span style="font-weight: 700; color: #3b82f6;">${formatTokens(m.input_tokens)}</span> <span style="font-size: 0.72rem; color: #93c5fd;">输入</span>`;
      subStatsHtml = `<span>总: ${formatTokens(m.total_tokens)}</span><span>出: ${formatTokens(m.output_tokens)} | 缓: ${formatTokens(m.cache_tokens)} · ${m.conv_count} 会话</span>`;
    } else {
      primaryValHtml = `<span style="font-weight: 700; color: #fff;">${formatTokens(m.total_tokens)}</span>`;
      subStatsHtml = `<span>${m.conv_count} 个会话</span><span>出: ${formatTokens(m.output_tokens)} | 入: ${formatTokens(m.input_tokens)} | 缓: ${formatTokens(m.cache_tokens)}</span>`;
    }

    item.innerHTML = `
      <div class="project-item-header">
        <span style="font-weight: 600; color: #fbbf24; display: flex; align-items: center; gap: 4px;">
          ${rankBadge} 🤖 ${escapeHtml(m.model_display_name)} ${activePill}
        </span>
        <div>${primaryValHtml}</div>
      </div>
      <div class="project-progress-bar">
        <div class="project-progress-fill" style="width: ${percent}%; background: linear-gradient(90deg, #f59e0b, #ec4899);"></div>
      </div>
      <div class="project-stats">
        ${subStatsHtml}
      </div>
    `;
    listEl.appendChild(item);
  });
}

function updateModelSelectOptions(currentScopeModels) {
  const sel = document.getElementById('modelFilterSelect');
  if (!sel) return;
  const currentVal = currentModel;

  const modelsToUse = (currentScopeModels && currentScopeModels.length > 0) ? currentScopeModels : allCachedModels;

  let opts = `<option value="all" ${currentVal === 'all' ? 'selected' : ''}>🎯 全部模型型号 (All Models)</option>`;
  let foundCurrent = (currentVal === 'all');

  (modelsToUse || []).forEach(m => {
    const name = m.model_display_name;
    if (name === currentVal) foundCurrent = true;
    const isSelected = (name === currentVal) ? 'selected' : '';
    opts += `<option value="${escapeHtml(name)}" ${isSelected}>🤖 ${escapeHtml(name)} (${formatTokens(m.total_tokens)})</option>`;
  });

  if (!foundCurrent && currentVal !== 'all') {
    opts += `<option value="${escapeHtml(currentVal)}" selected>🤖 ${escapeHtml(currentVal)}</option>`;
  }

  sel.innerHTML = opts;
}

// 3. Load Trends Chart (Zero-flicker update + interactive bar drill-down)
async function loadTrends() {
  const dateParam = (currentGranularity === 'month') ? 'all' : currentDate;
  const url = `/api/trends?granularity=${encodeURIComponent(currentGranularity)}&days=30&agent=${encodeURIComponent(currentAgent)}&model=${encodeURIComponent(currentModel)}&date=${encodeURIComponent(dateParam)}`;
  const rawTrends = await fetchAPI(url);
  const trends = Array.isArray(rawTrends) ? rawTrends : [];
  trendRawBuckets = trends.map(t => t.time_bucket);

  const todayStr = getLocalDateString();
  const chartTitleEl = document.getElementById('trendChartTitle');
  if (chartTitleEl) {
    const isToday = (currentDate === todayStr || currentDate === 'today');
    if (currentGranularity === 'month') {
      chartTitleEl.textContent = `Token 消耗与缓存走势 (跨月全景对比)`;
    } else if (isToday) {
      chartTitleEl.textContent = `Token 消耗与缓存走势 (今日 ${currentGranularity === 'hour' ? '24小时分时' : '当日统计'})`;
    } else if (currentDate && currentDate.length === 7) {
      chartTitleEl.textContent = `Token 消耗与缓存走势 (【${currentDate}】月度${currentGranularity === 'hour' ? '分时' : '每日'}走势)`;
    } else if (currentDate !== 'all') {
      chartTitleEl.textContent = `Token 消耗与缓存走势 (【${currentDate}】${currentGranularity === 'hour' ? '24小时分时' : '当日统计'})`;
    } else {
      const gLabel = currentGranularity === 'hour' ? '分时走势' : '近60天每日走势';
      chartTitleEl.textContent = `Token 消耗与缓存走势 (${gLabel})`;
    }
  }

  const labels = trends.map(t => {
    if (currentGranularity === 'hour') {
      return (currentDate && currentDate !== 'all' && currentDate.length === 10)
        ? t.time_bucket.slice(11)
        : t.time_bucket.slice(5);
    }
    if (currentGranularity === 'month') return t.time_bucket;
    return t.time_bucket.slice(5);
  });

  const inputData = trends.map(t => t.input_tokens);
  const outputData = trends.map(t => t.output_tokens);
  const cacheData = trends.map(t => t.cache_tokens);

  const inputBg = trends.map(t =>
    (currentDate !== 'all' && t.time_bucket === currentDate)
      ? 'rgba(168, 85, 247, 0.95)'
      : 'rgba(59, 130, 246, 0.78)'
  );
  const outputBg = trends.map(t =>
    (currentDate !== 'all' && t.time_bucket === currentDate)
      ? 'rgba(245, 158, 11, 0.95)'
      : 'rgba(236, 72, 153, 0.85)'
  );

  const barThickness = currentGranularity === 'month' ? 52 : 32;

  // Update existing chart in place to prevent 15s polling flicker
  if (trendChartInstance) {
    trendChartInstance.data.labels = labels;
    trendChartInstance.data.datasets[0].data = inputData;
    trendChartInstance.data.datasets[0].backgroundColor = inputBg;
    trendChartInstance.data.datasets[0].maxBarThickness = barThickness;
    trendChartInstance.data.datasets[1].data = outputData;
    trendChartInstance.data.datasets[1].backgroundColor = outputBg;
    trendChartInstance.data.datasets[1].maxBarThickness = barThickness;
    trendChartInstance.data.datasets[2].data = cacheData;
    trendChartInstance.update('none');
    return;
  }

  const ctx = document.getElementById('trendChart').getContext('2d');
  trendChartInstance = new Chart(ctx, {
    type: 'bar',
    data: {
      labels: labels,
      datasets: [
        {
          label: '输入 Tokens',
          data: inputData,
          backgroundColor: inputBg,
          hoverBackgroundColor: '#3b82f6',
          borderColor: '#3b82f6',
          borderWidth: 1,
          borderRadius: 4,
          maxBarThickness: barThickness,
          stack: 'tokens'
        },
        {
          label: '输出 Tokens',
          data: outputData,
          backgroundColor: outputBg,
          hoverBackgroundColor: '#ec4899',
          borderColor: '#ec4899',
          borderWidth: 1,
          borderRadius: 4,
          maxBarThickness: barThickness,
          stack: 'tokens'
        },
        {
          label: 'Prompt 缓存读取 (Cache)',
          data: cacheData,
          type: 'line',
          borderColor: '#10b981',
          backgroundColor: 'rgba(16, 185, 129, 0.12)',
          borderWidth: 2.5,
          pointRadius: 3,
          pointHoverRadius: 6,
          pointBackgroundColor: '#10b981',
          fill: true,
          tension: 0.4
        }
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      onHover: (event, elements) => {
        if (event?.native?.target) {
          event.native.target.style.cursor = (elements.length > 0 && currentGranularity !== 'hour') ? 'pointer' : 'default';
        }
      },
      onClick: (_event, elements) => {
        if (!elements || elements.length === 0 || currentGranularity === 'hour') return;
        const idx = elements[0].index;
        const bucket = trendRawBuckets[idx];
        if (!bucket) return;
        if (currentDate === bucket) {
          window.switchToAllDates();
        } else {
          currentDate = bucket;
          const sel = document.getElementById('dateFilterSelect');
          if (sel) sel.value = currentDate;
          currentPage = 0;
          showToast(`已从图表下钻至【${bucket}】`, 'info');
          loadAllData();
        }
      },
      plugins: {
        legend: {
          labels: {
            color: '#94a3b8',
            font: { size: 11, family: 'Inter, system-ui, sans-serif' },
            usePointStyle: true,
            boxWidth: 8
          }
        },
        tooltip: {
          mode: 'index',
          intersect: false,
          backgroundColor: 'rgba(15, 23, 42, 0.95)',
          borderColor: 'rgba(255, 255, 255, 0.12)',
          borderWidth: 1,
          padding: 10,
          titleColor: '#f8fafc',
          bodyColor: '#cbd5e1',
          cornerRadius: 8,
          callbacks: {
            title: (items) => {
              if (!items.length) return '';
              const b = trendRawBuckets[items[0].dataIndex];
              return b || items[0].label;
            },
            label: (ctx) => `  ${ctx.dataset.label}: ${formatTokens(ctx.raw)} (${formatExact(ctx.raw)})`
          }
        }
      },
      scales: {
        x: {
          stacked: true,
          grid: { color: 'rgba(255, 255, 255, 0.04)' },
          ticks: { color: '#64748b', font: { size: 11 } }
        },
        y: {
          grid: { color: 'rgba(255, 255, 255, 0.04)' },
          ticks: { color: '#64748b', font: { size: 11 }, callback: (v) => formatTokens(v) }
        }
      }
    }
  });
}

// 4. Load Conversations Table
async function loadConversations() {
  const offset = currentPage * pageSize;
  const url = `/api/conversations?agent=${encodeURIComponent(currentAgent)}&model=${encodeURIComponent(currentModel)}&date=${encodeURIComponent(currentDate)}&sort=${encodeURIComponent(currentSort)}&order=${encodeURIComponent(currentOrder)}&search=${encodeURIComponent(searchQuery)}&limit=${pageSize}&offset=${offset}`;

  const data = await fetchAPI(url);
  const tbody = document.getElementById('conversationTableBody');
  tbody.innerHTML = '';

  if (!data.items || data.items.length === 0) {
    const isMonthScope = (currentDate && currentDate.length === 7);
    const scopeLabel = isMonthScope ? `月份【${escapeHtml(currentDate)}】` : `日期【${escapeHtml(currentDate)}】`;
    const emptyHtml = `
      <div style="padding: 36px 15px; text-align: center;">
        <div style="font-size: 1rem; color: #cbd5e1; margin-bottom: 8px;">
          ${currentDate !== 'all' ? `🗓️ 所选${scopeLabel}暂无匹配会话` : '🔍 暂无匹配的会话数据'}
        </div>
        <div style="font-size: 0.85rem; color: var(--text-dim); margin-bottom: 16px;">
          ${currentDate !== 'all' ? '当前处于特定日期/月份或筛选视图，可点击下方按钮恢复全局视图。' : '请尝试更改筛选条件或清除搜索关键词。'}
        </div>
        <button type="button" class="btn btn-primary" style="padding: 6px 16px; font-size: 0.84rem;" onclick="resetAllFilters()">
          ↺ 重置全部筛选条件
        </button>
      </div>
    `;
    tbody.innerHTML = `<tr><td colspan="10">${emptyHtml}</td></tr>`;
    updatePagination(0, 0);
    return;
  }

  const maxTokens = Math.max(...data.items.map(c => c.total_tokens), 1);

  data.items.forEach(c => {
    const tr = document.createElement('tr');
    tr.className = 'conv-row';
    tr.title = '点击查看该会话逐轮上下文详情';

    const agentConf = AGENT_CONFIG[c.agent_type] || { name: c.agent_type, icon: '🤖', badgeClass: 'badge-project' };
    const agentBadge = `<span class="badge ${agentConf.badgeClass}">${agentConf.icon} ${escapeHtml(agentConf.name)}</span>`;
    const modelList = Array.from(new Set((c.model_display_name || 'Auto').split(',').map(m => m.trim()))).filter(Boolean);
    const modelBadge = modelList.map(m => `<span class="badge badge-model" style="margin-right: 3px; margin-bottom: 2px;">${escapeHtml(m)}</span>`).join('');

    const barWidth = Math.max(Math.min((c.total_tokens / maxTokens) * 100, 100), 2);
    const titleRaw = c.title || c.preview || '无标题会话';
    const safeTitle = escapeHtml(titleRaw);
    const projectLabel = c.project_id === 'outside-of-project' ? '独立' : (c.project_id ? c.project_id.slice(0, 16) : '--');
    const dateLabel = c.date_bucket ? `<span style="font-family: monospace; font-size: 0.78rem; color: #a5b4fc;">${escapeHtml(c.date_bucket)}</span>` : '--';

    tr.innerHTML = `
      <td>${dateLabel}</td>
      <td>${agentBadge}</td>
      <td>${modelBadge}</td>
      <td class="conv-title-cell">
        <div class="conv-title-text" title="${safeTitle}">${safeTitle}</div>
        <div class="conv-meta-badges">
          <span class="copy-cid-chip" style="font-family: monospace; font-size: 0.72rem; color: var(--text-dim); cursor: pointer;" title="点击复制完整会话 ID">
            #${escapeHtml(c.conversation_id.slice(0, 8))}
          </span>
        </div>
      </td>
      <td><span class="badge badge-project" title="${escapeHtml(c.project_id)}">${escapeHtml(projectLabel)}</span></td>
      <td style="font-weight: 600;">${c.step_records_count || c.step_count} 轮</td>
      <td class="token-cell" style="color: #10b981;">${formatTokens(c.total_cache_tokens)}</td>
      <td class="token-cell">
        <div>入: ${formatTokens(c.total_input_tokens)}</div>
        <div class="token-split">出: ${formatTokens(c.total_output_tokens)}</div>
      </td>
      <td class="token-cell">
        <div class="token-total">${formatTokens(c.total_tokens)}</div>
        <div class="project-progress-bar" style="margin-top: 4px; height: 4px;">
          <div class="project-progress-fill" style="width: ${barWidth}%;"></div>
        </div>
      </td>
      <td style="text-align: right;">
        <button type="button" class="btn btn-detail-row" style="padding: 4px 10px; font-size: 0.78rem;">
          详情 &gt;
        </button>
      </td>
    `;

    tr.addEventListener('click', (e) => {
      if (e.target.closest('.copy-cid-chip')) {
        e.stopPropagation();
        copyId(c.conversation_id);
        return;
      }
      openConversationDetail(c.conversation_id);
    });

    tbody.appendChild(tr);
  });

  updatePagination(data.total, offset);
}

function updatePagination(total, offset) {
  const totalPages = Math.ceil(total / pageSize) || 1;
  document.getElementById('pageInfo').textContent = `第 ${currentPage + 1} / ${totalPages} 页 (共 ${total} 条)`;
  document.getElementById('btnPrevPage').disabled = currentPage <= 0;
  document.getElementById('btnNextPage').disabled = (offset + pageSize) >= total;
}

// 5. Open Single Conversation Drawer (Race-safe)
async function openConversationDetail(cid) {
  const reqId = ++activeDetailRequestId;
  currentDrawerCid = cid;

  const drawer = document.getElementById('conversationDrawer');
  const backdrop = document.getElementById('drawerBackdrop');
  drawer.classList.add('open');
  backdrop.classList.add('open');

  document.getElementById('drawerTitle').textContent = '加载中...';
  document.getElementById('drawerCid').textContent = cid;
  document.getElementById('drawerContextPeak').textContent = '--';
  document.getElementById('drawerStepsBody').innerHTML = '<tr><td colspan="7" style="text-align:center; padding: 20px;">数据加载中...</td></tr>';

  try {
    const data = await fetchAPI(`/api/conversation/${encodeURIComponent(cid)}`);
    if (reqId !== activeDetailRequestId) return; // Stale response guard

    const agentConf = AGENT_CONFIG[data.agent_type] || { name: data.agent_type, icon: '🤖', badgeClass: 'badge-project' };
    const agentEl = document.getElementById('drawerAgentBadge');
    agentEl.className = `badge ${agentConf.badgeClass}`;
    agentEl.textContent = `${agentConf.icon} ${agentConf.name}`;

    const modelEl = document.getElementById('drawerModelBadge');
    const modelSet = Array.from(new Set((data.model_display_name || 'Auto').split(',').map(m => m.trim()))).filter(Boolean);
    modelEl.textContent = `🤖 ${modelSet.join(' · ')}`;

    const dateEl = document.getElementById('drawerDateBadge');
    dateEl.textContent = `📅 ${data.date_bucket || '--'}`;

    document.getElementById('drawerTitle').textContent = data.title || '无标题会话';
    document.getElementById('drawerTotalTokens').textContent = formatTokens(data.total_tokens);

    const cacheTotal = (data.total_input_tokens + data.total_cache_tokens);
    const cacheRate = cacheTotal > 0 ? ((data.total_cache_tokens / cacheTotal) * 100).toFixed(1) : 0;
    document.getElementById('drawerCacheRate').textContent = `${cacheRate}%`;
    document.getElementById('drawerStepCount').textContent = `${data.steps ? data.steps.length : (data.step_count || 1)} 轮`;

    const steps = data.steps || [];
    const peakCtx = steps.reduce((mx, st) => Math.max(mx, (st.input_tokens || 0) + (st.cache_read_tokens || 0)), 0);
    document.getElementById('drawerContextPeak').textContent = peakCtx > 0 ? formatTokens(peakCtx) : '--';

    renderStepGrowthChart(steps);

    // Subagents
    const subSection = document.getElementById('drawerSubagentsSection');
    const subList = document.getElementById('drawerSubagentsList');
    if (data.subagents && data.subagents.length > 0) {
      subSection.style.display = 'block';
      subList.innerHTML = '';
      data.subagents.forEach(s => {
        const item = document.createElement('div');
        item.className = 'project-item';
        item.innerHTML = `
          <div class="project-item-header">
            <span>🤖 ${escapeHtml(s.agent_role || 'Subagent')} [${escapeHtml(s.model_display_name)}]</span>
            <span style="color: #fff;">${formatTokens(s.total_tokens)}</span>
          </div>
          <div class="project-stats">
            <span>${s.step_records_count} 轮交互</span>
            <span style="color: var(--primary-light);">点击查看详情 &gt;</span>
          </div>
        `;
        item.addEventListener('click', () => openConversationDetail(s.conversation_id));
        subList.appendChild(item);
      });
    } else {
      subSection.style.display = 'none';
    }

    // Steps Table
    const stepsBody = document.getElementById('drawerStepsBody');
    stepsBody.innerHTML = '';

    if (steps.length === 0) {
      stepsBody.innerHTML = '<tr><td colspan="7" style="text-align:center; color: var(--text-dim); padding: 15px;">该会话仅有总体汇总，无单步流水</td></tr>';
      return;
    }

    steps.forEach(s => {
      const tr = document.createElement('tr');
      let toolsHtml = '<span style="color: var(--text-dim);">-</span>';
      if (s.tool_names) {
        toolsHtml = s.tool_names.split(',').filter(Boolean).map(name => `<span class="tool-tag">${escapeHtml(name.trim())}</span>`).join('');
      }

      tr.innerHTML = `
        <td style="font-weight: 600;">#${s.step_index}</td>
        <td><span class="badge badge-model" style="font-size: 0.7rem;">${escapeHtml(s.model_display_name || '--')}</span></td>
        <td style="font-family: monospace;">${formatTokens(s.input_tokens)}</td>
        <td style="font-family: monospace; color: #10b981;">${formatTokens(s.cache_read_tokens)}</td>
        <td style="font-family: monospace; color: #ec4899;">${formatTokens(s.output_tokens)}</td>
        <td>${toolsHtml}</td>
        <td style="font-size: 0.72rem; color: var(--text-dim);">${escapeHtml(formatDate(s.created_at))}</td>
      `;
      stepsBody.appendChild(tr);
    });

  } catch (err) {
    if (reqId !== activeDetailRequestId) return;
    console.error(err);
    document.getElementById('drawerTitle').textContent = '加载失败';
    document.getElementById('drawerStepsBody').innerHTML = '<tr><td colspan="7" style="text-align:center; color: #f43f5e; padding: 20px;">加载会话详情失败，请稍后重试</td></tr>';
  }
}

function renderStepGrowthChart(steps) {
  const ctx = document.getElementById('stepGrowthChart').getContext('2d');
  if (stepGrowthChartInstance) {
    stepGrowthChartInstance.destroy();
  }

  const labels = steps.map(s => `#${s.step_index}`);
  const inputData = steps.map(s => s.input_tokens);
  const cacheData = steps.map(s => s.cache_read_tokens);
  const outputData = steps.map(s => s.output_tokens);

  stepGrowthChartInstance = new Chart(ctx, {
    type: 'line',
    data: {
      labels: labels,
      datasets: [
        {
          label: '未缓存输入 (Uncached Input)',
          data: inputData,
          borderColor: '#6366f1',
          backgroundColor: 'rgba(99, 102, 241, 0.12)',
          fill: true,
          tension: 0.35,
          borderWidth: 2,
          pointRadius: 2.5,
          pointHoverRadius: 5
        },
        {
          label: '缓存读取 (Cache Hit)',
          data: cacheData,
          borderColor: '#10b981',
          borderWidth: 2,
          pointRadius: 2,
          pointHoverRadius: 4,
          borderDash: [4, 4]
        },
        {
          label: '模型生成 (Output Tokens)',
          data: outputData,
          borderColor: '#ec4899',
          borderWidth: 2,
          pointRadius: 2,
          pointHoverRadius: 4
        }
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: {
          labels: {
            color: '#94a3b8',
            font: { size: 10, family: 'Inter, system-ui, sans-serif' },
            usePointStyle: true,
            boxWidth: 8
          }
        },
        tooltip: {
          mode: 'index',
          intersect: false,
          backgroundColor: 'rgba(15, 23, 42, 0.95)',
          borderColor: 'rgba(255, 255, 255, 0.12)',
          borderWidth: 1,
          padding: 8,
          titleColor: '#f8fafc',
          bodyColor: '#cbd5e1',
          cornerRadius: 6,
          callbacks: {
            label: (ctx) => `  ${ctx.dataset.label}: ${formatTokens(ctx.raw)} (${formatExact(ctx.raw)})`
          }
        }
      },
      scales: {
        x: { grid: { color: 'rgba(255, 255, 255, 0.04)' }, ticks: { color: '#64748b' } },
        y: {
          grid: { color: 'rgba(255, 255, 255, 0.04)' },
          ticks: { color: '#64748b', callback: (v) => formatTokens(v) }
        }
      }
    }
  });
}

function closeDrawer() {
  document.getElementById('conversationDrawer').classList.remove('open');
  document.getElementById('drawerBackdrop').classList.remove('open');
}

function copyId(id) {
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(id).then(() => {
      showToast(`已复制会话 ID: #${id.slice(0, 8)}`, 'success');
    }).catch(() => {
      prompt('会话ID:', id);
    });
  } else {
    prompt('会话ID:', id);
  }
}
